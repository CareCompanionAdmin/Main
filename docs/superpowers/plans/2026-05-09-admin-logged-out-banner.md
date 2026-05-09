# Admin Logged-Out Banner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mirror the user-side `session_guard.js` UX on the admin portal so an admin idle past their JWT expiry, or whose session was killed by another admin, gets a graceful "Sign in again" banner with a silent-refresh path instead of a silent failure or unhelpful 401.

**Architecture:** Admin login already issues an access JWT cookie; we add an `admin_refresh_token` cookie at login and two new admin auth endpoints (`POST /api/admin/auth/refresh` public, `GET /api/admin/auth/check` protected). A new `static/js/admin_session_guard.js`, parameterized for the admin context, wraps `window.fetch` to do single-flight refresh-and-retry on 401s, and proactively refreshes ~5min before access-token expiry. The user-side guard is untouched and is not loaded on admin pages, so the two coexist by isolation.

**Tech Stack:** Go 1.24 + Chi (server), `golang-jwt/jwt/v5`, vanilla JS (no framework), Tailwind classes for the banner DOM. Reference patterns: `internal/handler/api/auth_handler.go:139` (`RefreshToken`) for the server, `static/js/session_guard.js` for the client.

**Spec:** `docs/superpowers/specs/2026-05-09-admin-logged-out-banner-design.md`

---

## File structure

| File | Status | Responsibility |
|---|---|---|
| `internal/handler/admin/auth_handlers.go` | Create | New JSON endpoints `AdminRefreshToken`, `AdminAuthCheck` |
| `internal/handler/admin/auth_handlers_test.go` | Create | Unit tests for `AdminRefreshToken` |
| `internal/handler/admin/ui_handlers.go` | Modify | `AdminLoginSubmit` sets refresh cookie; `AdminLogout` clears it |
| `internal/handler/admin/routes.go` | Modify | Wire `GET /auth/check` inside protected `Routes()` |
| `cmd/server/main.go` | Modify | Mount `POST /api/admin/auth/refresh` PUBLIC (before the protected Mount) |
| `static/js/admin_session_guard.js` | Create | Admin variant of `session_guard.js` (admin cookies, admin endpoints, admin path predicate) |
| `templates/admin/layout.html` | Modify | Include `<script src="/static/js/admin_session_guard.js"></script>` ahead of the inline script block |

---

### Task 1: Server — set `admin_refresh_token` cookie at login

**Files:**
- Modify: `internal/handler/admin/ui_handlers.go` (the `AdminLoginSubmit` cookie block around lines 187–197)

**Context:** `AdminLoginSubmit` currently sets only the `admin_access_token` cookie at `Path: "/"`. The refresh cookie scopes to `Path: "/api/admin/auth/refresh"` so it's only sent on the refresh call (not every admin page load). 7-day expiry mirrors the user side.

- [ ] **Step 1: Edit `AdminLoginSubmit` to also set the refresh cookie**

In `internal/handler/admin/ui_handlers.go`, immediately after the existing `http.SetCookie(w, &http.Cookie{Name: "admin_access_token", ...})` block (the one that ends `Expires: tokens.ExpiresAt`), add a second `http.SetCookie` call:

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

The `tokens` variable already has a `RefreshToken` field — see `internal/service/auth_service.go:521` (`RefreshToken: refreshTokenString,`).

- [ ] **Step 2: Compile**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /home/carecomp/carecompanion
git add internal/handler/admin/ui_handlers.go
git commit -m "feat(admin-auth): set admin_refresh_token cookie at login"
```

---

### Task 2: Server — clear `admin_refresh_token` cookie at logout

**Files:**
- Modify: `internal/handler/admin/ui_handlers.go` (the `AdminLogout` cookie-clear block around lines 207–221)

**Context:** `AdminLogout` already clears `admin_access_token` (Path `/`) and the legacy `access_token`. Add a third clear for the refresh cookie at its path-scoped location.

- [ ] **Step 1: Add the refresh-cookie clear to `AdminLogout`**

In `internal/handler/admin/ui_handlers.go`, after the existing block that clears the legacy `access_token` cookie (`Name: "access_token"` with `Expires: time.Unix(0, 0)`), and BEFORE the `http.Redirect(...)` call, add:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "admin_refresh_token",
    Value:    "",
    Path:     "/api/admin/auth/refresh",
    Expires:  time.Unix(0, 0),
    HttpOnly: true,
})
```

Path must match the path used at login (Task 1) — browsers only clear path-scoped cookies when the clear is sent at the same path.

- [ ] **Step 2: Compile**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /home/carecomp/carecompanion
git add internal/handler/admin/ui_handlers.go
git commit -m "feat(admin-auth): clear admin_refresh_token cookie at logout"
```

---

### Task 3: Server — write `AdminRefreshToken` handler with unit test (TDD)

**Files:**
- Create: `internal/handler/admin/auth_handlers.go`
- Create: `internal/handler/admin/auth_handlers_test.go`

**Context:** The user-side handler at `internal/handler/api/auth_handler.go:139` is the model. Differences for admin: reads from `admin_refresh_token` cookie (or JSON body fallback), writes new `admin_access_token` (Path `/`) + rotated `admin_refresh_token` (Path `/api/admin/auth/refresh`), JSON response shape `{access_token, refresh_token, expires_at}`. The existing `service.AuthService.RefreshToken(ctx, token)` already validates the JWT and re-issues sid-bearing tokens — handler just orchestrates cookies.

- [ ] **Step 1: Write the failing tests**

Create `internal/handler/admin/auth_handlers_test.go`:

```go
package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"carecompanion/internal/handler/admin"
)

// stubRefresher implements just enough of the auth-service surface that
// AdminRefreshToken needs. The handler depends on the *AuthService, so we
// instead verify by hitting the real handler with a mocked refresh token
// pulled from a test JWT key. Lighter alternative: test the handler's
// cookie/body parsing paths only.
//
// We test three pure-handler concerns here without spinning up the full
// service:
//   1. Empty body + missing cookie → 400
//   2. Unparseable JSON body + missing cookie → 400
//   3. Body present but blank refresh + missing cookie → 400 (treated as missing)
//
// The signature-validation path is exercised by the existing RefreshToken
// service tests; we don't re-test it here.

func TestAdminRefreshToken_NoCookieNoBody_Returns400(t *testing.T) {
	h := admin.NewHandler(nil, nil)
	req := httptest.NewRequest("POST", "/api/admin/auth/refresh", nil)
	rec := httptest.NewRecorder()
	h.AdminRefreshToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminRefreshToken_GarbageBodyNoCookie_Returns400(t *testing.T) {
	h := admin.NewHandler(nil, nil)
	req := httptest.NewRequest("POST", "/api/admin/auth/refresh", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.AdminRefreshToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminRefreshToken_EmptyRefreshFieldNoCookie_Returns400(t *testing.T) {
	h := admin.NewHandler(nil, nil)
	req := httptest.NewRequest("POST", "/api/admin/auth/refresh", strings.NewReader(`{"refresh_token":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.AdminRefreshToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go test ./internal/handler/admin/ -run TestAdminRefreshToken -v`
Expected: FAIL with `h.AdminRefreshToken undefined` (or the package doesn't compile yet because the file is missing).

- [ ] **Step 3: Create the handler file**

Create `internal/handler/admin/auth_handlers.go`:

```go
package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"carecompanion/internal/middleware"
)

// AdminRefreshToken handles admin token refresh.
//
// Public endpoint (mounted in cmd/server/main.go BEFORE the protected
// /api/admin Mount, so AuthMiddleware does NOT run on this path — by design,
// because refresh must work AFTER the access token has lapsed).
//
// Reads admin_refresh_token cookie (preferred) or {"refresh_token":"..."}
// JSON body. Writes a fresh admin_access_token + rotated admin_refresh_token
// cookie. Mirrors the user-side handler at internal/handler/api/auth_handler.go.
func (h *Handler) AdminRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	// Body first. JSON errors are non-fatal — fall back to cookie.
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.RefreshToken == "" {
		cookie, err := r.Cookie("admin_refresh_token")
		if err != nil || cookie.Value == "" {
			middleware.JSONError(w, "Refresh token required", http.StatusBadRequest)
			return
		}
		req.RefreshToken = cookie.Value
	}

	tokens, err := h.authService.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		middleware.JSONError(w, "Invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_access_token",
		Value:    tokens.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  tokens.ExpiresAt,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/api/admin/auth/refresh",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tokens)
}

// AdminAuthCheck is a lightweight liveness probe used by admin_session_guard.js.
// Mounted INSIDE the protected /api/admin Routes(), so AuthMiddleware runs first:
// a missing/expired/revoked session yields 401 from the middleware, otherwise
// the handler returns 200 with {"valid":true}.
func (h *Handler) AdminAuthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"valid": true})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go test ./internal/handler/admin/ -run TestAdminRefreshToken -v`
Expected: PASS for all 3 cases.

- [ ] **Step 5: Compile the whole package**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go build ./...`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
cd /home/carecomp/carecompanion
git add internal/handler/admin/auth_handlers.go internal/handler/admin/auth_handlers_test.go
git commit -m "feat(admin-auth): AdminRefreshToken + AdminAuthCheck handlers"
```

---

### Task 4: Server — wire `/auth/check` inside the protected admin Routes()

**Files:**
- Modify: `internal/handler/admin/routes.go` (after `r.Use(middleware.AuthMiddleware(...))`, before the existing `r.Delete("/sessions/{sessionID}", ...)`)

**Context:** `Routes()` already has `AuthMiddleware` applied at the top. Adding `/auth/check` here means a missing/expired/revoked session yields 401 from the middleware — exactly what the JS guard needs to detect liveness.

- [ ] **Step 1: Add the route**

In `internal/handler/admin/routes.go`, immediately after `r.Use(middleware.AuthMiddleware(h.authService))` (around line 84), add:

```go
// Lightweight liveness probe used by admin_session_guard.js. AuthMiddleware
// returns 401 on missing/expired/revoked session — handler just confirms 200.
r.Get("/auth/check", h.AdminAuthCheck)
```

- [ ] **Step 2: Compile**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /home/carecomp/carecompanion
git add internal/handler/admin/routes.go
git commit -m "feat(admin-auth): wire GET /api/admin/auth/check"
```

---

### Task 5: Server — wire public `POST /api/admin/auth/refresh` in main.go

**Files:**
- Modify: `cmd/server/main.go` (the `r.Route("/api/admin", ...)` block around lines 286–289)

**Context:** The protected `Routes()` applies `AuthMiddleware`, which would block a refresh after access-token expiry. We register the refresh path BEFORE the `Mount("/", adminHandler.Routes())` so chi matches it first. The `ContentTypeJSON` middleware applied at the parent block still wraps it (refresh is JSON, so this is fine).

- [ ] **Step 1: Edit main.go**

Replace this block:

```go
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(middleware.ContentTypeJSON)
		r.Mount("/", adminHandler.Routes())
	})
```

with:

```go
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(middleware.ContentTypeJSON)

		// Public refresh endpoint — registered BEFORE the protected Mount so
		// chi matches it first. AuthMiddleware does NOT run here; refresh
		// must work AFTER the access token has lapsed.
		r.Post("/auth/refresh", adminHandler.AdminRefreshToken)

		r.Mount("/", adminHandler.Routes())
	})
```

- [ ] **Step 2: Compile**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go build ./...`
Expected: no errors.

- [ ] **Step 3: Smoke-test on dev**

Restart dev: `cd /home/carecomp/carecompanion && ./scripts/dev.sh` (or the watcher will hot-reload).

Quick check that `/auth/refresh` is reachable WITHOUT auth (will 400 because no cookie/body, but should NOT 401):

Run: `curl -sS -X POST -i http://localhost:8090/api/admin/auth/refresh -o /dev/null -w '%{http_code}\n'`
Expected: `400` (Refresh token required) — NOT 401.

Quick check that `/auth/check` IS protected (no cookie → 401):

Run: `curl -sS -i http://localhost:8090/api/admin/auth/check -o /dev/null -w '%{http_code}\n'`
Expected: `401`.

- [ ] **Step 4: Commit**

```bash
cd /home/carecomp/carecompanion
git add cmd/server/main.go
git commit -m "feat(admin-auth): mount public POST /api/admin/auth/refresh"
```

---

### Task 6: Client — create `static/js/admin_session_guard.js`

**Files:**
- Create: `static/js/admin_session_guard.js`

**Context:** Mirrors `static/js/session_guard.js` architecture exactly: proactive timer (5min before exp, 1h cap), reactive `window.fetch` wrapper with single-flight refresh, htmx error guard, sticky-top red banner. Differences: reads `admin_access_token` cookie, posts to `/api/admin/auth/refresh`, banner copy is admin-specific, login redirect goes to `/admin/login?return=...`, `shouldGuard` matches `/api/admin/*` and `/admin/*` (admin UI HTML pages) and excludes `/admin/login`, `/admin/logout`, `/api/admin/auth/refresh`.

- [ ] **Step 1: Create the file**

Create `static/js/admin_session_guard.js`:

```javascript
// admin_session_guard.js — admin-portal variant of session_guard.js.
//
// Same architecture as the user-side guard, parameterized for admin
// cookies/endpoints/path predicate. Loaded ONLY on admin pages (the user-side
// guard is not loaded there, so the two don't fight over window.fetch).

(function () {
    'use strict';

    var SHOWN = false;
    var EXPIRY_TIMER = null;
    var REFRESH_PROMISE = null;

    var REFRESH_LEAD_SECONDS = 5 * 60;
    var TIMER_CAP_MS = 60 * 60 * 1000;

    function getCookie(name) {
        var v = '; ' + document.cookie;
        var parts = v.split('; ' + name + '=');
        return parts.length === 2 ? parts.pop().split(';').shift() : '';
    }

    // The admin access token is HttpOnly so document.cookie won't see it. We
    // still try (in case the deployment relaxes that), and fall back to a
    // server-supplied window.__ADMIN_TOKEN_EXP if present (not currently set
    // — left as a future hook). Without an exp, we simply skip the proactive
    // timer; reactive 401 handling still works.
    function getJWT() {
        return getCookie('admin_access_token') || '';
    }

    function tokenExp(jwt) {
        if (!jwt) return 0;
        var parts = jwt.split('.');
        if (parts.length !== 3) return 0;
        try {
            var pad = parts[1] + '==='.slice((parts[1].length + 3) % 4);
            var json = atob(pad.replace(/-/g, '+').replace(/_/g, '/'));
            var payload = JSON.parse(json);
            return typeof payload.exp === 'number' ? payload.exp : 0;
        } catch (_) {
            return 0;
        }
    }

    var ORIGINAL_FETCH = null;

    // Single-flight refresh. The HttpOnly admin_refresh_token cookie is sent
    // automatically because the request URL matches its Path. No body needed.
    function attemptRefresh() {
        if (REFRESH_PROMISE) return REFRESH_PROMISE;

        REFRESH_PROMISE = ORIGINAL_FETCH('/api/admin/auth/refresh', {
            method: 'POST',
            credentials: 'same-origin'
        })
            .then(function (resp) {
                if (!resp || !resp.ok) {
                    var err = new Error('admin refresh failed: ' + (resp && resp.status));
                    err.status = resp && resp.status;
                    throw err;
                }
                return resp.json();
            })
            .then(function (data) {
                scheduleProactiveRefresh();
                return data;
            })
            .catch(function (err) {
                showBanner();
                throw err;
            })
            .then(function (data) {
                REFRESH_PROMISE = null;
                return data;
            }, function (err) {
                REFRESH_PROMISE = null;
                throw err;
            });

        return REFRESH_PROMISE;
    }

    function scheduleProactiveRefresh() {
        if (EXPIRY_TIMER) {
            clearTimeout(EXPIRY_TIMER);
            EXPIRY_TIMER = null;
        }
        var jwt = getJWT();
        var exp = tokenExp(jwt);
        if (!exp) return;
        var nowSec = Math.floor(Date.now() / 1000);
        if (exp <= nowSec) {
            attemptRefresh().catch(function () { /* banner shown */ });
            return;
        }
        var delaySec = Math.max(exp - nowSec - REFRESH_LEAD_SECONDS, 1);
        var delayMs = Math.min(delaySec * 1000, TIMER_CAP_MS);
        EXPIRY_TIMER = setTimeout(function () {
            attemptRefresh().catch(function () { /* banner shown */ });
        }, delayMs);
    }

    function showBanner() {
        if (SHOWN) return;
        SHOWN = true;
        if (window.location.pathname === '/admin/login') return;

        var ret = window.location.pathname + window.location.search;
        var loginHref = '/admin/login?return=' + encodeURIComponent(ret);

        var existing = document.getElementById('admin-session-expired-banner');
        if (existing) existing.remove();

        var banner = document.createElement('div');
        banner.id = 'admin-session-expired-banner';
        banner.className = 'border-b border-red-300 bg-red-50 text-red-900 ' +
            'px-4 py-2 text-sm sticky top-0 z-50';
        banner.innerHTML = '<div class="max-w-7xl mx-auto flex items-center gap-3">' +
            '<svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">' +
            '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" ' +
            'd="M12 9v2m0 4h.01M5.07 19h13.86c1.54 0 2.5-1.67 1.73-3L13.73 4c-.77-1.33-2.69-1.33-3.46 0L3.34 16c-.77 1.33.19 3 1.73 3z" />' +
            '</svg>' +
            '<div class="flex-1">Your admin session has expired or been terminated. Sign in again to continue.</div>' +
            '<a href="' + loginHref + '" class="inline-flex items-center px-3 py-1 rounded-md bg-red-600 hover:bg-red-700 text-white font-medium whitespace-nowrap">' +
            'Sign in again</a>' +
            '</div>';
        document.body.insertBefore(banner, document.body.firstChild);
    }

    // Guard admin API and admin UI HTML routes. Skip the auth endpoints
    // themselves — login failures and the refresh call must not recurse.
    function shouldGuard(url) {
        if (typeof url !== 'string') {
            try { url = String(url); } catch (_) { return false; }
        }
        if (url.indexOf('/api/admin/auth/refresh') >= 0) return false;
        if (url.indexOf('/admin/login') >= 0) return false;
        if (url.indexOf('/admin/logout') >= 0) return false;
        if (url.indexOf('/api/admin/') >= 0) return true;
        // Same-origin admin UI HTML — relative URLs and absolute /admin/* hits.
        if (url.indexOf('://') < 0 && url.indexOf('/admin/') === 0) return true;
        return false;
    }

    function installFetchGuard() {
        if (!window.fetch) return;
        ORIGINAL_FETCH = window.fetch.bind(window);
        window.fetch = function (input, init) {
            var url = typeof input === 'string' ? input : (input && input.url) || '';
            return ORIGINAL_FETCH(input, init).then(function (resp) {
                if (!(resp && resp.status === 401 && shouldGuard(url))) {
                    return resp;
                }
                return attemptRefresh().then(function () {
                    return ORIGINAL_FETCH(input, init);
                }).catch(function () {
                    return resp;
                });
            });
        };
    }

    function installHtmxGuard() {
        if (!document.body) return;
        document.body.addEventListener('htmx:responseError', function (evt) {
            var status = evt.detail && evt.detail.xhr && evt.detail.xhr.status;
            if (status === 401) {
                attemptRefresh().catch(function () { /* banner shown */ });
            }
        });
    }

    function init() {
        installFetchGuard();
        installHtmxGuard();

        var jwt = getJWT();
        var exp = tokenExp(jwt);
        var nowSec = Math.floor(Date.now() / 1000);

        if (jwt && exp > 0 && exp <= nowSec) {
            attemptRefresh().catch(function () { /* banner shown */ });
        } else if (jwt && exp > nowSec) {
            scheduleProactiveRefresh();
        }
        // No readable token (HttpOnly is the default in this app): proactive
        // path is skipped, reactive 401 wrapper still does the work.
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
```

- [ ] **Step 2: Sanity-check the file is reachable**

Run (with dev still running from Task 5): `curl -sS -o /dev/null -w '%{http_code}\n' http://localhost:8090/static/js/admin_session_guard.js`
Expected: `200`.

- [ ] **Step 3: Commit**

```bash
cd /home/carecomp/carecompanion
git add static/js/admin_session_guard.js
git commit -m "feat(admin-auth): admin_session_guard.js — silent refresh + banner"
```

---

### Task 7: Template — include the guard script in `templates/admin/layout.html`

**Files:**
- Modify: `templates/admin/layout.html` (immediately before the existing `<script>` block at line ~170)

**Context:** Loading the script BEFORE the inline JS ensures `window.fetch` is wrapped before any other admin code fires API calls. The script is small and self-contained.

- [ ] **Step 1: Insert the script tag**

In `templates/admin/layout.html`, find the line that opens the inline script block (currently at ~line 170: `<script>` immediately followed by `// Admin sidebar toggle for mobile`). Add a new line BEFORE it:

```html
    <script src="/static/js/admin_session_guard.js"></script>
    <script>
        // Admin sidebar toggle for mobile
        ...
```

So the result reads (showing the surrounding context):

```html
        </main>
    </div>

    <script src="/static/js/admin_session_guard.js"></script>
    <script>
        // Admin sidebar toggle for mobile
        function toggleAdminSidebar() {
```

- [ ] **Step 2: Verify the page still renders**

Reload http://localhost:8090/admin/dashboard in the browser (after logging in). DevTools Network tab: `admin_session_guard.js` should load with 200 status. Console: no errors.

If you don't have a browser to hand, curl:

Run: `curl -sS -b /tmp/admin-cookies.txt http://localhost:8090/admin/dashboard | grep -c admin_session_guard.js`
(Assumes you've logged in via curl earlier; otherwise just check the static asset loads.)
Expected: `1` (or higher).

- [ ] **Step 3: Commit**

```bash
cd /home/carecomp/carecompanion
git add templates/admin/layout.html
git commit -m "feat(admin-auth): include admin_session_guard.js in admin layout"
```

---

### Task 8: Manual end-to-end verification on dev

**Files:** none (verification only).

**Context:** The unit tests cover the handler's input parsing. The full silent-refresh-and-banner flow needs a real round-trip with cookies + a sid that can be revoked. Use curl for the API plumbing and a browser for the JS behavior.

- [ ] **Step 1: Login round-trip — both cookies set**

Run:
```bash
curl -sS -i -c /tmp/admin-cookies.txt \
  -X POST http://localhost:8090/admin/login \
  -d 'email=<admin-email>&password=<password>' \
  | grep -iE 'set-cookie|^HTTP'
```

(Use a real admin email/password from the dev DB.)

Expected: `HTTP/1.1 303 See Other`, plus two `Set-Cookie:` lines — one for `admin_access_token` (Path=/), one for `admin_refresh_token` (Path=/api/admin/auth/refresh).

- [ ] **Step 2: `/auth/check` returns 200 with valid cookie**

Run:
```bash
curl -sS -b /tmp/admin-cookies.txt -w '\nHTTP %{http_code}\n' \
  http://localhost:8090/api/admin/auth/check
```

Expected: `{"valid":true}` and `HTTP 200`.

- [ ] **Step 3: `/auth/refresh` issues a new access cookie**

Run:
```bash
curl -sS -b /tmp/admin-cookies.txt -c /tmp/admin-cookies.txt -i \
  -X POST http://localhost:8090/api/admin/auth/refresh \
  | grep -iE 'set-cookie|^HTTP'
```

Expected: `HTTP/1.1 200 OK` and two `Set-Cookie:` lines (rotated `admin_access_token` + `admin_refresh_token`). Body (in stdout if you drop `-i`) should be `{"access_token":"...","refresh_token":"...","expires_at":"..."}`.

- [ ] **Step 4: Refresh with no cookie/body returns 400, not 401**

Run:
```bash
curl -sS -i -X POST http://localhost:8090/api/admin/auth/refresh \
  | grep '^HTTP'
```

Expected: `HTTP/1.1 400 Bad Request`. Confirms the path is NOT behind AuthMiddleware.

- [ ] **Step 5: Browser — banner appears after another admin kicks the session**

Manual:
1. In Chrome, log in at `http://localhost:8090/admin/login` as a super_admin or partner.
2. Open a second browser (or incognito + different admin account) and log in.
3. From the second browser, navigate to `/admin/sessions`, find the first browser's row, click its "Revoke" button.
4. In the first browser, leave the dashboard tab open. Within ~30s the next API call (e.g. the auto-refresh on the sessions page, or any nav) should 401, the JS guard attempts a refresh which now fails (sid revoked), and the red banner appears at top: "Your admin session has expired or been terminated. Sign in again to continue."
5. Click "Sign in again" → lands on `/admin/login?return=<previous-page>`.

If the first browser is on a static page with no API calls, force one: open DevTools console and run `fetch('/api/admin/auth/check')`. Banner should fire shortly after.

- [ ] **Step 6: Browser — silent refresh keeps a long-idle session alive**

Manual (only if time permits — slow test):
1. In a fresh browser, log in as admin.
2. Set the proactive timer to fire sooner by editing `REFRESH_LEAD_SECONDS` in `admin_session_guard.js` to `60` (refresh 1 min before exp) and reloading. (Revert this edit before commit.)
3. Wait until the timer fires (DevTools Network tab → look for `POST /api/admin/auth/refresh` 200).
4. Confirm the page never shows the banner. The `admin_access_token` cookie's `Expires` value should advance.

After verifying, REVERT the `REFRESH_LEAD_SECONDS` change before continuing — this is a debug-only tweak.

- [ ] **Step 7: Test with `go test ./...` to confirm nothing else broke**

Run: `cd /home/carecomp/carecompanion && /usr/local/go/bin/go test ./...`
Expected: PASS across the board (the existing test suite + the 3 new admin handler tests).

- [ ] **Step 8: No commit (verification only)**

If Step 6 required a debug tweak, ensure `git status` is clean before moving on.

```bash
cd /home/carecomp/carecompanion && git status
```
Expected: `nothing to commit, working tree clean`.

---

## Self-review

**Spec coverage:**

| Spec section | Covered by |
|---|---|
| Server change 1 — admin login sets `admin_refresh_token` cookie | Task 1 |
| Server change 2 — `POST /api/admin/auth/refresh` endpoint | Task 3 (handler) + Task 5 (route) |
| Server change 3 — `GET /api/admin/auth/check` endpoint | Task 3 (handler) + Task 4 (route) |
| Server change 4 — admin logout clears refresh cookie | Task 2 |
| Client `static/js/admin_session_guard.js` | Task 6 |
| Cookie / endpoint / path differences table | Task 6 (cookie names, endpoint, login href, path predicate, banner copy) |
| Behavior — proactive refresh, reactive 401, single-flight, banner | Task 6 (matches user-side architecture) |
| Path predicate `shouldGuard` | Task 6 (`/api/admin/*` and `/admin/*`, excludes login/logout/refresh) |
| Banner DOM (Tailwind classes, single-shot, body insert) | Task 6 |
| Template wiring | Task 7 |
| Coexistence with user-side guard | No new code needed — admin layout doesn't load `session_guard.js`, noted in spec |
| Testing — Go unit | Task 3 (3 cases) |
| Testing — integration via curl | Task 8 steps 1–4 |
| Testing — browser (kick + idle silent refresh) | Task 8 steps 5–6 |
| Risks — cookie path, fetch wrapping, banner on login page | Mitigated in Task 1 (path), Task 6 (single wrapper, login-page suppression) |

No spec gaps.

**Type consistency:** Handler method names are stable across tasks: `AdminRefreshToken`, `AdminAuthCheck`. Cookie names are stable: `admin_access_token` (Path /), `admin_refresh_token` (Path /api/admin/auth/refresh). Endpoint paths stable: `POST /api/admin/auth/refresh`, `GET /api/admin/auth/check`. Banner DOM id stable: `admin-session-expired-banner`. JS function names stable: `attemptRefresh`, `scheduleProactiveRefresh`, `showBanner`, `shouldGuard`, `installFetchGuard`, `installHtmxGuard`, `init`.

**Placeholder scan:** No "TBD"/"TODO"/vague clauses. All steps include exact code, exact commands, exact expected output.
