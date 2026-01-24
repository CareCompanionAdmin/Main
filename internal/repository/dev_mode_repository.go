package repository

import (
	"context"
	"database/sql"
	"time"

	"carecompanion/internal/models"

	"github.com/google/uuid"
)

// DevModeRepository defines the interface for dev mode settings
type DevModeRepository interface {
	Get(ctx context.Context) (*models.DevModeSettings, error)
	SetEnabled(ctx context.Context, enabled bool, userID uuid.UUID, allowedIP, sgRuleID string) error
	GetEnabledByUser(ctx context.Context, userID uuid.UUID) (string, error)
}

// DevModeRepo implements DevModeRepository
type DevModeRepo struct {
	db *sql.DB
}

// NewDevModeRepo creates a new DevModeRepo
func NewDevModeRepo(db *sql.DB) *DevModeRepo {
	return &DevModeRepo{db: db}
}

// Get returns the current dev mode settings
func (r *DevModeRepo) Get(ctx context.Context) (*models.DevModeSettings, error) {
	query := `
		SELECT id, is_enabled, allowed_ip, sg_rule_id, enabled_by,
		       enabled_at, disabled_at, created_at, updated_at
		FROM dev_mode_settings
		WHERE id = 'singleton'
	`

	var s models.DevModeSettings
	err := r.db.QueryRowContext(ctx, query).Scan(
		&s.ID, &s.IsEnabled, &s.AllowedIP, &s.SGRuleID, &s.EnabledBy,
		&s.EnabledAt, &s.DisabledAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SetEnabled updates the dev mode settings
func (r *DevModeRepo) SetEnabled(ctx context.Context, enabled bool, userID uuid.UUID, allowedIP, sgRuleID string) error {
	var query string
	var args []interface{}

	if enabled {
		query = `
			UPDATE dev_mode_settings
			SET is_enabled = true,
			    allowed_ip = $1,
			    sg_rule_id = $2,
			    enabled_by = $3,
			    enabled_at = $4,
			    updated_at = $4
			WHERE id = 'singleton'
		`
		args = []interface{}{allowedIP, sgRuleID, userID, time.Now()}
	} else {
		query = `
			UPDATE dev_mode_settings
			SET is_enabled = false,
			    allowed_ip = NULL,
			    sg_rule_id = NULL,
			    disabled_at = $1,
			    updated_at = $1
			WHERE id = 'singleton'
		`
		args = []interface{}{time.Now()}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetEnabledByUser returns the name of the user who enabled dev mode
func (r *DevModeRepo) GetEnabledByUser(ctx context.Context, userID uuid.UUID) (string, error) {
	query := `SELECT COALESCE(first_name || ' ' || last_name, email) FROM users WHERE id = $1`
	var name string
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}
