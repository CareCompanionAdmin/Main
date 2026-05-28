package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// RoleRepository persists the custom_roles + custom_role_permissions tables
// on the main DB. Built-in roles are NOT stored here — they live in
// auth/perm.go's hardcoded matrix.
type RoleRepository interface {
	List(ctx context.Context) ([]models.CustomRole, error)
	Get(ctx context.Context, id uuid.UUID) (*models.CustomRole, error)
	GetByName(ctx context.Context, name string) (*models.CustomRole, error)
	Create(ctx context.Context, r *models.CustomRole) error
	Update(ctx context.Context, r *models.CustomRole) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountAdminsByRoleName(ctx context.Context, name string) (int, []string, error)
	GetLevel(ctx context.Context, name, section string) (string, bool, error)
}

type roleRepo struct {
	db *sql.DB
}

func NewRoleRepo(db *sql.DB) RoleRepository {
	return &roleRepo{db: db}
}

func (r *roleRepo) List(ctx context.Context) ([]models.CustomRole, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, display_name, description, created_at, COALESCE(created_by_email,''), updated_at
		   FROM custom_roles ORDER BY display_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CustomRole
	for rows.Next() {
		var role models.CustomRole
		if err := rows.Scan(&role.ID, &role.Name, &role.DisplayName, &role.Description,
			&role.CreatedAt, &role.CreatedByEmail, &role.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Hydrate permissions for each role in one extra query
	for i := range out {
		perms, err := r.listPermissions(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Permissions = perms
	}
	return out, nil
}

func (r *roleRepo) Get(ctx context.Context, id uuid.UUID) (*models.CustomRole, error) {
	var role models.CustomRole
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, description, created_at, COALESCE(created_by_email,''), updated_at
		   FROM custom_roles WHERE id = $1`, id).
		Scan(&role.ID, &role.Name, &role.DisplayName, &role.Description,
			&role.CreatedAt, &role.CreatedByEmail, &role.UpdatedAt)
	if err != nil {
		return nil, err
	}
	role.Permissions, err = r.listPermissions(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) GetByName(ctx context.Context, name string) (*models.CustomRole, error) {
	var role models.CustomRole
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, description, created_at, COALESCE(created_by_email,''), updated_at
		   FROM custom_roles WHERE name = $1`, name).
		Scan(&role.ID, &role.Name, &role.DisplayName, &role.Description,
			&role.CreatedAt, &role.CreatedByEmail, &role.UpdatedAt)
	if err != nil {
		return nil, err
	}
	role.Permissions, err = r.listPermissions(ctx, role.ID)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) listPermissions(ctx context.Context, roleID uuid.UUID) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT section, level FROM custom_role_permissions WHERE role_id = $1`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var section, level string
		if err := rows.Scan(&section, &level); err != nil {
			return nil, err
		}
		out[section] = level
	}
	return out, rows.Err()
}

func (r *roleRepo) Create(ctx context.Context, role *models.CustomRole) error {
	if role.ID == uuid.Nil {
		role.ID = uuid.New()
	}
	now := time.Now()
	role.CreatedAt = now
	role.UpdatedAt = now

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO custom_roles (id, name, display_name, description, created_at, created_by_email, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7)`,
		role.ID, role.Name, role.DisplayName, role.Description, role.CreatedAt, role.CreatedByEmail, role.UpdatedAt); err != nil {
		return fmt.Errorf("insert custom_roles: %w", err)
	}
	if err := upsertPermissions(ctx, tx, role.ID, role.Permissions); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *roleRepo) Update(ctx context.Context, role *models.CustomRole) error {
	role.UpdatedAt = time.Now()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// We intentionally don't allow renaming (name change) — it would break
	// admin_users.system_role assignment. Only display_name + description
	// + permissions are mutable.
	if _, err := tx.ExecContext(ctx,
		`UPDATE custom_roles
		    SET display_name = $1, description = $2, updated_at = $3
		  WHERE id = $4`,
		role.DisplayName, role.Description, role.UpdatedAt, role.ID); err != nil {
		return fmt.Errorf("update custom_roles: %w", err)
	}
	// Replace permissions wholesale: delete then upsert.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM custom_role_permissions WHERE role_id = $1`, role.ID); err != nil {
		return fmt.Errorf("clear permissions: %w", err)
	}
	if err := upsertPermissions(ctx, tx, role.ID, role.Permissions); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertPermissions(ctx context.Context, tx *sql.Tx, roleID uuid.UUID, perms map[string]string) error {
	for section, level := range perms {
		if level == "" || level == "none" {
			continue
		}
		if level != "read" && level != "write" {
			return fmt.Errorf("invalid level %q for section %q", level, section)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO custom_role_permissions (role_id, section, level)
			 VALUES ($1,$2,$3)`,
			roleID, section, level); err != nil {
			return fmt.Errorf("insert permission %s=%s: %w", section, level, err)
		}
	}
	return nil
}

func (r *roleRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM custom_roles WHERE id = $1`, id)
	return err
}

// CountAdminsByRoleName returns the count of admin_users still assigned
// to this role plus up to 10 of their emails, so the UI can show what
// must be reassigned before deletion is safe.
func (r *roleRepo) CountAdminsByRoleName(ctx context.Context, name string) (int, []string, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admin_users WHERE system_role = $1`, name).Scan(&count); err != nil {
		return 0, nil, err
	}
	if count == 0 {
		return 0, nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT email FROM admin_users WHERE system_role = $1 ORDER BY email LIMIT 10`, name)
	if err != nil {
		return count, nil, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return count, nil, err
		}
		emails = append(emails, e)
	}
	return count, emails, rows.Err()
}

// GetLevel returns the access level for (role name, section), or empty +
// false if the role doesn't exist or has no permission row for that
// section. Pure SQL — no cache. The service wraps this with caching.
func (r *roleRepo) GetLevel(ctx context.Context, name, section string) (string, bool, error) {
	var level string
	err := r.db.QueryRowContext(ctx,
		`SELECT p.level
		   FROM custom_roles r
		   JOIN custom_role_permissions p ON p.role_id = r.id
		  WHERE r.name = $1 AND p.section = $2`, name, section).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return level, true, nil
}
