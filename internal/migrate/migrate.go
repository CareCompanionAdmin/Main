// Package migrate applies SQL migrations from a directory at app startup.
//
// Why this exists: until 2026-05-03 migrations were applied by hand. Migration
// 00026 was committed to the repo but never applied to prod, silently
// 500'ing the interaction-alert handler. This package closes that gap by
// running pending migrations on every container boot, before HTTP starts.
//
// Concurrency: when ASG refreshes multiple instances, several boots can race.
// pg_advisory_lock(advisoryLockKey) serializes them — only one runs the
// migration loop, others wait then see no work to do.
//
// Multi-statement files: migrations like 00001 contain CREATE FUNCTION blocks
// with `$$ ... ; ... $$` bodies that can't be split on `;`. We bypass
// database/sql's prepared-statement path and use pgx's simple query protocol
// via Conn.Raw, which accepts multi-statement strings as one round-trip.
//
// Bootstrap: existing dev/prod databases pre-date the runner. On first boot
// against a DB whose schema_migrations table is empty but whose `users` table
// exists, we backfill all versions <= bootstrapBaseline as already-applied
// (because they clearly are — the schema reflects them). Anything newer is
// then applied normally. Bootstrap fires at most once per environment; once
// schema_migrations has any rows, the branch is dead.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"
)

// advisoryLockKey is an arbitrary stable int64 used with pg_advisory_lock to
// serialize concurrent boots. Value chosen at random; do not change without
// understanding that an in-progress migration on the old value would not
// block a new boot on the new value.
const advisoryLockKey int64 = 7234923827489

// bootstrapBaseline is the last migration version that existed before the
// runner was introduced. On first boot of an existing DB, every migration up
// to and including this one is marked applied without re-running. Bump this
// only if you add a new migration that you've ALSO applied by hand to every
// environment — otherwise just commit the migration file and let the runner
// apply it normally.
const bootstrapBaseline = "00025_past_due_since"

// Run applies any pending SQL migrations from migrationsDir. Safe to call on
// every boot; idempotent. Returns an error if any migration fails — the
// caller should treat that as fatal so a bad migration doesn't ship behind
// healthy-looking new code.
func Run(ctx context.Context, db *sql.DB, migrationsDir string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	defer func() {
		// Release with a fresh context — request ctx may already be cancelled
		// by the time we reach this defer on shutdown.
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockKey)
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	versions, err := listMigrations(migrationsDir)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		log.Printf("[migrate] no migration files found at %s — runner is a no-op", migrationsDir)
		return nil
	}

	if err := bootstrapIfNeeded(ctx, conn, versions); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	applied, err := readApplied(ctx, conn)
	if err != nil {
		return err
	}

	pending := 0
	for _, v := range versions {
		if applied[v.version] {
			continue
		}
		log.Printf("[migrate] applying %s", v.version)
		if err := applyOne(ctx, conn, v); err != nil {
			return fmt.Errorf("apply %s: %w", v.version, err)
		}
		pending++
	}
	if pending == 0 {
		log.Printf("[migrate] up to date (%d applied total)", len(applied))
	} else {
		log.Printf("[migrate] applied %d pending migration(s)", pending)
	}
	return nil
}

func bootstrapIfNeeded(ctx context.Context, conn *sql.Conn, versions []migration) error {
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		return fmt.Errorf("count schema_migrations: %w", err)
	}
	if count > 0 {
		return nil
	}
	var hasUsers bool
	if err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables
		WHERE table_schema='public' AND table_name='users')`).Scan(&hasUsers); err != nil {
		return fmt.Errorf("check users table: %w", err)
	}
	if !hasUsers {
		// Brand-new DB. Don't backfill — let the runner apply every migration
		// from 00001 forward.
		log.Printf("[migrate] empty schema — fresh DB, will apply all migrations from scratch")
		return nil
	}
	log.Printf("[migrate] empty schema_migrations + existing users table — backfilling pre-runner migrations through %s", bootstrapBaseline)
	n := 0
	for _, v := range versions {
		if v.version > bootstrapBaseline {
			break
		}
		if _, err := conn.ExecContext(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING", v.version); err != nil {
			return err
		}
		n++
	}
	log.Printf("[migrate] backfilled %d pre-runner migration(s) as applied", n)
	return nil
}

type migration struct {
	version string // filename minus ".sql"
	path    string
}

func listMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}
	out := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v := strings.TrimSuffix(e.Name(), ".sql")
		out = append(out, migration{version: v, path: filepath.Join(dir, e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func readApplied(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func applyOne(ctx context.Context, conn *sql.Conn, m migration) error {
	body, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("read %s: %w", m.path, err)
	}
	// Wrap the migration body in a transaction along with the
	// schema_migrations insert so either both land or neither does. The
	// version is interpolated as a literal because simple-protocol Exec
	// doesn't bind parameters; we control the value (it's a filename from
	// our own migrations dir) and contains only [0-9a-z_], so injection is
	// not a concern, but we still escape single quotes defensively.
	versionLiteral := strings.ReplaceAll(m.version, "'", "''")
	combined := fmt.Sprintf("BEGIN;\n%s\nINSERT INTO schema_migrations (version) VALUES ('%s');\nCOMMIT;",
		string(body), versionLiteral)

	return conn.Raw(func(driverConn any) error {
		stdConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("expected *stdlib.Conn, got %T", driverConn)
		}
		// pgx.Conn.Exec with no args uses simple query protocol, which is
		// the only path that supports multi-statement strings. If any
		// statement errors the server aborts the transaction, and pgx
		// returns the error.
		_, err := stdConn.Conn().Exec(ctx, combined)
		return err
	})
}
