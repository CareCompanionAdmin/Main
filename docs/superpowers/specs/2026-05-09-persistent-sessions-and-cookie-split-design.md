# Persistent Sessions + Cookie Split — Design

**Date:** 2026-05-09
**Status:** Approved verbally; this slice ships to dev only.
**Bundles:** Roadmap features (3) concurrent admin+user session and (4) device/cookie session keys not IP.
**Unblocks:** Live Sessions admin page (feature 1), Partner role (5), admin logged-out banner (2).

## Goal

Replace today's stateless-JWT auth with a per-session identity layer so that:

1. Sessions can be individually revoked (kill-session) without invalidating the JWT signing key.
2. A single browser can hold one user session and one admin session at the same time.
3. All session restrictions key on a stable session id (`sid`) — never on IP. Multiple users from the same NAT'd IP must work.

## Non-goals (this slice)

- The Live Sessions admin UI itself — separate slice. This slice produces the data the UI will read.
- Cross-environment session viewing (showing prod sessions from the dev admin) — separate slice.
- Partner role — separate slice; doesn't depend on session shape.
- Killing/expiring existing live sessions on rollout — sessions created before this slice ships continue to validate by JWT signature alone until they expire naturally. New sessions get the `sid` claim and DB row.
- Refresh-token rotation, MFA, "remember me" toggles, and per-device session limits.

## Current state (verified 2026-05-09)

- Auth is HS256 JWT in cookie `access_token`. Claims: `user_id, email, family_id, role, system_role, first_name`. No `sid`.
- `JWT_ACCESS_EXPIRY` defaults to 8h (raised from 15m on `cadb14b`); silent refresh by `static/js/session_guard.js` (`3143177`).
- Schema has a `user_sessions` table (`migrations/00001`) but **no Go code reads or writes it** — verified via `grep -rn "user_sessions" --include='*.go'` returning zero hits. The table is unused scaffolding.
- IP is recorded in `error_logs` and `admin_audit_log` but never used for auth/validation.
- Single cookie `access_token` carries both regular-user and admin auth. Admin login overwrites user cookie and vice versa, so today the same browser cannot hold both at once.

## Design

### Session table

Drop the unused `user_sessions` table and create `sessions` with the columns we actually need:

```sql
CREATE TYPE session_kind AS ENUM ('user', 'admin');

CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind            session_kind NOT NULL,
    system_role     VARCHAR(32),                    -- snapshot at login; null for kind='user'
    family_id       UUID REFERENCES families(id),    -- snapshot at login; null for kind='admin'
    ip_at_start     INET,                            -- informational only, never validated against
    user_agent      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at      TIMESTAMPTZ,                    -- null = active
    expires_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_sessions_user        ON sessions(user_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_sessions_kind_active ON sessions(kind)    WHERE revoked_at IS NULL;
CREATE INDEX idx_sessions_expires     ON sessions(expires_at);
```

### JWT claims

Add `sid uuid.UUID` to `service.AuthClaims`. Existing JWTs (no `sid`) keep validating until they expire — middleware tolerates a missing `sid` claim during the rollover window and falls back to legacy stateless validation. Once `sid` is present, the middleware enforces the session row.

### Auth middleware (the new validation path)

```
1. Read cookie. For user routes prefer `user_access_token`; for admin routes prefer `admin_access_token`. Fall back to `access_token` (legacy).
2. Parse + verify JWT signature as today.
3. If claims have `sid`:
   a. Lookup `session:<sid>` in Redis (TTL 60s).
   b. On cache miss, SELECT one row from `sessions` by id.
   c. If row missing OR revoked_at IS NOT NULL OR expires_at < NOW(): return 401 with reason="session-revoked".
   d. Update `last_seen_at` opportunistically (best-effort UPDATE in a goroutine; failures logged not propagated).
   e. Cache the validation result for 60s.
4. If claims have NO `sid`: legacy path — accept on signature alone (backward compat for in-flight sessions). Drop this branch in a later slice once we're confident no legacy sessions remain.
```

### Cookie split

Login routes set the cookie that matches the session kind:
- `POST /api/auth/login` → `user_access_token` (kind=user)
- `POST /api/admin/auth/login` → `admin_access_token` (kind=admin)

Cookie attrs unchanged from today: `HttpOnly; Secure (prod); SameSite=Lax; Path=/; Max-Age=8h`.

Logout endpoints:
- `POST /api/auth/logout` clears `user_access_token` and revokes the user session.
- `POST /api/admin/auth/logout` clears `admin_access_token` and revokes the admin session.
- A new `POST /api/auth/logout-all` clears both cookies and revokes both sessions for the current user (if both exist).

Frontend kept simple: `session_guard.js` reads whichever cookie exists; admin templates explicitly look for `admin_access_token`. The single `access_token` cookie is preserved as a read-only fallback for in-flight sessions; new logins write only the kind-specific cookie.

### Kill-session API (foundation; UI in feature 1)

Internal-only for this slice — wire the service method, expose minimal admin endpoint for testing:

```
DELETE /api/admin/sessions/{sessionID}      → super_admin, support, partner only
```

Effect: `UPDATE sessions SET revoked_at = NOW() WHERE id = $1` + `DEL session:<sid>` in Redis.

Bulk version (`POST /api/admin/sessions/revoke` with body `{"ids":[...]}`) goes in feature 1 with the UI.

### Concurrent admin + user

Once the cookie split lands, a single browser can carry both `user_access_token` and `admin_access_token`. Each lives independently in its own session row. Logging into one does not touch the other. Logging out of one does not touch the other.

Constraint kept simple: at most one active session per `(user_id, kind)`. If a user logs in again with kind=user while an active user session exists, the existing one is revoked first (current behavior in spirit, just enforced at the row level now).

### Redis cache invariant

- Cache key: `session:<sid>` → string `"valid"` or `"revoked"`, TTL 60s.
- On revoke: write `"revoked"` immediately (before DB commit returns to client) to prevent a 60s window where a revoked session keeps validating.
- On rollover (cache miss + DB hit): write `"valid"` with TTL 60s.
- Worst-case staleness: 60s for an unrevoked-then-revoked session. Acceptable for an admin kill operation.

## Testing

- Unit: session repo (create / get / revoke / list-active / expire).
- Unit: auth middleware with `sid`-bearing claim — happy path, missing row, revoked row, expired row, missing `sid` (legacy).
- Integration on dev: login twice (user + admin) from the same browser via curl; both cookies present, both endpoints accessible. Revoke one; the other still works. Verify `sessions` rows.

## Out of scope reminders

The Live Sessions UI, the Partner role, admin logged-out banner, and any cross-environment session view are explicitly NOT part of this slice. They build on top of the `sessions` table this slice creates.

## Rollout

Dev only. Migration is `00029_sessions_table.sql`. After dev verification, ship to prod as a normal deploy. Existing JWTs in prod cookies keep validating against the legacy branch until they expire (≤8h after deploy), at which point all logins use the new path.
