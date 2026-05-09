package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// userRepo handles BOTH admin_users and app_users at the application layer.
//
// After migration 00032 split-users-table, the underlying schema is two
// disjoint tables — admin_users (rows with system_role) and app_users (rows
// without). A read-only VIEW called `users` UNIONs them so ID-based reads
// continue to work without changing every call site. Writes here always go
// to app_users; admin user CRUD lives on the admin repository.
//
// For email-based lookups callers MUST use GetAdminByEmail or GetAppByEmail
// — the unified view's GetByEmail can return 2 rows once an email exists in
// both tables, which is exactly the scenario this migration enables.
type userRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) UserRepository {
	return &userRepo{db: db}
}

// Create inserts a new APP user (parent / caregiver / doctor / etc.).
// Admin users are created via the admin repository (writes to admin_users).
func (r *userRepo) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO app_users (id, email, password_hash, first_name, last_name, phone, timezone, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	user.ID = uuid.New()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	if user.Status == "" {
		user.Status = models.UserStatusActive
	}

	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.FirstName,
		user.LastName,
		user.Phone,
		user.Timezone,
		user.Status,
		user.CreatedAt,
		user.UpdatedAt,
	)
	return err
}

// GetByID reads from the unified `users` view. UUIDs are unique across both
// tables, so this returns 0 or 1 row regardless of kind.
func (r *userRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, phone, timezone, time_format, status,
		       system_role, email_verified_at, last_login_at, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.Timezone,
		&user.TimeFormat,
		&user.Status,
		&user.SystemRole,
		&user.EmailVerifiedAt,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetByEmail is a kind-AGNOSTIC lookup. It can return either an admin or an
// app row. Once an email exists in BOTH tables (post-migration feature), the
// view UNIONs them and this method returns whichever the planner emits first
// — which is non-deterministic. Callers that need a deterministic result MUST
// use GetAdminByEmail or GetAppByEmail. Kept for backward-compat with
// pre-migration callers that only ever ran in single-kind environments.
func (r *userRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, phone, timezone, time_format, status,
		       system_role, email_verified_at, last_login_at, created_at, updated_at
		FROM users
		WHERE LOWER(email) = LOWER($1)
		LIMIT 1
	`
	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.Timezone,
		&user.TimeFormat,
		&user.Status,
		&user.SystemRole,
		&user.EmailVerifiedAt,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetAdminByEmail looks up an ADMIN user by email. Used by the admin login flow.
func (r *userRepo) GetAdminByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, status,
		       system_role, email_verified_at, last_login_at, created_at, updated_at
		FROM admin_users
		WHERE LOWER(email) = LOWER($1)
	`
	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.Status,
		&user.SystemRole,
		&user.EmailVerifiedAt,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetAppByEmail looks up an APP user (parent/caregiver/etc.) by email. Used
// by the parent login flow and by registration's duplicate-email check.
func (r *userRepo) GetAppByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, phone, timezone, time_format, status,
		       email_verified_at, last_login_at, created_at, updated_at
		FROM app_users
		WHERE LOWER(email) = LOWER($1)
	`
	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.Timezone,
		&user.TimeFormat,
		&user.Status,
		&user.EmailVerifiedAt,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// Update updates an APP user. Admin user updates go through the admin repo.
func (r *userRepo) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE app_users
		SET email = $2, first_name = $3, last_name = $4, phone = $5, timezone = $6, time_format = $7, updated_at = $8
		WHERE id = $1
	`
	user.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.FirstName,
		user.LastName,
		user.Phone,
		user.Timezone,
		user.TimeFormat,
		user.UpdatedAt,
	)
	return err
}

// UpdateStatus updates either kind. We don't know which table holds the row
// from the id alone, so update both and trust that exactly one matches.
// Same pattern for UpdateLastLogin and Delete.
func (r *userRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error {
	now := time.Now()
	if _, err := r.db.ExecContext(ctx, `UPDATE app_users SET status = $2, updated_at = $3 WHERE id = $1`, id, status, now); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE admin_users SET status = $2, updated_at = $3 WHERE id = $1`, id, status, now)
	return err
}

func (r *userRepo) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	if _, err := r.db.ExecContext(ctx, `UPDATE app_users SET last_login_at = $2, updated_at = $2 WHERE id = $1`, id, now); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = $2, updated_at = $2 WHERE id = $1`, id, now)
	return err
}

func (r *userRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM app_users WHERE id = $1`, id); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_users WHERE id = $1`, id)
	return err
}
