package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"carecompanion/internal/config"
	"carecompanion/internal/database"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

func main() {
	// Command line flags
	email := flag.String("email", "", "Admin email address (required)")
	firstName := flag.String("first-name", "", "Admin first name (required)")
	lastName := flag.String("last-name", "", "Admin last name")
	role := flag.String("role", "super_admin", "System role (super_admin, support, marketing, partner)")
	flag.Parse()

	// Validate required flags
	if *email == "" {
		fmt.Println("Usage: createadmin -email <email> -first-name <name> [-last-name <name>] [-role <role>]")
		fmt.Println("\nRoles: super_admin, support, marketing, partner")
		fmt.Println("\nExample:")
		fmt.Println("  go run cmd/createadmin/main.go -email admin@example.com -first-name Admin -role super_admin")
		os.Exit(1)
	}

	if *firstName == "" {
		fmt.Println("Error: -first-name is required")
		os.Exit(1)
	}

	// Validate role
	if !models.IsValidSystemRole(*role) {
		fmt.Printf("Error: Invalid role '%s'. Valid roles: super_admin, support, marketing, partner\n", *role)
		os.Exit(1)
	}

	// Get password securely
	fmt.Print("Enter password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		// Fallback for environments without terminal
		reader := bufio.NewReader(os.Stdin)
		password, _ := reader.ReadString('\n')
		passwordBytes = []byte(strings.TrimSpace(password))
	}
	fmt.Println()

	if len(passwordBytes) < 8 {
		fmt.Println("Error: Password must be at least 8 characters")
		os.Exit(1)
	}

	// Confirm password
	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirmBytes = []byte(strings.TrimSpace(confirm))
	}
	fmt.Println()

	if string(passwordBytes) != string(confirmBytes) {
		fmt.Println("Error: Passwords do not match")
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to local database
	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// If ADMIN_MIRROR_DB_DSN is set, route writes through the dual-write
	// wrapper so the new admin row replicates to the other env immediately.
	// Without this, a CLI-created admin would drift until the next boot-sync.
	var mirrorDB *database.DB
	if cfg.Database.AdminMirrorDSN != "" {
		mirrorDB, err = database.NewWithDSN(
			cfg.Database.AdminMirrorDSN,
			cfg.Database.MaxOpenConns,
			cfg.Database.MaxIdleConns,
			cfg.Database.ConnMaxLifetime,
		)
		if err != nil {
			log.Fatalf("Failed to connect to admin-mirror DB: %v", err)
		}
		defer mirrorDB.Close()
	}

	baseAdmin := repository.NewAdminRepo(db.DB, db.DB)
	var adminRepo repository.AdminRepository = baseAdmin
	if mirrorDB != nil {
		adminRepo = repository.NewReplicatingAdminRepo(baseAdmin, db.DB, mirrorDB.DB)
		fmt.Println("Replication ON: writes will dual-target local + mirror.")
	}
	userRepo := repository.NewUserRepo(db.DB)

	ctx := context.Background()

	// Check if email already exists in admin_users.
	// Post-00032: only admin_users matters for the createadmin CLI. We do NOT
	// promote/demote app_users rows from this CLI; if you want a parent who is
	// also an admin, create an admin row here AND register a parent at /register.
	existing, err := userRepo.GetAdminByEmail(ctx, *email)
	if err != nil {
		log.Fatalf("Failed to check existing admin: %v", err)
	}
	if existing != nil {
		fmt.Printf("User with email '%s' already exists.\n", *email)
		fmt.Print("Update their system role to ", *role, "? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
		if err := adminRepo.UpdateAdminRole(ctx, existing.ID, models.SystemRole(*role)); err != nil {
			log.Fatalf("Failed to update user: %v", err)
		}
		fmt.Printf("Successfully updated user to %s role.\n", *role)
		os.Exit(0)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	view, err := adminRepo.CreateAdminUser(ctx, *email, string(hashedPassword), *firstName, *lastName, models.SystemRole(*role))
	if err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	fmt.Println("\n====================================")
	fmt.Println("Admin user created successfully!")
	fmt.Println("====================================")
	fmt.Printf("ID:         %s\n", view.ID)
	fmt.Printf("Email:      %s\n", view.Email)
	fmt.Printf("Name:       %s %s\n", view.FirstName, view.LastName)
	fmt.Printf("Role:       %s\n", view.SystemRole)
	fmt.Println("\nYou can now log in at /admin/login")
	_ = uuid.Nil // keep uuid import used for any future need
	_ = time.Now()
}
