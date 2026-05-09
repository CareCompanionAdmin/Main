# Admin Logged-Out Banner — Design

**Date:** 2026-05-09
**Status:** Approved verbally; ships to dev only initially.
**Slice:** Roadmap feature (2).
**Depends on:** persistent-sessions slice + partner-role slice (both on dev).

## Goal

Mirror the user-side `session_guard.js` UX on the admin portal so an admin idle past their JWT expiry, or whose session was killed by another admin, gets a graceful "Sign in again" banner instead of a silent failure or unhelpful 401. Adds the silent-refresh path that today only exists for user sessions.

## Non-goals (this slice)

- Per-reason banner copy (`session_expired` vs `session_revoked`) — single message covers both.
- Differentiated UX for cross-env kills (revoked-from-dev affecting prod sessions).
- Auto-redirect after N seconds of banner display.

## Server changes

### 1. Admin login sets `admin_refresh_token` cookie

`AdminLoginSubmit` (`internal/handler/admin/ui_handlers.go`) currently writes only `admin_access_token`. Add a second cookie:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "admin_refresh_token",
    Value:    tokens.RefreshToken,
    Path:     "/api/admin/auth/refresh",
    HttpOnly: true,
    Secure:   isSecure,
    SameSite: http.SameSiteLaxMode,
    Expires:  time.Now().Add(7 * 24 * time.Hour),
})
```

Path-scoped to the refresh endpoint so the cookie isn't sent on every admin page load.

### 2. New endpoint `POST /api/admin/auth/refresh`

Public path (no auth middleware) — mirrors `/api/auth/refresh`. Handler reads `admin_refresh_token` cookie (or `{refresh_token}` body), calls `authService.RefreshToken(ctx, token)` which is already sid-aware, then writes a fresh `admin_access_token` + rotated `admin_refresh_token`.

The existing `authService.RefreshToken` validates the JWT signature and re-issues a sid-bearing access token. It does NOT currently re-validate the sid against the sessions table — but the next API call will, so a revoked sid still surfaces (within Redis cache TTL of 60s). Acceptable for this slice; tightening sid-on-refresh is a future improvement.

Returns JSON `{access_token, refresh_token, expires_at}` on success, 401 on failure (revoked refresh, malformed token).

### 3. New endpoint `GET /api/admin/auth/check`

Gated by `AuthMiddleware + RequireAnyAdminRole`. Body: `{"valid":true}` 200, or 401 from middleware. Lightweight — used by the JS guard to poll session liveness without doing any real work.

### 4. Admin logout clears the refresh cookie

`AdminLogout` already clears `admin_access_token`. Add a second clear for `admin_refresh_token` (path `/api/admin/auth/refresh`, expiry epoch).

## Client: `static/js/admin_session_guard.js`

Mirrors `session_guard.js` architecture, parameterized for the admin context.

### Cookie / endpoint / path differences

| Aspect | User-side (existing) | Admin-side (new) |
|---|---|---|
| Access cookie | `access_token` | `admin_access_token` |
| Refresh cookie | `refresh_token` | `admin_refresh_token` |
| Refresh endpoint | `POST /api/auth/refresh` | `POST /api/admin/auth/refresh` |
| Login redirect | `/login?return=<path>` | `/admin/login?return=<path>` |
| Path predicate | `/api/*` | `/api/admin/*` and `/admin/*` (admin UI HTML) |
| Banner copy | "Your session has expired. Sign in again to continue." | "Your admin session has expired or been terminated. Sign in again." |

### Behavior (same shape as user-side)

- **Proactive refresh:** decode JWT exp from `admin_access_token` cookie; schedule background refresh ~5min before expiry, capped at 1h.
- **Reactive 401 handling:** wrap `window.fetch`; any 401 on a guarded admin URL triggers the same single-flight refresh promise + retry-once.
- **Single-flight:** all parallel 401s share one network call.
- **Banner:** shown only when refresh genuinely fails (revoked sid OR expired refresh token). Styled red sticky-top with sign-in button.

### Path predicate (`shouldGuard`)

Returns true for any URL containing `/api/admin/` OR a same-origin URL whose path starts with `/admin/`. Returns false for the auth endpoints themselves (`/api/admin/auth/refresh`, `/admin/login`, `/admin/logout`) so login failures and the refresh call itself don't trigger recursive refresh attempts.

### Banner DOM

Same Tailwind classes as the user-side banner; copy and login href differ. Inserted as the first child of `<body>`. Single-shot (`SHOWN` flag) — never re-renders.

## Template wiring

`templates/admin/layout.html` includes the new script at the end of `<body>`, BEFORE the existing inline `<script>` block (so the guard is installed before any other admin JS fires API calls):

```html
<script src="/static/js/admin_session_guard.js"></script>
```

## Coexistence with the user-side guard

The user-side `session_guard.js` is NOT loaded on admin pages today (the admin layout doesn't reference it), so there's no conflict. If a developer later adds it to admin pages, the two guards would both wrap `window.fetch`; that's a future problem to solve, not a blocker for this slice — they wrap independently and the second wrapper sees the first's behavior, which is benign.

## Testing strategy

- **Unit (Go):** `AdminRefreshToken` handler — happy path (valid refresh cookie returns new tokens); rejects empty refresh; rejects bad signature.
- **Integration on dev (manual curl):**
  1. Admin login → both cookies set.
  2. `GET /api/admin/auth/check` → 200.
  3. Wait briefly, hit refresh manually → new access cookie issued.
  4. Revoke the admin's session via DELETE → next `/check` call → 401, refresh fails (revoked sid), simulated banner trigger.
- **Integration in browser:**
  - Log in as super_admin, leave page idle for 8h (or shorten `JWT_ACCESS_EXPIRY` for the test) — silent refresh keeps you logged in.
  - From a second admin login, kick the first session via Live Sessions page — first browser shows the red banner within 30s, click "Sign in again" → lands on `/admin/login?return=<the page they were on>`.

## Risks

- **Refresh-token cookie path** (`/api/admin/auth/refresh`) means it's only sent to that exact path. If a future endpoint moves, update both the cookie path and the JS body fallback.
- **Reactive 401 handler wrapping `window.fetch`:** an existing wrapper (e.g., a debug shim in dev) could double-wrap. Acceptable risk; matches the user-side pattern that has been live for weeks.
- **Banner on the login page itself** is suppressed (path check) — a stale cookie shouldn't show a banner on the page that fixes it.

## Rollout

Dev only first. After verification, ship to prod as part of the bundled deploy with the other three completed slices (report-storage, persistent-sessions, partner-role, live-sessions).
