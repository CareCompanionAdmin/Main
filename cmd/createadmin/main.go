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
)

func main() {
	// Command line flags
	email := flag.String("email", "", "Admin email address (required)")
	firstName := flag.String("first-name", "", "Admin first name (required)")
	lastName := flag.String("last-name", "", "Admin last name")
	role := flag.String("role", "super_admin", "System role (super_admin, support, marketing)")
	flag.Parse()

	// Validate required flags
	if *email == "" {
		fmt.Println("Usage: createadmin -email <email> -first-name <name> [-last-name <name>] [-role <role>]")
		fmt.Println("\nRoles: super_admin, support, marketing")
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
		fmt.Printf("Error: Invalid role '%s'. Valid roles: super_admin, support, marketing\n", *role)
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

	// Connect to database
	db, err := database.New(&cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Check if email already exists
	var existingID uuid.UUID
	err = db.DB.QueryRowContext(ctx, "SELECT id FROM users WHERE LOWER(email) = LOWER($1)", *email).Scan(&existingID)
	if err == nil {
		// User exists, update their system_role
		fmt.Printf("User with email '%s' already exists.\n", *email)
		fmt.Print("Update their system role to ", *role, "? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}

		_, err = db.DB.ExecContext(ctx,
			"UPDATE users SET system_role = $1, updated_at = $2 WHERE id = $3",
			*role, time.Now(), existingID,
		)
		if err != nil {
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

	// Create new admin user
	userID := uuid.New()
	now := time.Now()

	_, err = db.DB.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, first_name, last_name, system_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
	`, userID, *email, string(hashedPassword), *firstName, *lastName, *role, models.UserStatusActive, now)

	if err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	fmt.Println("\n====================================")
	fmt.Println("Admin user created successfully!")
	fmt.Println("====================================")
	fmt.Printf("ID:         %s\n", userID)
	fmt.Printf("Email:      %s\n", *email)
	fmt.Printf("Name:       %s %s\n", *firstName, *lastName)
	fmt.Printf("Role:       %s\n", *role)
	fmt.Println("\nYou can now log in at /admin/login")
}
