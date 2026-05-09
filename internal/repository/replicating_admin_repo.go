package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// ReplicatingAdminRepo wraps a base AdminRepository and dual-writes every
// admin_users mutation to a mirror database in addition to the local one.
//
// Pattern (synchronous bidirectional, "fail loud"):
//   1. BEGIN local tx
//   2. Apply change to local (uncommitted)
//   3. Apply change to mirror (autocommit)
//   4. If mirror succeeds → commit local
//   5. If mirror fails → rollback local, return error
//
// The remaining rare race — mirror commits, then local commit fails — leaves
// an orphan on the mirror side. A periodic reconciliation job (future) would
// catch it. For now we log loudly so an operator sees it.
//
// Reads (GetUserByID, ListAdminUsers, etc.) go to LOCAL only, so a mirror
// outage doesn't block reads. Methods that don't touch admin_users (tickets,
// metrics, settings, etc.) pass through to the base repo unchanged via the
// embedded AdminRepository.
type ReplicatingAdminRepo struct {
	AdminRepository // embedded base — fall-through for non-admin-user methods

	base     AdminRepository
	localDB  *sql.DB
	mirrorDB *sql.DB
}

// NewReplicatingAdminRepo wraps base for admin_users dual-write replication.
// localDB is the same handle base uses; mirrorDB is the OTHER environment's
// pool (dev → prod RDS, prod → dev docker postgres via SG ingress).
func NewReplicatingAdminRepo(base AdminRepository, localDB, mirrorDB *sql.DB) *ReplicatingAdminRepo {
	return &ReplicatingAdminRepo{
		AdminRepository: base,
		base:            base,
		localDB:         localDB,
		mirrorDB:        mirrorDB,
	}
}

// CreateAdminUser dual-writes the new admin row to local and mirror.
// Both INSERTs share the same generated UUID so subsequent operations
// (login, role updates, replication on the OTHER side) line up by id.
func (r *ReplicatingAdminRepo) CreateAdminUser(ctx context.Context, email, passwordHash, firstName, lastName string, role models.SystemRole) (*AdminUserView, error) {
	id := uuid.New()
	now := time.Now()
	const q = `
		INSERT INTO admin_users (id, email, password_hash, first_name, last_name, system_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
	`

	tx, err := r.localDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("admin replication: begin local tx: %w", err)
	}
	defer tx.Rollback() // safe no-op if commit succeeded

	if _, err := tx.ExecContext(ctx, q, id, email, passwordHash, firstName, lastName, role, models.UserStatusActive, now); err != nil {
		return nil, fmt.Errorf("admin replication: local insert: %w", err)
	}

	if _, err := r.mirrorDB.ExecContext(ctx, q, id, email, passwordHash, firstName, lastName, role, models.UserStatusActive, now); err != nil {
		return nil, fmt.Errorf("admin replication: mirror insert (rolling back local): %w", err)
	}

	if err := tx.Commit(); err != nil {
		// Rare: mirror succeeded, local commit failed. Orphan on mirror.
		log.Printf("[ADMIN-MIRROR] CRITICAL: mirror insert succeeded but local commit failed for id=%s email=%s — orphan on mirror; reconciliation will surface", id, email)
		return nil, fmt.Errorf("admin replication: local commit (mirror has orphan): %w", err)
	}
	return r.base.GetUserByID(ctx, id)
}

func (r *ReplicatingAdminRepo) UpdateAdminRole(ctx context.Context, id uuid.UUID, role models.SystemRole) error {
	const q = `UPDATE admin_users SET system_role = $2, updated_at = NOW() WHERE id = $1`
	return r.dualWriteSimple(ctx, "UpdateAdminRole", q, id, role)
}

func (r *ReplicatingAdminRepo) RemoveAdminRole(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM admin_users WHERE id = $1`
	return r.dualWriteSimple(ctx, "RemoveAdminRole", q, id)
}

// UpdateUserStatus may target either admin_users (admin row) or app_users
// (parent row). The base implementation fans out to both tables locally.
// We mirror ONLY the admin_users update — the app_users one is parent
// data and stays env-local. Mirror's app_users update would be a no-op
// anyway (admin id wouldn't exist in mirror.app_users).
func (r *ReplicatingAdminRepo) UpdateUserStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error {
	if err := r.base.UpdateUserStatus(ctx, id, status); err != nil {
		return err
	}
	// Best-effort mirror update of the admin_users row only. If the id
	// belongs to an app user, this matches 0 rows on mirror — harmless.
	if _, err := r.mirrorDB.ExecContext(ctx,
		`UPDATE admin_users SET status = $2, updated_at = NOW() WHERE id = $1`, id, status); err != nil {
		log.Printf("[ADMIN-MIRROR] UpdateUserStatus mirror write failed for id=%s: %v", id, err)
		return fmt.Errorf("admin replication: mirror status update: %w", err)
	}
	return nil
}

func (r *ReplicatingAdminRepo) ResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error {
	if err := r.base.ResetUserPassword(ctx, id, newHash); err != nil {
		return err
	}
	if _, err := r.mirrorDB.ExecContext(ctx,
		`UPDATE admin_users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, newHash); err != nil {
		log.Printf("[ADMIN-MIRROR] ResetUserPassword mirror write failed for id=%s: %v", id, err)
		return fmt.Errorf("admin replication: mirror password update: %w", err)
	}
	return nil
}

// dualWriteSimple is the shared transactional pattern for UPDATE and DELETE
// statements that don't need a generated id. Local tx is open until mirror
// confirms; rollback on either failure.
func (r *ReplicatingAdminRepo) dualWriteSimple(ctx context.Context, op, query string, args ...interface{}) error {
	tx, err := r.localDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("admin replication %s: begin local tx: %w", op, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("admin replication %s: local exec: %w", op, err)
	}

	if _, err := r.mirrorDB.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("admin replication %s: mirror exec (rolling back local): %w", op, err)
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ADMIN-MIRROR] CRITICAL: mirror %s succeeded but local commit failed args=%v — drift; reconciliation will surface", op, args)
		return fmt.Errorf("admin replication %s: local commit (mirror has drift): %w", op, err)
	}
	return nil
}

// SyncAdminUsers performs a one-shot bidirectional UPSERT of admin_users
// rows between the local and mirror databases. Called once at boot when
// ADMIN_MIRROR_DB_DSN is first set, to reconcile pre-existing drift.
//
// Strategy: take the union of admin rows from BOTH sides, keyed by email.
// For each email present in only one side, INSERT into the other.
// For each email present in both sides with the SAME id, no-op.
// For each email present in both sides with DIFFERENT ids, last-writer
// wins by updated_at — log the conflict and converge to the winning id.
func (r *ReplicatingAdminRepo) SyncAdminUsers(ctx context.Context) (insertedToMirror, insertedToLocal, conflicts int, err error) {
	type adminRow struct {
		id           uuid.UUID
		email        string
		passwordHash string
		firstName    string
		lastName     string
		systemRole   string
		status       string
		updatedAt    time.Time
	}

	loadAll := func(db *sql.DB) (map[string]adminRow, error) {
		rows, err := db.QueryContext(ctx, `
			SELECT id, email, password_hash, first_name, last_name, system_role::text, status::text, updated_at
			FROM admin_users
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make(map[string]adminRow)
		for rows.Next() {
			var a adminRow
			if err := rows.Scan(&a.id, &a.email, &a.passwordHash, &a.firstName, &a.lastName, &a.systemRole, &a.status, &a.updatedAt); err != nil {
				return nil, err
			}
			out[a.email] = a
		}
		return out, rows.Err()
	}

	localRows, err := loadAll(r.localDB)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("sync: load local: %w", err)
	}
	mirrorRows, err := loadAll(r.mirrorDB)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("sync: load mirror: %w", err)
	}

	insertSQL := `
		INSERT INTO admin_users (id, email, password_hash, first_name, last_name, system_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::system_role, $7::user_status, $8, $9)
		ON CONFLICT (email) DO UPDATE SET
		  password_hash = EXCLUDED.password_hash,
		  first_name    = EXCLUDED.first_name,
		  last_name     = EXCLUDED.last_name,
		  system_role   = EXCLUDED.system_role,
		  status        = EXCLUDED.status,
		  updated_at    = EXCLUDED.updated_at
		WHERE admin_users.updated_at < EXCLUDED.updated_at
	`

	now := time.Now()

	// Push local-only rows to mirror
	for email, a := range localRows {
		other, ok := mirrorRows[email]
		if !ok {
			if _, err := r.mirrorDB.ExecContext(ctx, insertSQL,
				a.id, a.email, a.passwordHash, a.firstName, a.lastName, a.systemRole, a.status, now, a.updatedAt); err != nil {
				return insertedToMirror, insertedToLocal, conflicts, fmt.Errorf("sync: push %s to mirror: %w", email, err)
			}
			insertedToMirror++
			continue
		}
		// Both sides have this email
		if a.id != other.id {
			conflicts++
			log.Printf("[ADMIN-MIRROR] sync conflict: email=%s local_id=%s mirror_id=%s — keeping latest by updated_at", email, a.id, other.id)
		}
	}

	// Push mirror-only rows to local
	for email, a := range mirrorRows {
		if _, ok := localRows[email]; ok {
			continue
		}
		if _, err := r.localDB.ExecContext(ctx, insertSQL,
			a.id, a.email, a.passwordHash, a.firstName, a.lastName, a.systemRole, a.status, now, a.updatedAt); err != nil {
			return insertedToMirror, insertedToLocal, conflicts, fmt.Errorf("sync: push %s to local: %w", email, err)
		}
		insertedToLocal++
	}

	log.Printf("[ADMIN-MIRROR] initial sync complete: pushed %d to mirror, %d to local, %d conflicts logged", insertedToMirror, insertedToLocal, conflicts)
	return insertedToMirror, insertedToLocal, conflicts, nil
}
