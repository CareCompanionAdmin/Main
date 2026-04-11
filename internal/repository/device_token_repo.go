package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// DeviceTokenRepository handles device token operations
type DeviceTokenRepository interface {
	Upsert(ctx context.Context, token *models.DeviceToken) error
	Deactivate(ctx context.Context, userID uuid.UUID, token string) error
	DeactivateAll(ctx context.Context, userID uuid.UUID) error
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error)
	DeactivateByToken(ctx context.Context, token string) error
}

type deviceTokenRepo struct {
	db *sql.DB
}

// NewDeviceTokenRepo creates a new device token repository
func NewDeviceTokenRepo(db *sql.DB) DeviceTokenRepository {
	return &deviceTokenRepo{db: db}
}

func (r *deviceTokenRepo) Upsert(ctx context.Context, token *models.DeviceToken) error {
	query := `
		INSERT INTO device_tokens (id, user_id, token, platform, device_name, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, $6, $6)
		ON CONFLICT (user_id, token) DO UPDATE SET
			platform = EXCLUDED.platform,
			device_name = EXCLUDED.device_name,
			active = true,
			updated_at = EXCLUDED.updated_at`

	now := time.Now()
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	token.Active = true
	token.CreatedAt = now
	token.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		token.ID, token.UserID, token.Token, token.Platform, token.DeviceName, now)
	return err
}

func (r *deviceTokenRepo) Deactivate(ctx context.Context, userID uuid.UUID, token string) error {
	query := `UPDATE device_tokens SET active = false, updated_at = NOW() WHERE user_id = $1 AND token = $2`
	_, err := r.db.ExecContext(ctx, query, userID, token)
	return err
}

func (r *deviceTokenRepo) DeactivateAll(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE device_tokens SET active = false, updated_at = NOW() WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *deviceTokenRepo) GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error) {
	query := `SELECT id, user_id, token, platform, COALESCE(device_name, ''), active, created_at, updated_at
		FROM device_tokens WHERE user_id = $1 AND active = true`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.DeviceToken
	for rows.Next() {
		var t models.DeviceToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Token, &t.Platform, &t.DeviceName, &t.Active, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (r *deviceTokenRepo) DeactivateByToken(ctx context.Context, token string) error {
	query := `UPDATE device_tokens SET active = false, updated_at = NOW() WHERE token = $1`
	_, err := r.db.ExecContext(ctx, query, token)
	return err
}
