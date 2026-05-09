# Live Sessions Admin Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `/admin/sessions` showing all active user JWT, admin JWT, and SSH sessions across both envs, with individual + bulk kill restricted to the local env.

**Architecture:** Three data sources merged in one page — local env's `sessions` table, optional cross-env `sessions` via `SESSIONS_PROD_DB_DSN` (read-only mirror of `SUPPORT_DB_DSN`), and SSH sessions via `DevModeService`. Sessions get denorm columns at login (`user_email`, `user_first_name`, `user_last_name`, `family_name`, `env_name`) so cross-env rows render without cross-DB JOINs. Kill is same-env only — UI hides the kill button on cross-env rows.

**Tech Stack:** Go 1.24, PostgreSQL, Chi router, server-rendered HTML + minimal client JS.

**Spec:** `docs/superpowers/specs/2026-05-09-live-sessions-admin-page-design.md`

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `migrations/00031_sessions_denorm.sql` | **Create** | Add `user_email`, `user_first_name`, `user_last_name`, `family_name`, `env_name` columns to `sessions`. Backfill existing rows via JOIN to `users` + `families`. |
| `internal/models/session.go` | **Modify** | Add the five new fields to `Session` struct. |
| `internal/repository/session_repo.go` | **Modify** | Update `Create` (insert denorm columns), `GetByID` / `ListActive` (select denorm columns). |
| `internal/config/config.go` | **Modify** | Add `SessionsProdDSN` field bound to env `SESSIONS_PROD_DB_DSN`. |
| `internal/repository/repository.go` | **Modify** | Add `SessionProd SessionRepository` field (nil-able) to `Repositories`. New repo points at second pool when configured. |
| `cmd/server/main.go` | **Modify** | Open optional second DB pool from `SESSIONS_PROD_DB_DSN`; pass to `NewRepositories`. |
| `internal/repository/repository.go` | **Modify** | Update `NewRepositories` signature to accept `sessionsProdDB *sql.DB` (nil-safe). |
| `internal/service/auth_service.go` | **Modify** | `LoginWithContext` populates the five denorm columns from `user.*` + `family.Name` + `cfg.App.Env`. |
| `internal/service/services.go` | **Modify** | Pass `cfg.App.Env` into the auth service so `LoginWithContext` can stamp it. |
| `internal/service/live_sessions_service.go` | **Create** | `LiveSnapshot{User, Admin, SSH []LiveSessionRow}` aggregator. Reads from local repo + (optional) cross-env repo + DevMode SSH. Each row carries `Env` and `Kind` fields. Cross-env failure becomes `CrossEnvError string` on the snapshot, never a 500. |
| `internal/handler/admin/sessions_handlers.go` | **Create** | `LiveSessionsPage` (UI), `ListLiveSessions` (JSON), `BulkRevokeSessions` (JSON), `KillSSHSessionJSON` (JSON wrapper around DevModeService.KillSession). All gated to super_admin/support/partner via inline check. |
| `internal/handler/admin/routes.go` | **Modify** | Wire `GET /admin/sessions` (UI), `GET /api/admin/sessions/live`, `POST /api/admin/sessions/revoke`, `POST /api/admin/sessions/ssh/kill`. |
| `templates/admin/sessions.html` | **Create** | Three sections: User (split dev/prod), Admin (combined), SSH (local). Auto-refresh JS, env badges, bulk select. |
| `templates/admin/layout.html` | **Modify** | Add `Live Sessions` link to Administration section gated by `canSee $role "live_sessions"`. |

---

## Task 1: Migration — sessions denorm columns

**Files:** Create `migrations/00031_sessions_denorm.sql`

- [ ] **Step 1: Write the migration**

```sql
-- Migration: 00031_sessions_denorm.sql
-- Adds denormalized user/family/env columns to sessions so the Live Sessions
-- admin page can render rows from a cross-env DB pool (where JOINs to the
-- local users/families tables aren't possible). Populated at login time
-- going forward; existing rows are backfilled via JOIN here.
--
-- Rollback (run by hand if needed):
--   ALTER TABLE sessions
--       DROP COLUMN env_name,
--       DROP COLUMN family_name,
--       DROP COLUMN user_last_name,
--       DROP COLUMN user_first_name,
--       DROP COLUMN user_email;

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS user_email      TEXT,
    ADD COLUMN IF NOT EXISTS user_first_name TEXT,
    ADD COLUMN IF NOT EXISTS user_last_name  TEXT,
    ADD COLUMN IF NOT EXISTS family_name     TEXT,
    ADD COLUMN IF NOT EXISTS env_name        TEXT;

COMMENT ON COLUMN sessions.user_email      IS 'Snapshot of users.email at login. Drifts if user changes email.';
COMMENT ON COLUMN sessions.user_first_name IS 'Snapshot of users.first_name at login.';
COMMENT ON COLUMN sessions.user_last_name  IS 'Snapshot of users.last_name at login.';
COMMENT ON COLUMN sessions.family_name     IS 'Snapshot of families.name at login (null for kind=admin).';
COMMENT ON COLUMN sessions.env_name        IS 'Environment that issued the session: development | production. Set from APP_ENV.';

-- Backfill existing rows.
UPDATE sessions s
SET user_email      = u.email,
    user_first_name = u.first_name,
    user_last_name  = u.last_name
FROM users u
WHERE s.user_id = u.id
  AND s.user_email IS NULL;

UPDATE sessions s
SET family_name = f.name
FROM families f
WHERE s.family_id = f.id
  AND s.family_name IS NULL;

-- env_name backfilled from a default; admins running this migration on prod
-- will see a mix of dev/prod existing rows but those get cleaned up as
-- sessions expire (8h) and replace with new (env_name-stamped) ones.
UPDATE sessions
SET env_name = 'development'
WHERE env_name IS NULL;
```

- [ ] **Step 2: Apply via psql (and let the runner record it on next boot)**

```bash
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -f migrations/00031_sessions_denorm.sql
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
    -c "\d sessions" | grep -E "user_email|user_first_name|user_last_name|family_name|env_name"
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
    -c "SELECT count(*) AS total, count(user_email) AS with_email FROM sessions;"
```

Expected: 5 column lines visible; `total` row count appears with `with_email` matching it (or close — only mismatched rows are sessions where the user was deleted with ON DELETE CASCADE in flight; should be 0 in practice).

- [ ] **Step 3: Commit**

```bash
git add migrations/00031_sessions_denorm.sql
git commit -m "migrate: add denorm columns to sessions for cross-env Live Sessions display"
```

---

## Task 2: Update Session model

**Files:** Modify `internal/models/session.go`

- [ ] **Step 1: Read current shape**

```bash
grep -n "type Session struct" -A20 internal/models/session.go
```

- [ ] **Step 2: Add the 5 fields**

After `UserAgent NullString`, before `CreatedAt time.Time`, add:

```go
UserEmail     NullString `json:"user_email,omitempty"`
UserFirstName NullString `json:"user_first_name,omitempty"`
UserLastName  NullString `json:"user_last_name,omitempty"`
FamilyName    NullString `json:"family_name,omitempty"`
EnvName       NullString `json:"env_name,omitempty"`
```

- [ ] **Step 3: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/models/session.go
git commit -m "models: add denorm fields to Session for cross-env display"
```

---

## Task 3: Update SessionRepository — Create, GetByID, ListActive

**Files:** Modify `internal/repository/session_repo.go`

- [ ] **Step 1: Update Create**

Replace the existing `Create` method body. New version inserts the 5 denorm columns:

```go
func (r *sessionRepo) Create(ctx context.Context, s *models.Session) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.LastSeenAt = s.CreatedAt
	const q = `
		INSERT INTO sessions
			(id, user_id, kind, system_role, family_id, ip_at_start, user_agent,
			 user_email, user_first_name, user_last_name, family_name, env_name,
			 created_at, last_seen_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`
	_, err := r.db.ExecContext(ctx, q,
		s.ID, s.UserID, s.Kind, s.SystemRole, s.FamilyID, s.IPAtStart, s.UserAgent,
		s.UserEmail, s.UserFirstName, s.UserLastName, s.FamilyName, s.EnvName,
		s.CreatedAt, s.LastSeenAt, s.ExpiresAt)
	return err
}
```

- [ ] **Step 2: Update GetByID + ListActive SELECT lists**

For each method's SELECT, add the 5 columns at the end (and the matching Scan args):

```go
const q = `
    SELECT id, user_id, kind, system_role, family_id, ip_at_start::text,
           user_agent, created_at, last_seen_at, revoked_at, expires_at,
           user_email, user_first_name, user_last_name, family_name, env_name
    FROM sessions WHERE id = $1`
```

Scan adds:

```go
&s.UserEmail, &s.UserFirstName, &s.UserLastName, &s.FamilyName, &s.EnvName
```

Same change for `ListActive` (SELECT + Scan).

- [ ] **Step 3: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 4: Run existing repo test**

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./internal/repository/ -run TestSessionRepo -v
```

Expected: PASS (the existing test doesn't assert denorm fields, so it should keep passing).

- [ ] **Step 5: Commit**

```bash
git add internal/repository/session_repo.go
git commit -m "session repo: read+write denorm columns"
```

---

## Task 4: Auth service — populate denorm columns at login

**Files:**
- Modify: `internal/service/auth_service.go`
- Modify: `internal/service/services.go`

- [ ] **Step 1: Add `appEnv` field to AuthService struct**

In `auth_service.go`, add to the `AuthService` struct:

```go
type AuthService struct {
	userRepo     repository.UserRepository
	familyRepo   repository.FamilyRepository
	sessionRepo  repository.SessionRepository
	sessionCache *SessionCache
	redis        *database.Redis
	jwtConfig    *config.JWTConfig
	emailService *EmailService
	appURL       string
	appEnv       string
	subSvc       *SubscriptionService
}
```

Update `NewAuthService` to take `appEnv string`:

```go
func NewAuthService(
	userRepo repository.UserRepository,
	familyRepo repository.FamilyRepository,
	sessionRepo repository.SessionRepository,
	sessionCache *SessionCache,
	redis *database.Redis,
	jwtConfig *config.JWTConfig,
	emailService *EmailService,
	appURL string,
	appEnv string,
) *AuthService {
	return &AuthService{
		userRepo: userRepo, familyRepo: familyRepo,
		sessionRepo: sessionRepo, sessionCache: sessionCache,
		redis: redis, jwtConfig: jwtConfig,
		emailService: emailService, appURL: appURL, appEnv: appEnv,
	}
}
```

- [ ] **Step 2: Populate denorm columns in LoginWithContext**

Inside `LoginWithContext`, after the existing `if user.HasSystemRole() { ... }` block where SystemRole / FamilyID / IPAtStart / UserAgent get set, add:

```go
sess.UserEmail = models.NullString{NullString: sql.NullString{String: user.Email, Valid: user.Email != ""}}
sess.UserFirstName = models.NullString{NullString: sql.NullString{String: user.FirstName, Valid: user.FirstName != ""}}
sess.UserLastName = models.NullString{NullString: sql.NullString{String: user.LastName, Valid: user.LastName != ""}}
if s.appEnv != "" {
    sess.EnvName = models.NullString{NullString: sql.NullString{String: s.appEnv, Valid: true}}
}
if familyID != uuid.Nil {
    if family, ferr := s.familyRepo.GetByID(ctx, familyID); ferr == nil && family != nil {
        sess.FamilyName = models.NullString{NullString: sql.NullString{String: family.Name, Valid: family.Name != ""}}
    }
}
```

If `familyRepo.GetByID` doesn't exist with that signature, run:

```bash
grep -n "func.*FamilyRepository\|GetByID\|GetFamily" internal/repository/family_repo.go | head
```

Adjust the call to whatever the project uses (e.g., `Get` or `GetFamily`). The fallback if family lookup fails: leave `sess.FamilyName` zero — the column is nullable.

- [ ] **Step 3: Update services.go**

```bash
grep -n "NewAuthService(" internal/service/services.go
```

Change the call to pass `cfg.App.Env`:

```go
Auth: NewAuthService(repos.User, repos.Family, repos.Session, sessionCache, redis, &cfg.JWT, emailService, cfg.App.URL, cfg.App.Env),
```

- [ ] **Step 4: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/service/auth_service.go internal/service/services.go
git commit -m "auth: stamp env_name + user/family denorm at login"
```

---

## Task 5: Config + main.go — optional cross-env DB pool

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/repository/repository.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add `SessionsProdDSN` to config**

```bash
grep -n "SupportDSN" internal/config/config.go | head
```

In the `DatabaseConfig` struct, add (right after `SupportDSN`):

```go
// SessionsProdDSN, when non-empty, opens a SECOND read-only connection to
// the prod sessions table so the dev Live Sessions admin page can show
// prod sessions alongside dev. Empty in prod (cross-env display is a
// dev-side affordance only).
SessionsProdDSN string
```

In the loader (around line 161 where SupportDSN is set):

```go
SessionsProdDSN: getEnv("SESSIONS_PROD_DB_DSN", ""),
```

- [ ] **Step 2: Add SessionProd to Repositories**

In `internal/repository/repository.go`, update the struct + constructor:

```go
type Repositories struct {
    // ... existing ...
    Session     SessionRepository
    SessionProd SessionRepository // nil when SESSIONS_PROD_DB_DSN unset
}
```

Update `NewRepositories` signature:

```go
func NewRepositories(db, supportDB *sql.DB, sessionsProdDB *sql.DB) *Repositories {
    repos := &Repositories{
        // ... existing fields ...
        Session: NewSessionRepo(db),
    }
    if sessionsProdDB != nil {
        repos.SessionProd = NewSessionRepo(sessionsProdDB)
    }
    return repos
}
```

- [ ] **Step 3: Open second pool in main.go**

Right after the existing `supportDB := db.DB; if cfg.Database.SupportDSN != "" { ... }` block, add:

```go
// Optional cross-env sessions pool (read-only display).
var sessionsProdDB *sql.DB
if cfg.Database.SessionsProdDSN != "" {
    s, err := database.NewWithDSN(
        cfg.Database.SessionsProdDSN,
        cfg.Database.MaxOpenConns,
        cfg.Database.MaxIdleConns,
        cfg.Database.ConnMaxLifetime,
    )
    if err != nil {
        log.Printf("[SESSIONS] cross-env pool init failed (%v) — continuing without it", err)
    } else {
        defer s.Close()
        sessionsProdDB = s.DB
        log.Println("Connected to cross-env sessions pool (SESSIONS_PROD_DB_DSN set)")
    }
}
```

(Note: this `log.Printf` + `continue without it` pattern instead of `log.Fatalf` — a misconfigured cross-env DSN must NOT prevent the dev server from starting.)

Update the `NewRepositories` call:

```go
repos := repository.NewRepositories(db.DB, supportDB, sessionsProdDB)
```

- [ ] **Step 4: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/repository/repository.go cmd/server/main.go
git commit -m "config+main: optional cross-env sessions pool via SESSIONS_PROD_DB_DSN"
```

---

## Task 6: Live sessions service — aggregator

**Files:** Create `internal/service/live_sessions_service.go`

- [ ] **Step 1: Create the file**

```go
package service

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// LiveSessionRow is a unified row across the three session sources, so the
// admin page renders a single template structure regardless of where the
// row came from.
type LiveSessionRow struct {
	ID            string    `json:"id"`              // session UUID for JWT, "ssh:<tty>" for SSH
	Kind          string    `json:"kind"`            // "user" | "admin" | "ssh"
	Env           string    `json:"env"`             // "development" | "production"
	UserEmail     string    `json:"user_email,omitempty"`
	UserFirstName string    `json:"user_first_name,omitempty"`
	UserLastName  string    `json:"user_last_name,omitempty"`
	FamilyName    string    `json:"family_name,omitempty"`
	SystemRole    string    `json:"system_role,omitempty"`
	IPAtStart     string    `json:"ip_at_start,omitempty"`
	UserAgent     string    `json:"user_agent,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	TTY           string    `json:"tty,omitempty"`   // SSH only
	IsLocalEnv    bool      `json:"is_local_env"`    // controls kill-button visibility client-side
}

// LiveSnapshot is what the page handler returns to the template / API client.
// CrossEnvError is non-empty when the cross-env pool is configured but the
// query failed; the page renders a banner and still shows local + SSH rows.
type LiveSnapshot struct {
	Users         []LiveSessionRow `json:"users"`
	Admins        []LiveSessionRow `json:"admins"`
	SSH           []LiveSessionRow `json:"ssh"`
	LocalEnv      string           `json:"local_env"`
	CrossEnvShown bool             `json:"cross_env_shown"`
	CrossEnvError string           `json:"cross_env_error,omitempty"`
}

type LiveSessionsService struct {
	localRepo     repository.SessionRepository
	prodRepo      repository.SessionRepository // may be nil
	devModeSvc    *DevModeService              // may be nil (e.g., on prod where DevMode isn't initialized)
	localEnv      string
}

func NewLiveSessionsService(localRepo, prodRepo repository.SessionRepository, devModeSvc *DevModeService, localEnv string) *LiveSessionsService {
	return &LiveSessionsService{localRepo: localRepo, prodRepo: prodRepo, devModeSvc: devModeSvc, localEnv: localEnv}
}

func (s *LiveSessionsService) Snapshot(ctx context.Context) LiveSnapshot {
	out := LiveSnapshot{LocalEnv: s.localEnv, CrossEnvShown: s.prodRepo != nil}

	// Local sessions.
	userKind := models.SessionKindUser
	adminKind := models.SessionKindAdmin
	localUsers, err := s.localRepo.ListActive(ctx, &userKind, 500)
	if err != nil {
		log.Printf("[LIVE_SESSIONS] local user list failed: %v", err)
	}
	for _, r := range localUsers {
		out.Users = append(out.Users, toRow(r, true))
	}
	localAdmins, err := s.localRepo.ListActive(ctx, &adminKind, 500)
	if err != nil {
		log.Printf("[LIVE_SESSIONS] local admin list failed: %v", err)
	}
	for _, r := range localAdmins {
		out.Admins = append(out.Admins, toRow(r, true))
	}

	// Cross-env sessions (read-only).
	if s.prodRepo != nil {
		prodUsers, perr := s.prodRepo.ListActive(ctx, &userKind, 500)
		if perr != nil {
			out.CrossEnvError = perr.Error()
		} else {
			for _, r := range prodUsers {
				out.Users = append(out.Users, toRow(r, false))
			}
			prodAdmins, _ := s.prodRepo.ListActive(ctx, &adminKind, 500)
			for _, r := range prodAdmins {
				out.Admins = append(out.Admins, toRow(r, false))
			}
		}
	}

	// SSH sessions (local only).
	if s.devModeSvc != nil {
		ssh, err := s.devModeSvc.ListSSHSessions(ctx)
		if err != nil {
			log.Printf("[LIVE_SESSIONS] ssh list failed: %v", err)
		} else {
			for _, sh := range ssh {
				out.SSH = append(out.SSH, LiveSessionRow{
					ID:        "ssh:" + sh.TTY,
					Kind:      "ssh",
					Env:       s.localEnv,
					UserEmail: sh.User,
					IPAtStart: sh.SourceIP,
					CreatedAt: sh.LoginTime,
					TTY:       sh.TTY,
					IsLocalEnv: true,
				})
			}
		}
	}

	return out
}

func toRow(r models.Session, isLocal bool) LiveSessionRow {
	row := LiveSessionRow{
		ID:         r.ID.String(),
		Kind:       string(r.Kind),
		CreatedAt:  r.CreatedAt,
		LastSeenAt: r.LastSeenAt,
		IsLocalEnv: isLocal,
	}
	if r.UserEmail.Valid {     row.UserEmail = r.UserEmail.String }
	if r.UserFirstName.Valid { row.UserFirstName = r.UserFirstName.String }
	if r.UserLastName.Valid {  row.UserLastName = r.UserLastName.String }
	if r.FamilyName.Valid {    row.FamilyName = r.FamilyName.String }
	if r.SystemRole.Valid {    row.SystemRole = r.SystemRole.String }
	if r.IPAtStart.Valid {     row.IPAtStart = r.IPAtStart.String }
	if r.UserAgent.Valid {     row.UserAgent = r.UserAgent.String }
	if r.EnvName.Valid {       row.Env = r.EnvName.String }
	return row
}

// RevokeBulk loops RevokeSession over a list of UUIDs (all assumed to belong
// to the LOCAL env — caller filters cross-env IDs out before calling).
func (s *LiveSessionsService) RevokeBulk(ctx context.Context, ids []uuid.UUID, auth *AuthService) (revoked int) {
	for _, id := range ids {
		if err := auth.RevokeSession(ctx, id); err == nil {
			revoked++
		}
	}
	return
}
```

If `models.SSHSession` field names differ (e.g., `User` vs `Username`, `LoginTime` vs `Started`), inspect:

```bash
grep -n "type SSHSession\|TTY\|User\b\|SourceIP\|LoginTime" internal/models/dev_mode.go
```

and adapt the field references inside the `for _, sh := range ssh` loop.

- [ ] **Step 2: Wire into services.go**

In `internal/service/services.go`, after the existing service constructions (around the AuthService line), add:

```go
LiveSessions: NewLiveSessionsService(repos.Session, repos.SessionProd, nil, cfg.App.Env),
```

(`nil` for devModeSvc as a placeholder — DevModeService is constructed elsewhere; we'll wire it after creation. Alternative: do this AFTER the DevMode service is constructed and pass it directly.)

```bash
grep -n "DevModeService\b\|devModeService\b\|NewDevModeService" internal/service/services.go internal/service/dev_mode_service.go | head
```

If `DevModeService` is a package-level singleton (`var devModeService *DevModeService` style), pass `service.GetDevModeService()` or whatever accessor exists; otherwise wire after construction.

The struct field in `Services`:

```go
LiveSessions *LiveSessionsService
```

- [ ] **Step 3: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/service/live_sessions_service.go internal/service/services.go
git commit -m "service: live sessions aggregator (local + cross-env + SSH)"
```

---

## Task 7: Sessions API + UI handlers

**Files:**
- Create: `internal/handler/admin/sessions_handlers.go`

- [ ] **Step 1: Create the file**

```go
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// allowKill returns true for super_admin / support / partner — the three
// roles approved to revoke sessions. Mirrors the inline check used by
// RevokeSession.
func (h *Handler) allowKill(claims *service.AuthClaims) bool {
	if claims == nil {
		return false
	}
	return claims.HasAnySystemRole(
		models.SystemRoleSuperAdmin,
		models.SystemRoleSupport,
		models.SystemRolePartner,
	)
}

// LiveSessionsPage renders /admin/sessions.
func (h *Handler) LiveSessionsPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	snap := h.services.LiveSessions.Snapshot(r.Context())

	tmpl, err := parseTemplates("layout.html", "sessions.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data := AdminPageData{
		Title: "Live Sessions",
		CurrentUser: AdminUser{
			ID: claims.UserID, Email: claims.Email,
			FirstName: claims.FirstName, SystemRole: string(claims.SystemRole),
		},
		Data: snap,
	}
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// ListLiveSessions returns the snapshot as JSON for the auto-refresh poll.
func (h *Handler) ListLiveSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if !h.allowKill(claims) {
		middleware.JSONError(w, "Forbidden", http.StatusForbidden)
		return
	}
	snap := h.services.LiveSessions.Snapshot(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

// BulkRevokeSessions accepts {"ids":["uuid",...]} and revokes each. The
// service's RevokeSession is called per-id; failures are counted but do not
// abort the loop. Returns {"revoked":N}.
func (h *Handler) BulkRevokeSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if !h.allowKill(claims) {
		middleware.JSONError(w, "Forbidden", http.StatusForbidden)
		return
	}
	var body struct{ IDs []string `json:"ids"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.JSONError(w, "Invalid body", http.StatusBadRequest)
		return
	}
	parsed := make([]uuid.UUID, 0, len(body.IDs))
	for _, idStr := range body.IDs {
		if id, err := uuid.Parse(idStr); err == nil {
			parsed = append(parsed, id)
		}
	}
	revoked := h.services.LiveSessions.RevokeBulk(r.Context(), parsed, h.services.Auth)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"revoked": revoked})
}

// KillSSHSessionJSON is a JSON wrapper around DevModeService.KillSession for
// the Live Sessions page (the existing form-encoded handler redirects to
// /admin/development which is wrong here).
func (h *Handler) KillSSHSessionJSON(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if !h.allowKill(claims) {
		middleware.JSONError(w, "Forbidden", http.StatusForbidden)
		return
	}
	if devModeService == nil {
		middleware.JSONError(w, "DevMode service not configured", http.StatusInternalServerError)
		return
	}
	var body struct{ TTY string `json:"tty"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TTY == "" {
		middleware.JSONError(w, "tty required", http.StatusBadRequest)
		return
	}
	if err := devModeService.KillSession(r.Context(), body.TTY); err != nil {
		middleware.JSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

If the `Handler` type doesn't expose `services *service.Services` (e.g., it's a different field name), adjust accordingly:

```bash
grep -n "type Handler struct\|services\b" internal/handler/admin/handlers.go | head
```

- [ ] **Step 2: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean. Most likely first failures: `h.services` field name, or `models.SystemRolePartner` import (already exists per partner slice).

- [ ] **Step 3: Commit**

```bash
git add internal/handler/admin/sessions_handlers.go
git commit -m "admin: live sessions handlers (page, list JSON, bulk revoke, SSH kill)"
```

---

## Task 8: Wire routes

**Files:** Modify `internal/handler/admin/routes.go`

- [ ] **Step 1: Add UI route**

Inside the protected UI group (the one at line ~251 with `RequireAnyAdminRole`), add a new section group:

```go
// Live Sessions (Partner=full, super_admin/support=full)
r.Group(func(r chi.Router) {
    r.Use(middleware.RequireSection("live_sessions"))
    r.Get("/sessions", h.LiveSessionsPage)
})
```

- [ ] **Step 2: Add API routes**

In `Routes()` (the `/api/admin` router) at the top level (next to the existing `r.Delete("/sessions/{sessionID}", h.RevokeSession)`), add:

```go
// Live Sessions JSON + bulk + SSH kill — inline role check inside each
// handler (super_admin / support / partner) so the routes can sit at the
// admin top level alongside the existing single-session DELETE.
r.Get("/sessions/live", h.ListLiveSessions)
r.Post("/sessions/revoke", h.BulkRevokeSessions)
r.Post("/sessions/ssh/kill", h.KillSSHSessionJSON)
```

- [ ] **Step 3: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/handler/admin/routes.go
git commit -m "admin: wire /admin/sessions UI + /api/admin/sessions/{live,revoke,ssh/kill}"
```

---

## Task 9: Sessions page template

**Files:** Create `templates/admin/sessions.html`

- [ ] **Step 1: Create the template**

```html
{{define "content"}}
{{$snap := .Data}}
<div class="max-w-7xl">
  <div class="flex items-center justify-between mb-6">
    <h1 class="text-2xl font-bold text-gray-900">Live Sessions</h1>
    <div class="flex items-center gap-2">
      <span class="text-xs text-gray-500" id="last-refresh">just now</span>
      <button onclick="refreshSessions()" class="px-3 py-1 bg-indigo-600 text-white text-sm rounded hover:bg-indigo-700">Refresh</button>
    </div>
  </div>

  {{if $snap.CrossEnvError}}
  <div class="mb-4 p-3 bg-yellow-50 border border-yellow-200 rounded text-yellow-800 text-sm">
    Production sessions unavailable: {{$snap.CrossEnvError}}
  </div>
  {{end}}

  <!-- USER SESSIONS -->
  <section class="mb-8" id="user-section">
    <h2 class="text-lg font-semibold mb-2">User Sessions</h2>
    <div class="bg-white rounded shadow overflow-hidden">
      <div class="px-4 py-2 border-b flex items-center justify-between bg-gray-50">
        <label class="text-xs"><input type="checkbox" onclick="toggleAll('users', this.checked)"> Select all</label>
        <button onclick="killSelected('users')" class="text-xs px-2 py-1 bg-red-600 text-white rounded hover:bg-red-700">Kill selected</button>
      </div>
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-xs text-gray-500 uppercase">
          <tr>
            <th class="px-3 py-2 text-left w-8"></th>
            <th class="px-3 py-2 text-left">User</th>
            <th class="px-3 py-2 text-left">Email</th>
            <th class="px-3 py-2 text-left">Family</th>
            <th class="px-3 py-2 text-left">Source IP</th>
            <th class="px-3 py-2 text-left">Started</th>
            <th class="px-3 py-2 text-left">Last seen</th>
            <th class="px-3 py-2 text-left">Env</th>
            <th class="px-3 py-2 text-right">Action</th>
          </tr>
        </thead>
        <tbody id="user-rows">
          {{range $snap.Users}}
          <tr data-id="{{.ID}}" data-local="{{.IsLocalEnv}}" data-kind="user">
            <td class="px-3 py-2">{{if .IsLocalEnv}}<input type="checkbox" class="row-check users-check" value="{{.ID}}">{{end}}</td>
            <td class="px-3 py-2">{{.UserFirstName}} {{.UserLastName}}</td>
            <td class="px-3 py-2 text-gray-600">{{.UserEmail}}</td>
            <td class="px-3 py-2">{{.FamilyName}}</td>
            <td class="px-3 py-2 font-mono text-xs">{{.IPAtStart}}</td>
            <td class="px-3 py-2 text-xs text-gray-600">{{.CreatedAt.Format "Jan 2 15:04"}}</td>
            <td class="px-3 py-2 text-xs text-gray-600">{{.LastSeenAt.Format "Jan 2 15:04"}}</td>
            <td class="px-3 py-2"><span class="px-2 py-0.5 text-xs rounded {{if eq .Env "production"}}bg-red-100 text-red-800{{else}}bg-blue-100 text-blue-800{{end}}">{{.Env}}</span></td>
            <td class="px-3 py-2 text-right">
              {{if .IsLocalEnv}}
              <button onclick="killOne('{{.ID}}', 'user')" class="text-xs px-2 py-1 bg-red-600 text-white rounded hover:bg-red-700">Kill</button>
              {{else}}
              <span class="text-xs text-gray-400" title="Log into {{.Env}} admin to kill this session">cross-env</span>
              {{end}}
            </td>
          </tr>
          {{else}}
          <tr><td colspan="9" class="px-3 py-6 text-center text-gray-400 text-sm">No active user sessions.</td></tr>
          {{end}}
        </tbody>
      </table>
    </div>
  </section>

  <!-- ADMIN SESSIONS -->
  <section class="mb-8" id="admin-section">
    <h2 class="text-lg font-semibold mb-2">Admin Portal Sessions</h2>
    <div class="bg-white rounded shadow overflow-hidden">
      <div class="px-4 py-2 border-b flex items-center justify-between bg-gray-50">
        <label class="text-xs"><input type="checkbox" onclick="toggleAll('admins', this.checked)"> Select all</label>
        <button onclick="killSelected('admins')" class="text-xs px-2 py-1 bg-red-600 text-white rounded hover:bg-red-700">Kill selected</button>
      </div>
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-xs text-gray-500 uppercase">
          <tr>
            <th class="px-3 py-2 text-left w-8"></th>
            <th class="px-3 py-2 text-left">User</th>
            <th class="px-3 py-2 text-left">Email</th>
            <th class="px-3 py-2 text-left">Role</th>
            <th class="px-3 py-2 text-left">Source IP</th>
            <th class="px-3 py-2 text-left">Started</th>
            <th class="px-3 py-2 text-left">Last seen</th>
            <th class="px-3 py-2 text-left">User agent</th>
            <th class="px-3 py-2 text-left">Env</th>
            <th class="px-3 py-2 text-right">Action</th>
          </tr>
        </thead>
        <tbody id="admin-rows">
          {{range $snap.Admins}}
          <tr data-id="{{.ID}}" data-local="{{.IsLocalEnv}}" data-kind="admin">
            <td class="px-3 py-2">{{if .IsLocalEnv}}<input type="checkbox" class="row-check admins-check" value="{{.ID}}">{{end}}</td>
            <td class="px-3 py-2">{{.UserFirstName}} {{.UserLastName}}</td>
            <td class="px-3 py-2 text-gray-600">{{.UserEmail}}</td>
            <td class="px-3 py-2">{{.SystemRole}}</td>
            <td class="px-3 py-2 font-mono text-xs">{{.IPAtStart}}</td>
            <td class="px-3 py-2 text-xs text-gray-600">{{.CreatedAt.Format "Jan 2 15:04"}}</td>
            <td class="px-3 py-2 text-xs text-gray-600">{{.LastSeenAt.Format "Jan 2 15:04"}}</td>
            <td class="px-3 py-2 text-xs text-gray-500 truncate max-w-xs" title="{{.UserAgent}}">{{.UserAgent}}</td>
            <td class="px-3 py-2"><span class="px-2 py-0.5 text-xs rounded {{if eq .Env "production"}}bg-red-100 text-red-800{{else}}bg-blue-100 text-blue-800{{end}}">{{.Env}}</span></td>
            <td class="px-3 py-2 text-right">
              {{if .IsLocalEnv}}
              <button onclick="killOne('{{.ID}}', 'admin')" class="text-xs px-2 py-1 bg-red-600 text-white rounded hover:bg-red-700">Kill</button>
              {{else}}
              <span class="text-xs text-gray-400" title="Log into {{.Env}} admin to kill this session">cross-env</span>
              {{end}}
            </td>
          </tr>
          {{else}}
          <tr><td colspan="10" class="px-3 py-6 text-center text-gray-400 text-sm">No active admin sessions.</td></tr>
          {{end}}
        </tbody>
      </table>
    </div>
  </section>

  <!-- SSH SESSIONS -->
  <section class="mb-8" id="ssh-section">
    <h2 class="text-lg font-semibold mb-2">SSH Sessions <span class="text-sm text-gray-400 font-normal">({{$snap.LocalEnv}} only)</span></h2>
    <div class="bg-white rounded shadow overflow-hidden">
      <div class="px-4 py-2 border-b flex items-center justify-between bg-gray-50">
        <label class="text-xs"><input type="checkbox" onclick="toggleAll('ssh', this.checked)"> Select all</label>
        <button onclick="killSelectedSSH()" class="text-xs px-2 py-1 bg-red-600 text-white rounded hover:bg-red-700">Kill selected</button>
      </div>
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-xs text-gray-500 uppercase">
          <tr>
            <th class="px-3 py-2 text-left w-8"></th>
            <th class="px-3 py-2 text-left">User</th>
            <th class="px-3 py-2 text-left">Source IP</th>
            <th class="px-3 py-2 text-left">Started</th>
            <th class="px-3 py-2 text-left">Terminal</th>
            <th class="px-3 py-2 text-left">Env</th>
            <th class="px-3 py-2 text-right">Action</th>
          </tr>
        </thead>
        <tbody id="ssh-rows">
          {{range $snap.SSH}}
          <tr data-id="{{.ID}}" data-tty="{{.TTY}}" data-kind="ssh">
            <td class="px-3 py-2"><input type="checkbox" class="row-check ssh-check" value="{{.TTY}}"></td>
            <td class="px-3 py-2">{{.UserEmail}}</td>
            <td class="px-3 py-2 font-mono text-xs">{{.IPAtStart}}</td>
            <td class="px-3 py-2 text-xs text-gray-600">{{.CreatedAt.Format "Jan 2 15:04"}}</td>
            <td class="px-3 py-2 font-mono text-xs">{{.TTY}}</td>
            <td class="px-3 py-2"><span class="px-2 py-0.5 text-xs rounded bg-blue-100 text-blue-800">{{.Env}}</span></td>
            <td class="px-3 py-2 text-right">
              <button onclick="killSSH('{{.TTY}}')" class="text-xs px-2 py-1 bg-red-600 text-white rounded hover:bg-red-700">Kill</button>
            </td>
          </tr>
          {{else}}
          <tr><td colspan="7" class="px-3 py-6 text-center text-gray-400 text-sm">No active SSH sessions.</td></tr>
          {{end}}
        </tbody>
      </table>
    </div>
  </section>
</div>

<script>
const SECTIONS = { users: 'users-check', admins: 'admins-check', ssh: 'ssh-check' };

function toggleAll(section, checked) {
  document.querySelectorAll('.' + SECTIONS[section]).forEach(c => c.checked = checked);
}

function selectedIds(section) {
  return Array.from(document.querySelectorAll('.' + SECTIONS[section] + ':checked')).map(c => c.value);
}

async function killOne(id, kind) {
  if (!confirm('Kill this session?')) return;
  const resp = await fetch('/api/admin/sessions/' + id, { method: 'DELETE', credentials: 'same-origin' });
  if (!resp.ok && resp.status !== 204) {
    alert('Failed to kill session: ' + resp.status);
    return;
  }
  await refreshSessions();
}

async function killSelected(section) {
  const ids = selectedIds(section);
  if (ids.length === 0) return;
  if (!confirm('Kill ' + ids.length + ' session(s)?')) return;
  const resp = await fetch('/api/admin/sessions/revoke', {
    method: 'POST', credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ids: ids }),
  });
  if (resp.ok) {
    const j = await resp.json();
    alert('Revoked ' + j.revoked + ' session(s)');
    await refreshSessions();
  } else {
    alert('Bulk revoke failed: ' + resp.status);
  }
}

async function killSSH(tty) {
  if (!confirm('Kill SSH session on ' + tty + '?')) return;
  const resp = await fetch('/api/admin/sessions/ssh/kill', {
    method: 'POST', credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tty: tty }),
  });
  if (resp.ok || resp.status === 204) {
    await refreshSessions();
  } else {
    alert('Failed to kill SSH session: ' + resp.status);
  }
}

async function killSelectedSSH() {
  const ttys = selectedIds('ssh');
  if (ttys.length === 0) return;
  if (!confirm('Kill ' + ttys.length + ' SSH session(s)?')) return;
  for (const t of ttys) {
    await fetch('/api/admin/sessions/ssh/kill', {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tty: t }),
    });
  }
  await refreshSessions();
}

async function refreshSessions() {
  // Simple full-page reload — keeps the template logic single-source-of-truth.
  // 30s auto-refresh; manual button hits the same path.
  window.location.reload();
}

setInterval(refreshSessions, 30000);
</script>
{{end}}
```

- [ ] **Step 2: Add Live Sessions sidebar link**

Edit `templates/admin/layout.html`. In the Administration section's block, add (placed right after the Admin Users link):

```html
{{if canSee $role "live_sessions"}}
<a href="/admin/sessions" class="block px-3 py-2 rounded hover:bg-gray-100">Live Sessions</a>
{{end}}
```

(Place this inside the existing `{{if or (canSee ... "admin_users") ...}}` wrapper that gates the section header — extend the OR list to include `(canSee $role "live_sessions")`.)

Specifically change the existing:

```html
{{if or (canSee $role "admin_users") (or (canSee $role "system_settings") (or (canSee $role "audit_log") (canSee $role "version_log")))}}
```

to:

```html
{{if or (canSee $role "admin_users") (or (canSee $role "system_settings") (or (canSee $role "audit_log") (or (canSee $role "version_log") (canSee $role "live_sessions"))))}}
```

- [ ] **Step 3: Build + smoke**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add templates/admin/sessions.html templates/admin/layout.html
git commit -m "admin/ui: live sessions page + sidebar link"
```

---

## Task 10: Dev verification

**Files:** none (manual verification)

- [ ] **Step 1: Rebuild + restart**

```bash
export PATH=$PATH:/usr/local/go/bin && go build -buildvcs=false -o bin/carecompanion-new ./cmd/server
sudo systemctl stop carecompanion && mv bin/carecompanion-new bin/carecompanion && sudo systemctl start carecompanion && sleep 4
sudo journalctl -u carecompanion -n 30 --no-pager | grep -E "migrate|server on" | tail
```

Expected: `[migrate] applying 00031_sessions_denorm` then `applied 1 pending migration(s)`.

- [ ] **Step 2: Login a fresh user to populate denorm columns**

```bash
curl -s -i -c /tmp/cookies-user.txt -X POST http://localhost:8090/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"joe_parent1@test.com","password":"TestPass1!"}' | head -3
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
    -c "SELECT user_email, user_first_name, family_name, env_name FROM sessions ORDER BY created_at DESC LIMIT 1;"
```

Expected: latest row populated with email, first name, family, env_name='development'.

- [ ] **Step 3: Login a fresh admin (super_admin)**

```bash
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -c "UPDATE users SET system_role = 'super_admin' WHERE email = 'joe_parent2@test.com';"
curl -s -i -c /tmp/cookies-super.txt -L -X POST http://localhost:8090/admin/login \
  -d "email=joe_parent2@test.com&password=TestPass1!" | grep -E "HTTP/"
```

Expected: 303 to /admin/dashboard.

- [ ] **Step 4: GET /api/admin/sessions/live**

```bash
curl -s -b /tmp/cookies-super.txt http://localhost:8090/api/admin/sessions/live | jq '{users:(.users|length), admins:(.admins|length), ssh:(.ssh|length), local_env, cross_env_shown}'
```

Expected: at least 1 user, 1+ admins, ssh count 0+, local_env="development", cross_env_shown=false (no DSN configured).

- [ ] **Step 5: Render the UI**

```bash
curl -sb /tmp/cookies-super.txt http://localhost:8090/admin/sessions -o /tmp/page.html -w "ui_http=%{http_code} bytes=%{size_download}\n"
grep -c "Live Sessions" /tmp/page.html
grep -c "user_first_name\|admin-rows\|ssh-rows" /tmp/page.html
```

Expected: ui_http=200, bytes > 5000, "Live Sessions" appears at least once, table rows render.

- [ ] **Step 6: Bulk revoke**

```bash
USER_SID=$(PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -tAc "SELECT id FROM sessions WHERE kind='user' AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1;")
echo "ID: $USER_SID"
curl -s -b /tmp/cookies-super.txt -X POST http://localhost:8090/api/admin/sessions/revoke \
    -H "Content-Type: application/json" \
    -d "{\"ids\":[\"$USER_SID\"]}" | jq
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
    -c "SELECT id, revoked_at IS NOT NULL AS killed FROM sessions WHERE id='$USER_SID';"
```

Expected: response `{"revoked":1}`, DB shows killed=t.

- [ ] **Step 7: Sidebar visible to Partner**

```bash
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -c "UPDATE users SET system_role = 'partner' WHERE email = 'joe@workmaninsurancegroup.com';"
curl -s -i -c /tmp/cookies-partner.txt -L -X POST http://localhost:8090/admin/login \
  -d "email=joe@workmaninsurancegroup.com&password=TestPass1!" | grep "HTTP/"
curl -sb /tmp/cookies-partner.txt http://localhost:8090/admin/dashboard | grep -c "Live Sessions"
curl -sb /tmp/cookies-partner.txt -o /dev/null -w "partner_sessions_page=%{http_code}\n" http://localhost:8090/admin/sessions
```

Expected: 303 redirect on login; "Live Sessions" appears 1 time in sidebar; sessions page returns 200.

- [ ] **Step 8: Marketing role does NOT see Live Sessions**

```bash
curl -s -i -c /tmp/cookies-mkt.txt -L -X POST http://localhost:8090/admin/login \
  -d "email=market@test.com&password=TestPass1!" | grep "HTTP/"
curl -sb /tmp/cookies-mkt.txt http://localhost:8090/admin/dashboard | grep -c "Live Sessions"
curl -sb /tmp/cookies-mkt.txt -o /dev/null -w "mkt_sessions_page=%{http_code}\n" http://localhost:8090/admin/sessions
```

Expected: "Live Sessions" appears 0 times; sessions page returns 403.

- [ ] **Step 9: Cleanup test role assignments**

```bash
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion <<SQL
UPDATE users SET system_role = NULL WHERE email = 'joe_parent2@test.com';
UPDATE users SET system_role = 'partner' WHERE email = 'joe@workmaninsurancegroup.com';
SQL
```

(Keeps Partner role on joe@workmaninsurancegroup.com per the previous slice; reverts the temporary super_admin promotion.)

- [ ] **Step 10: Notes**

If any step deviated, capture specifics. We do NOT ship to prod from this slice.

---

## Self-Review

**1. Spec coverage**
- Migration adds denorm columns + backfill → Task 1
- Session model fields → Task 2
- Repo Create + reads honor denorm → Task 3
- LoginWithContext stamps env_name + user/family → Task 4
- Optional cross-env pool config + main.go wiring → Task 5
- Aggregator service (local + cross-env + SSH) → Task 6
- Page handler + JSON list + bulk revoke + SSH kill → Task 7
- Routes wired (UI + API) → Task 8
- Page template + sidebar link → Task 9
- Verification → Task 10

**2. Placeholders** — none. Each step contains exact code or exact commands.

**3. Type consistency**
- `LiveSessionRow` / `LiveSnapshot` / `LiveSessionsService` defined in Task 6, consumed in Tasks 7+9.
- `models.Session` denorm fields named identically across Tasks 2, 3, 4 (`UserEmail`, `UserFirstName`, `UserLastName`, `FamilyName`, `EnvName`).
- `repos.Session` (local) vs `repos.SessionProd` (cross-env, may be nil) — declared in Task 5, consumed in Task 6.
- Endpoint paths consistent: `/api/admin/sessions/live`, `/api/admin/sessions/revoke`, `/api/admin/sessions/ssh/kill`, `/api/admin/sessions/{id}` (DELETE existing).

**4. Risks**
- Task 4 may need a tweak if `familyRepo.GetByID` doesn't have that exact signature — fallback documented inline.
- Task 6 references `SSHSession` field names that may differ — fallback documented inline.
- The page template's auto-refresh uses full `window.location.reload()` rather than fetching JSON and re-rendering. Simpler, slightly heavier. Acceptable for this slice.
- Misconfigured `SESSIONS_PROD_DB_DSN` is logged-not-fatal in Task 5 — ensures dev still boots.
