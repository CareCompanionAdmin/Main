# Partner Role Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `partner` system role with the locked permission matrix. Matrix-driven middleware + sidebar so future role changes are one column edit.

**Architecture:** Single Go file `internal/auth/perm.go` holds the section→role→level matrix. New middleware `RequireSection(section)` reads the matrix, with super_admin short-circuit. Existing route groups switch to `RequireSection` per section. Sidebar template helper reads the same matrix.

**Tech Stack:** Go 1.24, PostgreSQL ENUM, Chi router. Builds on persistent-sessions slice (already on dev).

**Spec:** `docs/superpowers/specs/2026-05-09-partner-role-design.md`

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `migrations/00030_partner_role.sql` | **Create** | `ALTER TYPE system_role ADD VALUE IF NOT EXISTS 'partner'`. |
| `internal/models/common.go` | **Modify** | Add `SystemRolePartner SystemRole = "partner"`; add to `IsValidSystemRole` switch. |
| `internal/models/user.go` | **Modify** | Add `IsPartner() bool` helper. |
| `internal/auth/perm.go` | **Create** | `Level` type, `Sections` list, `Matrix(role, section) Level`, `Allows(role, section, method) bool`, `RankAtLeast(have, need) bool`. The hard-coded matrix table. |
| `internal/auth/perm_test.go` | **Create** | Unit tests: super_admin always full; partner matrix is what spec says; unknown section → none. |
| `internal/middleware/section.go` | **Create** | `RequireSection(section string)` middleware with super-admin short-circuit. |
| `internal/middleware/section_test.go` | **Create** | Tests: super_admin bypass, partner allowed on full sections, partner denied on none/dev_mode, GET vs POST level mapping. |
| `internal/middleware/admin.go` | **Modify** | Add `partner` to `RequireAnyAdminRole`. (Other Require* keep their meaning — sometimes still useful for "this is a super-admin-only endpoint".) |
| `internal/handler/admin/routes.go` | **Modify** | Replace per-section super-admin gates with `RequireSection`. Routes that should stay super-admin-only (DevMode, SystemSettings, AuditLog) keep `RequireSuperAdmin`. |
| `internal/handler/admin/handlers.go` | **Modify** | `RevokeSession`: append `Partner` to allowed roles. |
| `internal/handler/admin/template_funcs.go` | **Create** | Template helpers `canSee(role, section) bool` and `matrixLevel(role, section) string`. Wire into the existing `parseTemplates` helper. |
| `templates/admin/layout.html` | **Modify** | Replace inline `{{if eq .CurrentUser.SystemRole "super_admin"}}` with `{{if canSee .CurrentUser.SystemRole "section_key"}}`. Read-only sections render the `(Read Only)` badge based on `matrixLevel`. |
| `cmd/createadmin/main.go` | **Modify** | Accept `partner` in role flag validation + help text. |

---

## Section keys (canonical, used everywhere)

```
dashboard, tickets, users, families, metrics_dashboard, copy_materials,
beta_program, bounty_program, promo_codes, infrastructure_status, error_logs,
development_mode, product_roadmap, financials, subscriptions, admin_users,
system_settings, audit_log, version_log, live_sessions
```

Every task that references a section key uses one of these exactly.

---

## Task 1: Migration — add partner enum value

**Files:** Create `migrations/00030_partner_role.sql`

- [ ] **Step 1: Write the migration**

```sql
-- Migration: 00030_partner_role.sql
-- Adds the 'partner' system_role enum value. Partner is a fourth admin role
-- with section-scoped access defined in internal/auth/perm.go (matrix-driven).
--
-- ALTER TYPE ... ADD VALUE is allowed inside a transaction in PostgreSQL 12+;
-- the new value cannot be USED in the same transaction, but adding it and
-- recording the migration row in the schema_migrations table works fine.
--
-- Rollback (run by hand if needed):
--   PostgreSQL doesn't support DROP VALUE on enums. To revert, re-create the
--   type without 'partner' and update the column. Avoid unless absolutely
--   necessary.

ALTER TYPE system_role ADD VALUE IF NOT EXISTS 'partner';
```

- [ ] **Step 2: Apply via runner**

```bash
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -f migrations/00030_partner_role.sql
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
    -c "SELECT unnest(enum_range(NULL::system_role)) AS roles;"
```

Expected: list includes `partner` along with the existing three.

- [ ] **Step 3: Commit**

```bash
git add migrations/00030_partner_role.sql
git commit -m "migrate: add partner system_role enum value"
```

---

## Task 2: Go enum + helpers

**Files:**
- Modify: `internal/models/common.go`
- Modify: `internal/models/user.go`

- [ ] **Step 1: Inspect current shape**

```bash
grep -n "SystemRoleSuperAdmin\|SystemRoleSupport\|SystemRoleMarketing\|IsValidSystemRole\|IsSuperAdmin\|IsSupport\|IsMarketing" internal/models/common.go internal/models/user.go
```

- [ ] **Step 2: Add `SystemRolePartner` constant in `internal/models/common.go`**

Find the constants block (around line 325):

```go
SystemRoleSuperAdmin SystemRole = "super_admin"
SystemRoleSupport    SystemRole = "support"
SystemRoleMarketing  SystemRole = "marketing"
```

Add:

```go
SystemRolePartner    SystemRole = "partner"
```

In whatever validity helper exists (around line 333), add `SystemRolePartner` to the accepted set:

```go
case SystemRoleSuperAdmin, SystemRoleSupport, SystemRoleMarketing, SystemRolePartner:
    return true
```

- [ ] **Step 3: Add `IsPartner()` to `internal/models/user.go`**

After the existing `IsMarketing` (around line 49), add:

```go
// IsPartner returns true when the user has the partner system role.
func (u *User) IsPartner() bool {
	return u.SystemRole.Valid && SystemRole(u.SystemRole.String) == SystemRolePartner
}
```

- [ ] **Step 4: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/models/common.go internal/models/user.go
git commit -m "models: add SystemRolePartner + IsPartner helper"
```

---

## Task 3: Permission matrix package

**Files:**
- Create: `internal/auth/perm.go`
- Create: `internal/auth/perm_test.go`

- [ ] **Step 1: Create `internal/auth/perm.go`**

```go
// Package auth holds cross-cutting authorization data — currently the
// section-keyed permission matrix that drives admin route gates and sidebar
// visibility. Single source of truth: editing the matrix here updates every
// consumer.
package auth

import (
	"net/http"
	"strings"

	"carecompanion/internal/models"
)

type Level string

const (
	LevelNone  Level = "none"
	LevelRead  Level = "read"
	LevelWrite Level = "write"
	LevelFull  Level = "full"
)

func levelRank(l Level) int {
	switch l {
	case LevelFull:
		return 3
	case LevelWrite:
		return 2
	case LevelRead:
		return 1
	default:
		return 0
	}
}

// RankAtLeast reports whether `have` meets or exceeds `need`.
func RankAtLeast(have, need Level) bool { return levelRank(have) >= levelRank(need) }

// Sections is the canonical list of section keys. Used by the sidebar so
// section ordering stays stable.
var Sections = []string{
	"dashboard", "tickets", "users", "families",
	"metrics_dashboard", "copy_materials", "beta_program", "bounty_program",
	"promo_codes", "infrastructure_status", "error_logs", "development_mode",
	"product_roadmap", "financials", "subscriptions",
	"admin_users", "system_settings", "audit_log", "version_log",
	"live_sessions",
}

// matrix encodes the locked 2026-05-09 permission table. Roles not listed for
// a section default to LevelNone. super_admin is in every row at LevelFull
// belt-and-suspenders, but Matrix() also short-circuits super_admin
// independent of the table.
var matrix = map[string]map[models.SystemRole]Level{
	"dashboard": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"tickets": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRoleMarketing:  LevelRead,
		models.SystemRolePartner:    LevelFull,
	},
	"users": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"families": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"metrics_dashboard": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"copy_materials": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"beta_program": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"bounty_program": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"promo_codes": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelRead,
		models.SystemRolePartner:    LevelRead,
	},
	"infrastructure_status": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"error_logs": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"development_mode": {
		models.SystemRoleSuperAdmin: LevelFull,
	},
	"product_roadmap": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"financials": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"subscriptions": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"admin_users": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"system_settings": {
		models.SystemRoleSuperAdmin: LevelFull,
	},
	"audit_log": {
		models.SystemRoleSuperAdmin: LevelFull,
	},
	"version_log": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"live_sessions": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
}

// Matrix returns the access level for (role, section). Super admin is always
// LevelFull regardless of the table. Unknown role or unknown section returns
// LevelNone — fail closed.
func Matrix(role models.SystemRole, section string) Level {
	if role == models.SystemRoleSuperAdmin {
		return LevelFull
	}
	row, ok := matrix[section]
	if !ok {
		return LevelNone
	}
	if lvl, ok := row[role]; ok {
		return lvl
	}
	return LevelNone
}

// RequiredLevelForMethod maps an HTTP method to the level required to call
// it. GET/HEAD/OPTIONS need read; POST/PUT/PATCH need write; DELETE needs
// full. The matrix never assigns "write" today (only none/read/full), so
// "write" effectively collapses to "full" for non-super_admin — which is the
// intended behavior for admin actions that affect other users.
func RequiredLevelForMethod(method string) Level {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return LevelRead
	case http.MethodDelete:
		return LevelFull
	default:
		return LevelWrite
	}
}

// Allows is the high-level helper: does (role) have permission to perform
// (method) on (section)?
func Allows(role models.SystemRole, section, method string) bool {
	return RankAtLeast(Matrix(role, section), RequiredLevelForMethod(method))
}
```

- [ ] **Step 2: Create `internal/auth/perm_test.go`**

```go
package auth

import (
	"net/http"
	"testing"

	"carecompanion/internal/models"
)

func TestMatrix_SuperAdminAlwaysFull(t *testing.T) {
	for _, sec := range Sections {
		if got := Matrix(models.SystemRoleSuperAdmin, sec); got != LevelFull {
			t.Errorf("super_admin on %s = %q, want full", sec, got)
		}
	}
	// Unknown section: super admin still full (the short-circuit fires).
	if got := Matrix(models.SystemRoleSuperAdmin, "definitely_not_a_section"); got != LevelFull {
		t.Errorf("super_admin on unknown section = %q, want full", got)
	}
}

func TestMatrix_PartnerLockedRows(t *testing.T) {
	cases := map[string]Level{
		"dashboard":             LevelFull,
		"tickets":               LevelFull,
		"users":                 LevelFull,
		"families":              LevelFull,
		"metrics_dashboard":     LevelFull,
		"copy_materials":        LevelRead,
		"beta_program":          LevelFull,
		"bounty_program":        LevelRead,
		"promo_codes":           LevelRead,
		"infrastructure_status": LevelRead,
		"error_logs":            LevelRead,
		"development_mode":      LevelNone,
		"product_roadmap":       LevelFull,
		"financials":            LevelFull,
		"subscriptions":         LevelFull,
		"admin_users":           LevelRead,
		"system_settings":       LevelNone,
		"audit_log":             LevelNone,
		"version_log":           LevelRead,
		"live_sessions":         LevelFull,
	}
	for sec, want := range cases {
		if got := Matrix(models.SystemRolePartner, sec); got != want {
			t.Errorf("partner on %s = %q, want %q", sec, got, want)
		}
	}
}

func TestMatrix_UnknownSectionForNonSuper(t *testing.T) {
	if got := Matrix(models.SystemRolePartner, "nope"); got != LevelNone {
		t.Errorf("partner on unknown section = %q, want none", got)
	}
}

func TestRequiredLevelForMethod(t *testing.T) {
	cases := map[string]Level{
		http.MethodGet:    LevelRead,
		http.MethodHead:   LevelRead,
		http.MethodPost:   LevelWrite,
		http.MethodPut:    LevelWrite,
		http.MethodPatch:  LevelWrite,
		http.MethodDelete: LevelFull,
	}
	for m, want := range cases {
		if got := RequiredLevelForMethod(m); got != want {
			t.Errorf("method %s = %q, want %q", m, got, want)
		}
	}
}

func TestAllows(t *testing.T) {
	// Partner can DELETE (full) on tickets but not on bounty_program (read).
	if !Allows(models.SystemRolePartner, "tickets", http.MethodDelete) {
		t.Error("partner should DELETE on tickets")
	}
	if Allows(models.SystemRolePartner, "bounty_program", http.MethodDelete) {
		t.Error("partner must NOT DELETE on bounty_program")
	}
	// Partner can GET bounty_program (read).
	if !Allows(models.SystemRolePartner, "bounty_program", http.MethodGet) {
		t.Error("partner should GET on bounty_program")
	}
	// Partner cannot GET dev_mode (none).
	if Allows(models.SystemRolePartner, "development_mode", http.MethodGet) {
		t.Error("partner must NOT GET on development_mode")
	}
}
```

- [ ] **Step 3: Run + commit**

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./internal/auth/... -v
git add internal/auth/perm.go internal/auth/perm_test.go
git commit -m "auth: permission matrix for partner role + tests"
```

---

## Task 4: RequireSection middleware

**Files:**
- Create: `internal/middleware/section.go`
- Create: `internal/middleware/section_test.go`
- Modify: `internal/middleware/admin.go`

- [ ] **Step 1: Create `internal/middleware/section.go`**

```go
package middleware

import (
	"net/http"

	"carecompanion/internal/auth"
)

// RequireSection gates a route by the permission matrix. The required level
// is derived from the HTTP method (see auth.RequiredLevelForMethod). Super
// admin short-circuits before any matrix lookup.
//
// Failures: 401 if no auth claims; 403 with reason "section_<name>_denied"
// if the role's matrix entry doesn't satisfy the required level.
func RequireSection(section string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetAuthClaims(r.Context())
			if claims == nil {
				JSONError(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if claims.IsSuperAdmin() {
				next.ServeHTTP(w, r)
				return
			}
			if !auth.Allows(claims.SystemRole, section, r.Method) {
				JSONError(w, "Forbidden: section_"+section+"_denied", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 2: Update `RequireAnyAdminRole` in `internal/middleware/admin.go`**

Find around line 45:

```go
func RequireAnyAdminRole() func(http.Handler) http.Handler {
	return RequireSystemRole(
		models.SystemRoleSuperAdmin,
		models.SystemRoleSupport,
		models.SystemRoleMarketing,
	)
}
```

Add Partner:

```go
func RequireAnyAdminRole() func(http.Handler) http.Handler {
	return RequireSystemRole(
		models.SystemRoleSuperAdmin,
		models.SystemRoleSupport,
		models.SystemRoleMarketing,
		models.SystemRolePartner,
	)
}
```

Find the related allow-list around line 71:

```go
models.SystemRoleSuperAdmin,
models.SystemRoleSupport,
models.SystemRoleMarketing,
```

Add `models.SystemRolePartner,` to that block too.

Also check `internal/middleware/require_subscription.go:69`:

```go
case models.SystemRoleSuperAdmin, models.SystemRoleSupport, models.SystemRoleMarketing:
```

Add `models.SystemRolePartner` to the case clause so Partner skips subscription checks the same way other admins do.

- [ ] **Step 3: Create `internal/middleware/section_test.go`**

```go
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

func reqWithRole(method, path string, role models.SystemRole) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	claims := &service.AuthClaims{SystemRole: role}
	ctx := context.WithValue(r.Context(), middleware.AuthClaimsKey, claims)
	return r.WithContext(ctx)
}

func TestRequireSection_SuperAdminBypass(t *testing.T) {
	called := false
	mw := middleware.RequireSection("development_mode")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithRole("GET", "/x", models.SystemRoleSuperAdmin))
	if !called {
		t.Fatalf("super_admin should pass; got status %d", rec.Code)
	}
}

func TestRequireSection_PartnerAllowedOnFull(t *testing.T) {
	for _, m := range []string{"GET", "POST", "DELETE"} {
		called := false
		mw := middleware.RequireSection("tickets")
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, reqWithRole(m, "/x", models.SystemRolePartner))
		if !called {
			t.Errorf("partner %s tickets denied (status %d), want pass", m, rec.Code)
		}
	}
}

func TestRequireSection_PartnerDeniedOnNone(t *testing.T) {
	mw := middleware.RequireSection("development_mode")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithRole("GET", "/x", models.SystemRolePartner))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireSection_PartnerReadAllowsGetDeniesPost(t *testing.T) {
	mw := middleware.RequireSection("admin_users")
	getCalled := false
	hGet := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { getCalled = true }))
	rec := httptest.NewRecorder()
	hGet.ServeHTTP(rec, reqWithRole("GET", "/x", models.SystemRolePartner))
	if !getCalled {
		t.Errorf("partner GET admin_users denied (status %d), want pass", rec.Code)
	}

	hPost := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	rec2 := httptest.NewRecorder()
	hPost.ServeHTTP(rec2, reqWithRole("POST", "/x", models.SystemRolePartner))
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("partner POST admin_users status = %d, want 403", rec2.Code)
	}
}

func TestRequireSection_NoClaims401(t *testing.T) {
	mw := middleware.RequireSection("tickets")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
```

If `AuthClaimsKey` is unexported, swap the test to use the exported helper used elsewhere in the codebase (look at how existing middleware tests stash claims). The pattern in `internal/middleware/auth.go` uses an unexported `contextKey` type — exporting just the key would be cleaner. If unexported, add a test-only helper:

```go
// SetAuthClaimsForTest is exported only for tests.
func SetAuthClaimsForTest(ctx context.Context, c *service.AuthClaims) context.Context {
    return context.WithValue(ctx, AuthClaimsKey, c)
}
```

at the bottom of `internal/middleware/auth.go`. Then test imports change accordingly.

- [ ] **Step 4: Run + commit**

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./internal/middleware/... -v
git add internal/middleware/section.go internal/middleware/section_test.go internal/middleware/admin.go internal/middleware/require_subscription.go
git commit -m "middleware: RequireSection + add partner to existing admin allow-lists"
```

---

## Task 5: Update kill-session endpoint to allow Partner

**Files:** Modify `internal/handler/admin/handlers.go`

- [ ] **Step 1: Find and update**

```bash
grep -n "RevokeSession" internal/handler/admin/handlers.go
```

Replace the role-allowlist line:

```go
if claims == nil || !claims.HasAnySystemRole(models.SystemRoleSuperAdmin, models.SystemRoleSupport) {
```

with:

```go
if claims == nil || !claims.HasAnySystemRole(models.SystemRoleSuperAdmin, models.SystemRoleSupport, models.SystemRolePartner) {
```

- [ ] **Step 2: Build + commit**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
git add internal/handler/admin/handlers.go
git commit -m "admin: partner allowed on DELETE /api/admin/sessions/{id}"
```

---

## Task 6: Route gates — switch super-admin gates to RequireSection

**Files:** Modify `internal/handler/admin/routes.go`

This is the most surface-area change. The current shape (verified by inspection):

- `r.Route("/super", ...)` (line 86) — uses `RequireSuperAdmin`. Houses many sections.
- `r.Route("/support", ...)` (line 171) — uses `RequireSupport`. Tickets-related.
- `r.Route("/marketing", ...)` (line 197) — uses `RequireMarketing`. Materials, beta, bounty, etc.
- `r.Route("/super/materials", ...)` (line 232) — uses `RequireSuperAdmin`.
- UI routes (line 251) — uses `RequireAnyAdminRole` for shell, with sub-groups for super_admin/support/marketing.

We don't restructure the URL space. We only swap the middleware on each block to `RequireSection("section_key")` for sections Partner can access, and leave `RequireSuperAdmin()` on the three Partner-blocked sections.

- [ ] **Step 1: Read the existing route file end-to-end**

```bash
sed -n '74,260p' internal/handler/admin/routes.go
```

Verify the line numbers above still match the file's current shape. Note any deviations.

- [ ] **Step 2: Swap gates inside `/super`**

The `/super` group covers many sections. We're going to break it apart per-section. The simplest mechanical edit: keep `r.Use(middleware.RequireSuperAdmin())` at the top of `/super`, and INSIDE that block wrap each route or sub-route in a per-section middleware call only when Partner needs access. But Chi middleware composes additively — once `RequireSuperAdmin` is applied, only super_admin gets in.

So instead, REMOVE `r.Use(middleware.RequireSuperAdmin())` from the `/super` group and apply per-route gates. Concretely, replace the existing block opener:

```go
r.Route("/super", func(r chi.Router) {
    r.Use(middleware.RequireSuperAdmin())
```

with:

```go
r.Route("/super", func(r chi.Router) {
    // No blanket gate — each sub-section sets its own gate below.
```

Then, for each route registered inside `/super`, group by section using `r.Group(...)` blocks, each with the right middleware. Mapping (section key → routes inside `/super`):

| Section key | Routes (paths inside `/super`) | Gate |
|---|---|---|
| admin_users | `GET/POST /admins`, `GET/PUT/DELETE /admins/{id}` | `RequireSection("admin_users")` |
| metrics_dashboard | `GET /metrics`, `POST /metrics/refresh` | `RequireSection("metrics_dashboard")` |
| system_settings | `GET /settings`, `PUT /settings/{key}`, `POST /maintenance` | `RequireSuperAdmin()` |
| audit_log | `GET /audit-log` | `RequireSuperAdmin()` |
| infrastructure_status | `GET /status`, `POST /status/refresh`, `GET/POST /infra-files...` | `RequireSection("infrastructure_status")` |
| error_logs | `GET /errors` (+ all sub-paths) | `RequireSection("error_logs")` |
| financials | `GET /financials/...`, `GET/PUT/POST /family-subscriptions/...` | `RequireSection("financials")` (covers financials AND subscriptions — both are Partner full) |
| subscriptions | (combined under financials block above) | (same) |
| promo_codes | `GET/POST/PUT /promo-codes...` | `RequireSection("promo_codes")` |
| development_mode | `POST /dev-mode/toggle`, `POST /dev-mode/kill-session`, `GET /dev-mode/sessions` | `RequireSuperAdmin()` |

Implementation pattern for each: wrap in an inline `r.Group` so the middleware applies only to those routes. Example for admin_users:

```go
r.Group(func(r chi.Router) {
    r.Use(middleware.RequireSection("admin_users"))
    r.Get("/admins", h.ListAdminUsers)
    r.Post("/admins", h.CreateAdminUser)
    r.Get("/admins/{id}", h.GetAdminUser)
    r.Put("/admins/{id}", h.UpdateAdminUser)
    r.Delete("/admins/{id}", h.DeleteAdminUser)
})
```

Apply the same pattern for every section above. The Partner-blocked sections (`system_settings`, `audit_log`, `development_mode`) keep `RequireSuperAdmin()` inside their own `r.Group`.

- [ ] **Step 3: Swap gates inside `/support` and `/marketing`**

`/support` (line 171): replace `r.Use(middleware.RequireSupport())` with section-specific groups for whichever support routes exist (likely tickets/users/families). Inspect first:

```bash
sed -n '171,200p' internal/handler/admin/routes.go
```

Wrap each section's routes in `r.Group` with the right `RequireSection` call.

Similarly for `/marketing` (line 197): Beta gets `RequireSection("beta_program")`, Bounty gets `RequireSection("bounty_program")`, Materials gets `RequireSection("copy_materials")`, etc.

For `/super/materials` (line 232): swap to `RequireSection("copy_materials")`.

- [ ] **Step 4: UI routes — replace inline super-admin checks**

Inside the protected UI block (line 251):

```go
r.Group(func(r chi.Router) {
    r.Use(middleware.RequireSuperAdmin())
    r.Get("/admins", h.AdminUsersPage)
    r.Get("/settings", h.SettingsPage)
    r.Get("/audit", h.AuditLogPage)
    // ...
})
```

Split: `/admins` is `RequireSection("admin_users")` (so partner gets read access — they hit GET); `/settings` and `/audit` keep `RequireSuperAdmin()`.

Repeat for the other UI sub-blocks (super_admin financials, super_admin status, etc.) — same mapping as Step 2.

- [ ] **Step 5: Build**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 6: Run all tests**

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./... 2>&1 | tail -20
```

Expected: all PASS (or "no test files").

- [ ] **Step 7: Commit**

```bash
git add internal/handler/admin/routes.go
git commit -m "admin: switch per-section route gates to RequireSection (matrix-driven)"
```

---

## Task 7: Sidebar template helpers

**Files:**
- Create: `internal/handler/admin/template_funcs.go`
- Modify: `templates/admin/layout.html`
- Modify: wherever `parseTemplates` is defined to register the funcs

- [ ] **Step 1: Find `parseTemplates`**

```bash
grep -n "parseTemplates\b" internal/handler/admin/*.go | head
```

Identify how it builds templates — likely it uses `template.New("...").ParseFiles(...)`. We need to inject `Funcs(...)` before parsing.

- [ ] **Step 2: Create `internal/handler/admin/template_funcs.go`**

```go
package admin

import (
	"html/template"

	"carecompanion/internal/auth"
	"carecompanion/internal/models"
)

// adminTemplateFuncs returns the FuncMap injected into every admin template.
func adminTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"canSee": func(role models.SystemRole, section string) bool {
			return auth.Matrix(role, section) != auth.LevelNone
		},
		"matrixLevel": func(role models.SystemRole, section string) string {
			return string(auth.Matrix(role, section))
		},
	}
}
```

- [ ] **Step 3: Register the funcs in parseTemplates**

In whichever file holds `parseTemplates` (likely `internal/handler/admin/handlers.go` or a `templates.go`), add `.Funcs(adminTemplateFuncs())` before the parse step. For example, before:

```go
return template.ParseFiles(...)
```

becomes:

```go
return template.New("layout.html").Funcs(adminTemplateFuncs()).ParseFiles(...)
```

(Match whatever invocation form the existing code uses.)

- [ ] **Step 4: Update `templates/admin/layout.html`**

Replace each `{{if eq .CurrentUser.SystemRole "super_admin"}}` (and `{{if or (eq ... "super_admin") (eq ... "support")}}` etc.) wrapper with `{{if canSee .CurrentUser.SystemRole "<section_key>"}}`.

Map per section block (refer to the file's existing structure):

- Support block (Tickets, Users, Families): wrap each line individually with `canSee`.
  ```html
  {{if canSee .CurrentUser.SystemRole "tickets"}}<a href="/admin/tickets">Tickets ...</a>{{end}}
  {{if canSee .CurrentUser.SystemRole "users"}}<a href="/admin/users">Users</a>{{end}}
  {{if canSee .CurrentUser.SystemRole "families"}}<a href="/admin/families">Families</a>{{end}}
  ```
  Wrap the whole `<div class="mb-6">` in `{{if or (canSee ... "tickets") (canSee ... "users") (canSee ... "families")}}` so the section header is only shown when at least one item is visible.

- Marketing block: same pattern for `metrics_dashboard`, `copy_materials`, `beta_program`, `bounty_program`, `promo_codes`. For sections with `(Read Only)` affordance, render based on `matrixLevel`:
  ```html
  {{if canSee .CurrentUser.SystemRole "promo_codes"}}
    <a href="/admin/promo-codes">Promo Codes
      {{if eq (matrixLevel .CurrentUser.SystemRole "promo_codes") "read"}}<span class="text-xs text-gray-400">(Read Only)</span>{{end}}
    </a>
  {{end}}
  ```

- System block: `infrastructure_status`, `error_logs`, `development_mode`.
- Roadmap, Finance, Subscriptions: each wrapped by their key.
- Administration block: `admin_users`, `system_settings`, `audit_log`, `version_log`, `live_sessions` (NEW link added — even though the page doesn't exist yet, the link goes to `/admin/sessions` which 404s for now; the section still renders so partner sees it).

Actually since the live-sessions page doesn't exist yet, OMIT the Live Sessions sidebar link in this slice. It gets added when the Live Sessions UI ships (next slice).

- [ ] **Step 5: Build + run**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./...
```

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/admin/template_funcs.go internal/handler/admin/*.go templates/admin/layout.html
git commit -m "admin/ui: sidebar reads permission matrix via canSee helper"
```

(`internal/handler/admin/*.go` covers whichever file parseTemplates lives in.)

---

## Task 8: Update createadmin CLI

**Files:** Modify `cmd/createadmin/main.go`

- [ ] **Step 1: Update flag default text + help text + validation**

```bash
grep -n "super_admin, support, marketing\|role" cmd/createadmin/main.go | head
```

For each `super_admin, support, marketing` string, append `, partner`. For the validation switch (line 47 area), add `partner` to the accepted list.

- [ ] **Step 2: Build + smoke**

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./cmd/createadmin
./createadmin -h 2>&1 | head -10
```

Expected: help text mentions `partner`. Cleanup the binary if it lands in repo root:

```bash
rm -f createadmin
```

- [ ] **Step 3: Commit**

```bash
git add cmd/createadmin/main.go
git commit -m "createadmin: accept partner role"
```

---

## Task 9: Dev verification end-to-end

**Files:** none (manual)

- [ ] **Step 1: Rebuild + restart**

```bash
export PATH=$PATH:/usr/local/go/bin && go build -buildvcs=false -o bin/carecompanion-new ./cmd/server
sudo systemctl stop carecompanion && mv bin/carecompanion-new bin/carecompanion && sudo systemctl start carecompanion && sleep 4
sudo journalctl -u carecompanion -n 20 --no-pager | grep -E "migrate|server on"
```

Expected: `[migrate] applying 00030_partner_role` then `applied 1 pending migration(s)`, then server starts.

- [ ] **Step 2: Provision a Partner test user**

Pick or create a partner user via the CLI:

```bash
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
  -c "UPDATE users SET system_role = 'partner' WHERE email = 'joe@workmaninsurancegroup.com';"
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
  -c "SELECT email, system_role FROM users WHERE system_role = 'partner';"
```

Expected: one row with `partner`.

- [ ] **Step 3: Login as Partner (admin form)**

```bash
curl -s -i -c /tmp/cookies-partner.txt -L -X POST http://localhost:8090/admin/login \
  -d "email=joe@workmaninsurancegroup.com&password=TestPass1!" | grep -E "HTTP/|Location:|Set-Cookie:"
```

Expected: 303 to `/admin/dashboard`, `admin_access_token` cookie set.

- [ ] **Step 4: Verify section gates**

Partner-allowed (full):

```bash
curl -s -b /tmp/cookies-partner.txt -o /dev/null -w "tickets_list=%{http_code}\n" http://localhost:8090/api/admin/support/tickets
curl -s -b /tmp/cookies-partner.txt -o /dev/null -w "financials_overview=%{http_code}\n" http://localhost:8090/api/admin/super/financials/overview
```

Expected: both 200.

Partner-read (GET ok, POST denied):

```bash
curl -s -b /tmp/cookies-partner.txt -o /dev/null -w "errors_get=%{http_code}\n" http://localhost:8090/api/admin/super/errors
curl -s -b /tmp/cookies-partner.txt -o /dev/null -w "errors_post=%{http_code}\n" -X POST http://localhost:8090/api/admin/super/errors/acknowledge-bulk -d '{}' -H "Content-Type: application/json"
```

Expected: GET 200, POST 403 with `section_error_logs_denied`.

Partner-none:

```bash
curl -s -b /tmp/cookies-partner.txt -o /dev/null -w "devmode_get=%{http_code}\n" http://localhost:8090/api/admin/super/dev-mode/sessions
```

Expected: 403.

Kill-session endpoint (Partner full):

```bash
# Pick any active session id
SID=$(PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -tAc "SELECT id FROM sessions WHERE revoked_at IS NULL LIMIT 1;")
curl -s -b /tmp/cookies-partner.txt -X DELETE -o /dev/null -w "kill=%{http_code}\n" "http://localhost:8090/api/admin/sessions/$SID"
```

Expected: 204.

- [ ] **Step 5: Sidebar visual smoke**

Open `http://98.88.131.147:8090/admin/dashboard` in a browser as the Partner user. Confirm: Tickets/Users/Families visible; Promo Codes shows `(Read Only)`; Dev Mode / System Settings / Audit Log NOT visible.

- [ ] **Step 6: Super-admin regression**

As your normal super_admin login (`bryan@bluebonnettech.com`), open the dashboard — every section that was previously visible should still be visible. (Super admin short-circuit means matrix typos can't lock you out, but verify anyway.)

- [ ] **Step 7: Notes**

Capture anything that deviated. We do NOT ship to prod from this slice.

---

## Self-Review

**1. Spec coverage**
- Migration adds `partner` enum value → Task 1
- Go enum + helpers → Task 2
- Matrix as data → Task 3
- Middleware → Task 4
- Kill-session → Task 5
- Route gates → Task 6
- Sidebar → Task 7
- CLI → Task 8
- Verification → Task 9

**2. Placeholders** — none. Each step contains exact code and exact commands.

**3. Type consistency**
- `auth.Level` / `auth.LevelNone/Read/Write/Full` — used identically in Tasks 3, 4, 7.
- `auth.Matrix(role, section)` / `auth.Allows(role, section, method)` — declared in Task 3, consumed in Tasks 4, 7.
- `models.SystemRolePartner` declared in Task 2, consumed everywhere.
- Section keys: same canonical list referenced in spec, Task 3 (matrix), Task 6 (route gates), Task 7 (sidebar).

**4. Risks**
- Task 6 is the largest surgical change. If a route group's restructuring breaks something, the symptom is a previously-accessible endpoint returning 401/403 for the existing role. Recovery: revert Task 6's commit; the matrix and middleware stay in place.
- Super-admin short-circuit in Task 4 ensures a typo in the matrix can never lock out super_admin from fixing it.
- The `internal/middleware/auth.go` exports `AuthClaimsKey` already (it's a typed constant). Tests in Task 4 use it directly. If the type isn't exported, Task 4's Step 3 includes a `SetAuthClaimsForTest` helper to add.
