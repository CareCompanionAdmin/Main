package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BetaInvitation is one row of beta_invitations — the lifecycle of a single
// would-be tester from "admin entered their email" through "added to the
// External Beta Testers group in App Store Connect".
type BetaInvitation struct {
	ID                  uuid.UUID  `json:"id"`
	Email               string     `json:"email"`
	InvitedBy           uuid.UUID  `json:"invited_by"`
	Token               uuid.UUID  `json:"token"`
	Status              string     `json:"status"`
	AppleID             string     `json:"apple_id,omitempty"`
	AppleFirstName      string     `json:"apple_first_name,omitempty"`
	AppleLastName       string     `json:"apple_last_name,omitempty"`
	InvitedAt           time.Time  `json:"invited_at"`
	AppleIDCollectedAt  *time.Time `json:"apple_id_collected_at,omitempty"`
	AddedToTestFlightAt *time.Time `json:"added_to_testflight_at,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	Notes               string     `json:"notes,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	// Joined convenience fields (not in beta_invitations).
	InvitedByEmail string `json:"invited_by_email,omitempty"`
	InvitedByName  string `json:"invited_by_name,omitempty"`
}

// BetaInvitationRepository is the DB layer for beta invitations. Cross-cutting
// orchestration (sending emails, calling App Store Connect) lives in services.
type BetaInvitationRepository interface {
	Create(ctx context.Context, inv *BetaInvitation) error
	GetByID(ctx context.Context, id uuid.UUID) (*BetaInvitation, error)
	GetByToken(ctx context.Context, token uuid.UUID) (*BetaInvitation, error)
	GetByEmail(ctx context.Context, email string) (*BetaInvitation, error)
	List(ctx context.Context) ([]BetaInvitation, error)

	UpdateAppleID(ctx context.Context, id uuid.UUID, appleID, firstName, lastName string) error
	MarkAddedToTestFlight(ctx context.Context, id uuid.UUID) error
	MarkError(ctx context.Context, id uuid.UUID, errMsg string) error
}

type betaInvitationRepo struct {
	db *sql.DB
}

// NewBetaInvitationRepo wires the DB.
func NewBetaInvitationRepo(db *sql.DB) BetaInvitationRepository {
	return &betaInvitationRepo{db: db}
}

const betaSelectColumns = `
    bi.id, bi.email, bi.invited_by, bi.token, bi.status,
    COALESCE(bi.apple_id,''), COALESCE(bi.apple_first_name,''), COALESCE(bi.apple_last_name,''),
    bi.invited_at, bi.apple_id_collected_at, bi.added_to_testflight_at,
    COALESCE(bi.last_error,''), COALESCE(bi.notes,''),
    bi.created_at, bi.updated_at,
    COALESCE(u.email,''),
    COALESCE(NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''), '')
`

const betaFromJoins = `
    FROM beta_invitations bi
    LEFT JOIN users u ON u.id = bi.invited_by
`

func (r *betaInvitationRepo) scan(row interface {
	Scan(...interface{}) error
}) (*BetaInvitation, error) {
	inv := &BetaInvitation{}
	err := row.Scan(
		&inv.ID, &inv.Email, &inv.InvitedBy, &inv.Token, &inv.Status,
		&inv.AppleID, &inv.AppleFirstName, &inv.AppleLastName,
		&inv.InvitedAt, &inv.AppleIDCollectedAt, &inv.AddedToTestFlightAt,
		&inv.LastError, &inv.Notes,
		&inv.CreatedAt, &inv.UpdatedAt,
		&inv.InvitedByEmail, &inv.InvitedByName,
	)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (r *betaInvitationRepo) Create(ctx context.Context, inv *BetaInvitation) error {
	q := `
        INSERT INTO beta_invitations (email, invited_by, notes)
        VALUES ($1, $2, NULLIF($3, ''))
        RETURNING id, token, status, invited_at, created_at, updated_at
    `
	return r.db.QueryRowContext(ctx, q, strings.TrimSpace(inv.Email), inv.InvitedBy, inv.Notes).
		Scan(&inv.ID, &inv.Token, &inv.Status, &inv.InvitedAt, &inv.CreatedAt, &inv.UpdatedAt)
}

func (r *betaInvitationRepo) GetByID(ctx context.Context, id uuid.UUID) (*BetaInvitation, error) {
	q := "SELECT " + betaSelectColumns + betaFromJoins + " WHERE bi.id = $1"
	return r.scan(r.db.QueryRowContext(ctx, q, id))
}

func (r *betaInvitationRepo) GetByToken(ctx context.Context, token uuid.UUID) (*BetaInvitation, error) {
	q := "SELECT " + betaSelectColumns + betaFromJoins + " WHERE bi.token = $1"
	return r.scan(r.db.QueryRowContext(ctx, q, token))
}

func (r *betaInvitationRepo) GetByEmail(ctx context.Context, email string) (*BetaInvitation, error) {
	q := "SELECT " + betaSelectColumns + betaFromJoins + " WHERE LOWER(bi.email) = LOWER($1)"
	return r.scan(r.db.QueryRowContext(ctx, q, strings.TrimSpace(email)))
}

func (r *betaInvitationRepo) List(ctx context.Context) ([]BetaInvitation, error) {
	q := "SELECT " + betaSelectColumns + betaFromJoins + " ORDER BY bi.created_at DESC"
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BetaInvitation
	for rows.Next() {
		inv, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func (r *betaInvitationRepo) UpdateAppleID(ctx context.Context, id uuid.UUID, appleID, firstName, lastName string) error {
	_, err := r.db.ExecContext(ctx, `
        UPDATE beta_invitations
        SET apple_id = $2,
            apple_first_name = NULLIF($3, ''),
            apple_last_name = NULLIF($4, ''),
            apple_id_collected_at = COALESCE(apple_id_collected_at, NOW()),
            status = CASE WHEN status = 'invited' THEN 'apple_id_collected'::beta_invitation_status ELSE status END,
            last_error = NULL
        WHERE id = $1
    `, id, strings.TrimSpace(appleID), strings.TrimSpace(firstName), strings.TrimSpace(lastName))
	return err
}

func (r *betaInvitationRepo) MarkAddedToTestFlight(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
        UPDATE beta_invitations
        SET status = 'added_to_testflight',
            added_to_testflight_at = NOW(),
            last_error = NULL
        WHERE id = $1
    `, id)
	return err
}

func (r *betaInvitationRepo) MarkError(ctx context.Context, id uuid.UUID, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `
        UPDATE beta_invitations
        SET status = 'error',
            last_error = $2
        WHERE id = $1
    `, id, errMsg)
	return err
}
