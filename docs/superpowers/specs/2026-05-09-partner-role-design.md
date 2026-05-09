# Partner Role — Design

**Date:** 2026-05-09
**Status:** Approved verbally; ships to dev only.
**Slice:** Roadmap feature (5).
**Depends on:** Persistent sessions slice (already on dev).
**Unblocks:** Live Sessions admin UI (feature 1) — Partner is one of the roles allowed to kill sessions.

## Goal

Add a fourth admin system role, `partner`, with the permission matrix Bryan locked. The matrix lives as Go data so adding future roles is a single-column edit and the sidebar/middleware/createadmin all read from the same source of truth.

## Permission matrix (locked 2026-05-09)

| Section              | super_admin | support | marketing | partner |
|----------------------|-------------|---------|-----------|---------|
| Dashboard            | full        | full    | full      | full    |
| Tickets              | full        | full    | read      | full    |
| Users                | full        | full    | none      | full    |
| Families             | full        | full    | none      | full    |
| Metrics Dashboard    | full        | none    | full      | full    |
| Copy & Materials     | full        | none    | full      | read    |
| Beta Program         | full        | none    | full      | full    |
| Bounty Program       | full        | none    | full      | read    |
| Promo Codes          | full        | none    | read      | read    |
| Infrastructure Status| full        | none    | none      | read    |
| Error Logs           | full        | none    | none      | read    |
| Development Mode     | full        | none    | none      | none    |
| Product Roadmap      | full        | none    | none      | full    |
| Financials           | full        | none    | none      | full    |
| Subscriptions        | full        | none    | none      | full    |
| Admin Users          | full        | none    | none      | read    |
| System Settings      | full        | none    | none      | none    |
| Audit Log            | full        | none    | none      | none    |
| Version Log          | full        | none    | none      | read    |
| Live Sessions        | full        | full    | none      | full    |

**Levels:** `none` (no access), `read` (GET only), `write` (mutations that don't affect other users), `full` (any mutation including affecting other users). Partner never has `write` in this matrix — the read-vs-not split via HTTP method is sufficient. `support` and `marketing` columns are reverse-engineered from the existing route gates so the matrix stays a complete picture.

## Non-goals (this slice)

- An admin UI for granting Partner. Provision via `cmd/createadmin` or SQL, same as other roles today.
- Per-row record-level permissions. The matrix is section-level.
- Audit-log entries for Partner actions specifically. Existing audit hooks already capture admin actions and continue to fire.

## Design

### Source of truth

A single Go file: `internal/auth/perm.go`. Two exported items:

```go
// Level is "none" | "read" | "write" | "full". Comparable via levelRank.
type Level string

// Matrix returns the access level for (role, section). If section is unknown,
// returns "none" — fail-closed.
func Matrix(role models.SystemRole, section string) Level
```

A `Sections` slice exposes the canonical section-key list (used by the sidebar template helper). The matrix is hard-coded in this file as a `map[string]map[models.SystemRole]Level`.

### Middleware

`internal/middleware/section.go`:

```go
func RequireSection(section string) func(http.Handler) http.Handler
```

Behavior:
1. If claims missing → 401.
2. If `claims.IsSuperAdmin()` → pass. Super admin bypasses the matrix entirely (a missing matrix entry can never lock them out).
3. Map HTTP method to required level: `GET/HEAD/OPTIONS → read`; `POST/PUT/PATCH → write`; `DELETE → full`. (Most admin POST endpoints in this codebase are full-impact; `write` and `full` collapse to the same gate for non-super_admin in practice.)
4. Lookup `auth.Matrix(claims.SystemRole, section)`. If `levelRank(matrix) < levelRank(required)` → 403 with reason `section_<name>_denied`.

### Route changes

Each existing super-admin route group either:
- Keeps `RequireSuperAdmin()` if the section is super-admin-only (DevMode, SystemSettings, AuditLog).
- Switches to `RequireSection("section_key")` so Partner (and any future role) is gated by the matrix.

For mixed groups (e.g., `/super` currently mixes Promo Codes, Beta, Bounty, Roadmap, etc.), the existing block is split into per-section sub-routes each gated by their own `RequireSection` call. Sub-routes that should stay super-admin-only stay under `RequireSuperAdmin()`.

The `support` middleware (whatever route group serves Tickets/Users/Families today) already permits both super_admin and support. We replace it with `RequireSection("tickets")` etc. — the matrix says super_admin/support/partner all have full there, so the route-group behavior is identical for the existing two roles.

### Kill-session endpoint

`internal/handler/admin/handlers.go::RevokeSession` currently checks `claims.HasAnySystemRole(SuperAdmin, Support)`. Append `Partner` to the allowed list. (When the bulk endpoint ships with the Live Sessions UI, it will use `RequireSection("live_sessions")` from this slice's foundation.)

### Sidebar

`templates/admin/layout.html` currently uses inline `{{if eq .CurrentUser.SystemRole "super_admin"}}` checks per section. Replace with calls to a template helper:

```go
// In handlers/admin/template_funcs.go (new file or appended to existing helpers).
"canSee": func(role models.SystemRole, section string) bool {
    return auth.Matrix(role, section) != "none"
}
```

Template usage:

```html
{{if canSee .CurrentUser.SystemRole "tickets"}}
  <a href="/admin/tickets">Tickets</a>
{{end}}
```

Read-only sections render with the existing "(Read Only)" affordance:

```html
{{$lvl := matrixLevel .CurrentUser.SystemRole "promo_codes"}}
{{if ne $lvl "none"}}
  <a href="/admin/promo-codes">Promo Codes
    {{if eq $lvl "read"}}<span class="text-xs text-gray-400">(Read Only)</span>{{end}}
  </a>
{{end}}
```

A second template helper `matrixLevel` returns the raw level string for read-only labeling.

### CLI provisioning

`cmd/createadmin/main.go` currently accepts `super_admin`, `support`, `marketing`. Add `partner` to the accepted-roles validation + the prompt copy.

### Database

Migration `00030_partner_role.sql`:

```sql
-- Migration: 00030_partner_role.sql
-- Adds 'partner' to the users.system_role CHECK constraint.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_system_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_system_role_check
    CHECK (system_role IS NULL OR system_role IN ('super_admin','support','marketing','partner'));
```

(The current constraint is identified by inspection — if it has a different name, the migration drops by exact name and re-adds. Verified at apply time.)

### Go enum

`internal/models/admin.go` (or wherever `SystemRole` constants live):

```go
SystemRolePartner SystemRole = "partner"
```

Plus add to any role-iteration helpers (e.g., `AllSystemRoles()` if it exists).

## Testing

- Unit: matrix lookup — `super_admin` is `full` on every section; Partner returns the matrix value; unknown section returns `none`.
- Unit: `RequireSection` middleware — short-circuits super_admin; allows partner on `tickets` GET+POST+DELETE; denies partner on `dev_mode` for any method; denies anonymous.
- Integration on dev: provision a Partner user via `createadmin`, log into admin UI, confirm sidebar shows the right items, navigate into Tickets (full), Promo Codes (read-only label visible), and Dev Mode (404/403). Hit DELETE `/api/admin/sessions/{sid}` from Partner — 204.

## Risk & rollback

- The matrix abstraction is opt-in per route. If a route group's switch from `RequireSuperAdmin()` to `RequireSection()` proves wrong, revert that one line; no schema change to roll back. The migration's down step is the dropped-and-rewritten CHECK constraint reverting to the prior name.
- Super-admin short-circuit means a typo in the matrix can never lock out the only role that can fix it.
