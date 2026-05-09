package repository_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

func openTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://carecompanion:carecompanion@localhost:5432/carecompanion?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("dev db not reachable, skipping: %v", err)
	}
	return db
}

func TestSessionRepo_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := repository.NewSessionRepo(db)
	ctx := context.Background()

	// Pin onto a known seeded user from the Smith Test Family fixtures.
	var userID uuid.UUID
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = 'joe_parent1@test.com'`).Scan(&userID); err != nil {
		t.Skipf("test user missing, skipping: %v", err)
	}

	s := &models.Session{
		UserID:    userID,
		Kind:      models.SessionKindUser,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.ID == uuid.Nil {
		t.Fatal("Create did not assign ID")
	}
	defer db.ExecContext(ctx, "DELETE FROM sessions WHERE id = $1", s.ID)

	got, err := repo.GetByID(ctx, s.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v %v", err, got)
	}
	if got.Kind != models.SessionKindUser {
		t.Fatalf("kind = %q, want user", got.Kind)
	}
	if !got.IsActive(time.Now()) {
		t.Fatal("freshly created session should be active")
	}

	if err := repo.Revoke(ctx, s.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got2, _ := repo.GetByID(ctx, s.ID)
	if got2 == nil || got2.RevokedAt == nil {
		t.Fatal("Revoke did not set revoked_at")
	}
	if got2.IsActive(time.Now()) {
		t.Fatal("revoked session should not be active")
	}
}
