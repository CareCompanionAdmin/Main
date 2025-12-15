package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type familyRepo struct {
	db *sql.DB
}

func NewFamilyRepo(db *sql.DB) FamilyRepository {
	return &familyRepo{db: db}
}

func (r *familyRepo) Create(ctx context.Context, family *models.Family) error {
	query := `
		INSERT INTO families (id, name, created_by, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	family.ID = uuid.New()
	family.CreatedAt = time.Now()
	family.UpdatedAt = time.Now()
	if family.Settings == nil {
		family.Settings = models.JSONB{}
	}

	_, err := r.db.ExecContext(ctx, query,
		family.ID,
		family.Name,
		family.CreatedBy,
		family.Settings,
		family.CreatedAt,
		family.UpdatedAt,
	)
	return err
}

func (r *familyRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Family, error) {
	query := `
		SELECT id, name, created_by, settings, created_at, updated_at
		FROM families
		WHERE id = $1
	`
	family := &models.Family{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&family.ID,
		&family.Name,
		&family.CreatedBy,
		&family.Settings,
		&family.CreatedAt,
		&family.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return family, nil
}

func (r *familyRepo) Update(ctx context.Context, family *models.Family) error {
	query := `
		UPDATE families
		SET name = $2, settings = $3, updated_at = $4
		WHERE id = $1
	`
	family.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		family.ID,
		family.Name,
		family.Settings,
		family.UpdatedAt,
	)
	return err
}

func (r *familyRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM families WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *familyRepo) AddMember(ctx context.Context, membership *models.FamilyMembership) error {
	query := `
		INSERT INTO family_memberships (id, family_id, user_id, role, permissions, invited_by, invited_at, accepted_at, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	membership.ID = uuid.New()
	membership.CreatedAt = time.Now()
	membership.UpdatedAt = time.Now()
	if membership.Permissions == nil {
		membership.Permissions = models.JSONB{}
	}
	// For self-registration, mark as immediately accepted
	now := time.Now()
	membership.AcceptedAt.Time = now
	membership.AcceptedAt.Valid = true
	membership.IsActive = true

	_, err := r.db.ExecContext(ctx, query,
		membership.ID,
		membership.FamilyID,
		membership.UserID,
		membership.Role,
		membership.Permissions,
		membership.InvitedBy,
		membership.InvitedAt,
		membership.AcceptedAt,
		membership.IsActive,
		membership.CreatedAt,
		membership.UpdatedAt,
	)
	return err
}

func (r *familyRepo) RemoveMember(ctx context.Context, familyID, userID uuid.UUID) error {
	query := `DELETE FROM family_memberships WHERE family_id = $1 AND user_id = $2`
	_, err := r.db.ExecContext(ctx, query, familyID, userID)
	return err
}

func (r *familyRepo) GetMembers(ctx context.Context, familyID uuid.UUID) ([]models.FamilyMembership, error) {
	query := `
		SELECT fm.id, fm.family_id, fm.user_id, fm.role, fm.is_active, fm.created_at,
		       u.email, u.first_name, u.last_name
		FROM family_memberships fm
		JOIN users u ON u.id = fm.user_id
		WHERE fm.family_id = $1 AND fm.is_active = true
		ORDER BY fm.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.FamilyMembership
	for rows.Next() {
		var m models.FamilyMembership
		m.User = &models.User{}
		err := rows.Scan(
			&m.ID, &m.FamilyID, &m.UserID, &m.Role, &m.IsActive, &m.CreatedAt,
			&m.User.Email, &m.User.FirstName, &m.User.LastName,
		)
		if err != nil {
			return nil, err
		}
		m.User.ID = m.UserID
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *familyRepo) GetMembership(ctx context.Context, familyID, userID uuid.UUID) (*models.FamilyMembership, error) {
	query := `
		SELECT id, family_id, user_id, role, is_active, created_at
		FROM family_memberships
		WHERE family_id = $1 AND user_id = $2
	`
	m := &models.FamilyMembership{}
	err := r.db.QueryRowContext(ctx, query, familyID, userID).Scan(
		&m.ID, &m.FamilyID, &m.UserID, &m.Role, &m.IsActive, &m.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *familyRepo) GetUserFamilies(ctx context.Context, userID uuid.UUID) ([]models.FamilyMembership, error) {
	query := `
		SELECT fm.id, fm.family_id, fm.user_id, fm.role, fm.is_active, fm.created_at,
		       f.name
		FROM family_memberships fm
		JOIN families f ON f.id = fm.family_id
		WHERE fm.user_id = $1 AND fm.is_active = true
		ORDER BY fm.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []models.FamilyMembership
	for rows.Next() {
		var m models.FamilyMembership
		m.Family = &models.Family{}
		err := rows.Scan(
			&m.ID, &m.FamilyID, &m.UserID, &m.Role, &m.IsActive, &m.CreatedAt,
			&m.Family.Name,
		)
		if err != nil {
			return nil, err
		}
		m.Family.ID = m.FamilyID
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

func (r *familyRepo) UpdateMemberRole(ctx context.Context, familyID, userID uuid.UUID, role models.FamilyRole) error {
	query := `UPDATE family_memberships SET role = $3 WHERE family_id = $1 AND user_id = $2`
	_, err := r.db.ExecContext(ctx, query, familyID, userID, role)
	return err
}
