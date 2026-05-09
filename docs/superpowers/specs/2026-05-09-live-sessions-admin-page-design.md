# Live Sessions Admin Page — Design

**Date:** 2026-05-09
**Status:** Approved verbally; ships to dev only.
**Slice:** Roadmap feature (1).
**Depends on:** persistent-sessions slice + partner-role slice (both on dev).

## Goal

Admin page at `/admin/sessions` that shows every active session — user JWTs, admin JWTs, and SSH — across both environments, with individual + bulk kill for the local env. Foundation for "Partner can kick someone in an emergency."

## Non-goals (this slice)

- Cross-env kill. Killing prod sessions from dev (or vice versa) requires either cross-env API auth or write access to the other env's DB. Out of scope: log into the target env to kill there.
- Historical / revoked-session list.
- Per-session activity timeline.
- Session impersonation / "view as user".

## Architecture

Three data sources merged into one HTML page:

1. **Local-env JWT sessions** — `SELECT * FROM sessions WHERE revoked_at IS NULL AND expires_at > NOW()` against the local DB. Already wired (T2 of the persistent-sessions slice).
2. **Cross-env JWT sessions** — same query against a SECOND DB connection pool opened from `SESSIONS_PROD_DB_DSN` (new env var). Read-only. When the env var is empty, the cross-env section is hidden — keeps prod admin clean (prod doesn't reach back to dev).
3. **Local SSH sessions** — `DevModeService.ListSSHSessions`, which already exists. Local env only.

Why a second DB pool, not a shared SSO/RPC: matches the existing `SUPPORT_DB_DSN` pattern, no new auth layer, no inter-env network dependency. Read-only role on the prod side limits blast radius — even with the DSN, dev cannot mutate prod's `sessions`.

## Sessions table — denorm columns

Direct JOINs across env DB pools aren't possible. To render user/email/family on cross-env rows we snapshot at login:

```sql
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS user_email      TEXT,
    ADD COLUMN IF NOT EXISTS user_first_name TEXT,
    ADD COLUMN IF NOT EXISTS user_last_name  TEXT,
    ADD COLUMN IF NOT EXISTS family_name     TEXT,
    ADD COLUMN IF NOT EXISTS env_name        TEXT;
```

`env_name` comes from `cfg.App.Env` (`'development'` or `'production'`) at row insert. Backfill existing rows via JOIN to `users` + `families` (one-time at migration apply). `LoginWithContext` populates these going forward — same pattern that `system_role` already uses.

Migration: `00031_sessions_denorm_columns.sql`. Plain SQL (no goose markers) per the project's runtime-runner quirk.

## Page layout

Single template, three sections stacked. Env shown as a colored badge on every row — `dev` blue, `prod` red. Auto-refresh every 30s; manual "Refresh" button.

### Section 1 — User Sessions

If `SESSIONS_PROD_DB_DSN` is set, two side-by-side tables: "Dev" and "Production". Otherwise one table for the local env.

Columns: select checkbox, user (first + last), email, family, source IP, started (`created_at`), last seen, env badge, action.

### Section 2 — Admin Portal Sessions

Single table, all envs combined.

Columns: select checkbox, user, email, role (`system_role` snapshot), source IP, started, last seen, idle (`NOW() - last_seen_at`), user agent (truncated), env badge, action.

### Section 3 — SSH Sessions

Local env only (env var `APP_ENV` shown as a badge).

Columns: select checkbox, user, source IP, started, idle, terminal / PID, env badge, action.

## Kill behavior

### Individual kill

- JWT session, local env: existing `DELETE /api/admin/sessions/{id}` (handler already permits super_admin/support/partner).
- JWT session, cross-env: kill button replaced with a disabled button + tooltip `"Log into <env> admin to kill this session"`. No client request fires.
- SSH session: existing `POST /api/admin/super/dev-mode/kill-session`. (Already gated to super_admin via the route group; spec change: relax to super_admin/support/partner via inline check or move to `RequireSection("live_sessions")`. Pick: inline check, since it's a single endpoint and matches the kill-session pattern.)

### Bulk kill

- Checkboxes per row, "Select all (this section)" toggle, "Kill selected" button.
- Client filters out cross-env JWT rows before submitting. UI shows a count: "Killing N sessions; M cross-env rows skipped."
- New endpoint: `POST /api/admin/sessions/revoke` body `{"ids":[...]}` returning `{"revoked":N}`. Inline role check (super_admin/support/partner). Loops the existing `RevokeSession` service method per id, returns count.
- SSH bulk: loops existing kill-session endpoint client-side. (Server bulk SSH not worth a new endpoint for what is at most 5 sessions.)

## Authorization

`/admin/sessions` page (UI) and the API endpoints — both gated to `super_admin / support / partner`. Matrix entry already exists: `live_sessions: full` for those three roles.

- UI route: wrap in `RequireSection("live_sessions")`.
- API DELETE / POST endpoints: keep inline `HasAnySystemRole(...)` check (matches the kill-session pattern from the partner slice).

## Display details

- IP shown as the bare address (sessions table stores `inet`; reads use `::text` cast — already done in T2 repo).
- Idle: rendered client-side as relative ("3m ago") from `last_seen_at`.
- User agent: truncated to 60 chars with full value in `title=` attribute.
- Family for admin sessions: shown as "—" (admins don't have a family scope).
- Env badge styling matches the existing role badge in the top nav.

## Sidebar

Add a `Live Sessions` link to the Administration section in `templates/admin/layout.html`, gated by `canSee $role "live_sessions"`. Matrix already says yes for super_admin / support / partner.

## API surface added

```
GET    /api/admin/sessions/live      → JSON: { user: [...], admin: [...], ssh: [...] }
POST   /api/admin/sessions/revoke    → body: {"ids":["uuid"...]}, returns {"revoked":N}
```

(`DELETE /api/admin/sessions/{id}` already exists.)

The `live` endpoint queries:
1. Local sessions (active, not revoked, not expired) — denorm columns drive display.
2. Cross-env sessions if `SESSIONS_PROD_DB_DSN` is set.
3. SSH sessions from `DevModeService`.

Each row carries an `env` field (`'dev'` | `'production'`) and a `kind` field (`'user'` | `'admin'` | `'ssh'`).

## Repository surface

Extend `SessionRepository`:

```go
ListLive(ctx, kind models.SessionKind, env string) ([]models.Session, error)
```

Wire a SECOND `*sessionRepo` instance pointing at the cross-env pool when configured. Naming: `repos.Session` (local) and `repos.SessionProd` (cross-env, may be nil).

## Out of scope reminders

Cross-env kill, history, impersonation, mobile sessions filtering. SSH on prod admin (no SSH service runs there). Any "online users count" badge on the dashboard.

## Risk

- A misconfigured `SESSIONS_PROD_DB_DSN` (e.g., wrong creds, network unreachable) must not break the page. Cross-env read errors render "Production sessions unavailable: <err>" inline, not a 500.
- The denorm columns can drift if a user changes their email after login. Acceptable: session row is a snapshot; on next login the new email lands. Documented in the migration comment.

## Rollout

Dev only. Migration `00031` runs via the runner on next boot. After dev verification (kill round-trip, cross-env display when DSN set, sidebar visibility per role), revisit shipping to prod.
