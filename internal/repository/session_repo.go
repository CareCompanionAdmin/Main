package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type SessionRepository interface {
	Create(ctx context.Context, s *models.Session) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Session, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	RevokeForUserKind(ctx context.Context, userID uuid.UUID, kind models.SessionKind) error
	TouchLastSeen(ctx context.Context, id uuid.UUID) error
	ListActive(ctx context.Context, kind *models.SessionKind, limit int) ([]models.Session, error)
}

type sessionRepo struct{ db *sql.DB }

func NewSessionRepo(db *sql.DB) SessionRepository { return &sessionRepo{db: db} }

func (r *sessionRepo) Create(ctx context.Context, s *models.Session) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.LastSeenAt = s.CreatedAt
	// Post-00032: sessions has admin_id + app_user_id (NOT user_id). Pick the
	// right column based on Kind. The Session model still carries a single
	// UserID field; the repo translates it to the right column.
	var adminID, appUserID *uuid.UUID
	if s.Kind == models.SessionKindAdmin {
		id := s.UserID
		adminID = &id
	} else {
		id := s.UserID
		appUserID = &id
	}
	const q = `
		INSERT INTO sessions
			(id, admin_id, app_user_id, kind, system_role, family_id, ip_at_start, user_agent,
			 user_email, user_first_name, user_last_name, family_name, env_name,
			 created_at, last_seen_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`
	_, err := r.db.ExecContext(ctx, q,
		s.ID, adminID, appUserID, s.Kind, s.SystemRole, s.FamilyID, s.IPAtStart, s.UserAgent,
		s.UserEmail, s.UserFirstName, s.UserLastName, s.FamilyName, s.EnvName,
		s.CreatedAt, s.LastSeenAt, s.ExpiresAt)
	return err
}

func (r *sessionRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Session, error) {
	// Post-00032: COALESCE admin_id + app_user_id back into UserID for the model.
	const q = `
		SELECT id, COALESCE(admin_id, app_user_id) AS user_id, kind, system_role, family_id, ip_at_start::text,
		       user_agent, created_at, last_seen_at, revoked_at, expires_at,
		       user_email, user_first_name, user_last_name, family_name, env_name
		FROM sessions WHERE id = $1`
	var s models.Session
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&s.ID, &s.UserID, &s.Kind, &s.SystemRole, &s.FamilyID, &s.IPAtStart,
		&s.UserAgent, &s.CreatedAt, &s.LastSeenAt, &s.RevokedAt, &s.ExpiresAt,
		&s.UserEmail, &s.UserFirstName, &s.UserLastName, &s.FamilyName, &s.EnvName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *sessionRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

func (r *sessionRepo) RevokeForUserKind(ctx context.Context, userID uuid.UUID, kind models.SessionKind) error {
	// Post-00032: user_id no longer exists on sessions. Use the right column
	// for the kind being revoked.
	col := "app_user_id"
	if kind == models.SessionKindAdmin {
		col = "admin_id"
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = NOW()
		 WHERE `+col+` = $1 AND kind = $2 AND revoked_at IS NULL`, userID, kind)
	return err
}

func (r *sessionRepo) TouchLastSeen(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE sessions SET last_seen_at = NOW() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

func (r *sessionRepo) ListActive(ctx context.Context, kind *models.SessionKind, limit int) ([]models.Session, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `
		SELECT id, COALESCE(admin_id, app_user_id) AS user_id, kind, system_role, family_id, ip_at_start::text,
		       user_agent, created_at, last_seen_at, revoked_at, expires_at,
		       user_email, user_first_name, user_last_name, family_name, env_name
		FROM sessions
		WHERE revoked_at IS NULL AND expires_at > NOW()`
	args := []any{}
	if kind != nil {
		q += ` AND kind = $1`
		args = append(args, *kind)
		q += ` ORDER BY last_seen_at DESC LIMIT $2`
		args = append(args, limit)
	} else {
		q += ` ORDER BY last_seen_at DESC LIMIT $1`
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Session
	for rows.Next() {
		var s models.Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.Kind, &s.SystemRole, &s.FamilyID,
			&s.IPAtStart, &s.UserAgent, &s.CreatedAt, &s.LastSeenAt, &s.RevokedAt, &s.ExpiresAt,
			&s.UserEmail, &s.UserFirstName, &s.UserLastName, &s.FamilyName, &s.EnvName); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
