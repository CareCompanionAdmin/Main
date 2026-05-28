# User Roles — Custom Role Builder

**Date:** 2026-05-28
**Status:** Approved by Bryan, implementing
**Scope:** New `/admin/user-roles` section under Administration. Lists the four built-in system roles (read-only) and lets a super-admin create / edit / delete custom roles with per-section None/Read/Write capabilities. Bundles the swap-out of Pro QA's temporary `RequireSuperAdmin()` gate so the new custom roles actually apply.

## Problem

Today `internal/auth/perm.go` carries a hardcoded matrix of four roles (`super_admin`, `support`, `marketing`, `partner`) × twenty sections × four levels (`none`/`read`/`write`/`full`). To grant the paid QA tester access to the Pro QA workspace, Bryan would have to either (a) promote her to `super_admin` (over-privileged), or (b) hand-edit the matrix in code and ship a release each time a new role is needed. The 2026-05-22 Pro QA ship called this out explicitly as the next planned slice.

## Goal

A super-admin can:
- Open Administration → User Roles
- See the four built-in roles in a read-only "locked" panel
- Create a new custom role with a machine name (e.g. `pro_qa`), display name, description, and a section-by-section permission grid
- Assign that role to an admin user from the existing admin-user form (the dropdown gains the new option)
- Trust that the route middleware (`RequireSection`) and sidebar visibility both honor the custom permissions immediately

## Non-goals

- **Editing the four built-in roles.** They stay locked in code per the 2026-05-09 comment. Changing them remains a code change + deploy.
- **Many-roles-per-user.** Single-role model is preserved (one `system_role` string column on `admin_users`).
- **A `full` (DELETE-allowed) level for custom roles.** DELETE remains super-admin-only by default. YAGNI for the QA tester case; trivial to add later as a third radio option.
- **Permission audit log of role mutations.** The existing `audit_log` will capture admin actions globally; no dedicated table.

## Data model

New migration `migrations/00041_user_roles.sql` on the **main DB** (not the support DB — these are operational tables, distinct from the cross-env support data):

```sql
CREATE TABLE IF NOT EXISTS custom_roles (
    id               UUID PRIMARY KEY,
    name             TEXT UNIQUE NOT NULL,
    display_name     TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_email TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_roles_name_format CHECK (name ~ '^[a-z][a-z0-9_]{1,49}$'),
    CONSTRAINT custom_roles_name_not_builtin CHECK (
        name NOT IN ('super_admin', 'support', 'marketing', 'partner')
    )
);

CREATE TABLE IF NOT EXISTS custom_role_permissions (
    role_id  UUID NOT NULL REFERENCES custom_roles(id) ON DELETE CASCADE,
    section  TEXT NOT NULL,
    level    TEXT NOT NULL CHECK (level IN ('read', 'write')),
    PRIMARY KEY (role_id, section)
);
```

`admin_users.system_role` keeps its current `TEXT` shape. Its value is either a built-in name or a `custom_roles.name` slug. No migration to existing rows.

### Built-in admin-mirror handling

Per `project_carecompanion_admin_replication.md`, admin user changes already dual-write through the mirror. The new `custom_roles` and `custom_role_permissions` tables are **not** replicated — they're operational config tables, not user identity. Each environment maintains its own. Bryan accepts that custom roles defined in dev must be re-created in prod (and vice versa) until/unless a future slice extends mirror coverage. The Matrix() lookup is local-DB-only.

## Code changes

### Matrix() extension (`internal/auth/perm.go`)

Today `Matrix(role, section) Level` is a pure function over the hardcoded map. We extend it to consult a service when the role name is not recognized:

```go
// PermResolver is what perm.go calls when it sees a non-builtin role name.
// Returned levels are "read", "write", or "" (= LevelNone).
type PermResolver interface {
    LookupCustomRole(name, section string) (Level, bool)
}

var customResolver PermResolver

// SetCustomResolver wires the runtime resolver. Called once at boot from main.go
// after the role service has its repo. Safe to call with nil to disable.
func SetCustomResolver(r PermResolver) { customResolver = r }
```

`Matrix(role, section)` becomes:

1. `super_admin` → `LevelFull` (unchanged short-circuit)
2. Built-in matrix lookup. If row exists for the role, return its level (unchanged).
3. If no built-in row and `customResolver != nil`, call `LookupCustomRole(role, section)`.
4. Otherwise `LevelNone`.

The resolver is implemented in `internal/service/role_service.go` and **caches results in-memory for 60 s** with explicit invalidation on role mutation (avoids per-request DB hit). Cache key: `(roleName, section)`. On any role create/update/delete the service calls `cache.Clear()`.

### Service + repository

- `internal/repository/role_repository.go`
  - `ListCustomRoles(ctx)` — for the list page
  - `GetCustomRole(ctx, id)` and `GetCustomRoleByName(ctx, name)` — for edit + resolver
  - `CreateCustomRole(ctx, r)` (writes the row + its permissions in a transaction)
  - `UpdateCustomRole(ctx, r)` (updates fields + replaces permissions in a transaction)
  - `DeleteCustomRole(ctx, id)` (cascades to permissions)
  - `ListPermissionsForRole(ctx, roleID)` and `GetLevelForRoleSection(ctx, name, section)` — for resolver
  - `CountAdminsByRoleName(ctx, name)` — for delete-safety check
- `internal/service/role_service.go`
  - Wraps the repo + cache
  - `Resolve(name, section)` — implements `auth.PermResolver`
  - Standard CRUD methods + cache invalidation
- Wire in `internal/service/services.go` and `cmd/server/main.go` (`auth.SetCustomResolver(services.Role)`).

### Sections list

`auth.Sections` in `perm.go` currently misses `pro_qa`. Add it. Resulting list passed to the role-builder UI grid.

### Routes (`internal/handler/admin/routes.go`)

New route group under super-admin gate:

```go
r.Route("/user-roles", func(r chi.Router) {
    r.Use(middleware.RequireSuperAdmin())  // only super_admin can manage roles
    r.Get("/",           h.UserRolesPage)
    r.Get("/new",        h.UserRoleNewPage)
    r.Post("/",          h.UserRoleCreate)
    r.Get("/{id}",       h.UserRoleEditPage)
    r.Post("/{id}",      h.UserRoleUpdate)
    r.Post("/{id}/delete", h.UserRoleDelete)
})
```

### Pro QA gate swap (bundled)

- `internal/handler/admin/routes.go`: `r.Use(middleware.RequireSuperAdmin())` on the `/admin/pro-qa` group → `r.Use(middleware.RequireSection("pro_qa"))`.
- `templates/admin/layout.html`: the sidebar's hard-coded `{{if eq $role "super_admin"}}` around the Pro QA entry → `{{if canSee $role "pro_qa"}}` (using the existing `canSee` template func that consults Matrix).
- Add the Pro QA entry to the role-builder grid because `pro_qa` is now in `Sections`.

### Admin user form

The existing admin-user create/edit form has a `system_role` `<select>`. Extend it to include custom roles as additional `<option>` entries grouped under an "Optgroup: Custom roles". No DB change.

### Templates

- `templates/admin/user_roles.html` — list page; built-in roles rendered as locked cards, custom roles in a table with edit/delete buttons.
- `templates/admin/user_role_form.html` — used for both new and edit. Header with name + display name + description fields; section grid below (left col: section label; right cols: 3 radio buttons per row: None / Read / Write). Per-row defaults to None for new roles.

### Sidebar

Add a "User Roles" link under the existing Administration grouping in `templates/admin/layout.html`. Visible to **super-admin only** (matching the route gate). Custom-role management is sensitive — exposing it to anyone else would be a privilege-escalation footgun.

## Validation / safety rules

- **Name format:** lowercase ASCII slug, `^[a-z][a-z0-9_]{1,49}$`. Enforced by CHECK constraint + server-side trim/lowercase before insert.
- **Name not built-in:** CHECK constraint rejects `super_admin`/`support`/`marketing`/`partner`.
- **Delete safety:** before delete, count admins assigned to this role; if > 0, return a 409 with a list of affected emails and block the action. Super-admin must reassign first.
- **Self-lockout safety:** A super-admin cannot delete `super_admin` (it isn't in `custom_roles` to begin with) or change their own role from the admin-user form (existing behavior, retained).
- **Unknown sections:** the UI grid only renders rows for sections in `auth.Sections`. Saving silently drops any extra sections that came back in the form body.

## Rollback plan

1. **Code:** implement on a feature branch (`user-roles-builder`). Revert is `git checkout master` (pre-merge) or `git revert <commits>` + redeploy (post-merge).
2. **Schema:** migration is additive (two new tables). No alter on existing tables. Rollback SQL:
   ```sql
   DROP TABLE IF EXISTS custom_role_permissions;
   DROP TABLE IF EXISTS custom_roles;
   ```
3. **Data:** any admin users assigned a custom role retain the string in `admin_users.system_role`. After rollback, those values fail to resolve in Matrix() and the user is denied access to admin pages (fails closed). Pre-rollback step: reassign affected users to a built-in role. The role list page renders this dependency before delete; same check applies before drop.
4. **Pro QA gate:** if rollback is needed and the gate has been swapped, revert restores `RequireSuperAdmin()` and the sidebar `{{if eq $role "super_admin"}}` check. Any user previously granted `pro_qa` via custom role then loses Pro QA access; super-admin retains. Acceptable.

## Testing plan (dev first, then prod)

1. **Migration smoke:** apply 00041 against dev local; both tables present; constraints work (reject illegal names).
2. **Empty list page:** `/admin/user-roles` shows the 4 built-in roles as locked cards and "No custom roles yet."
3. **Create role:** name = `pro_qa`, display = "Pro QA Tester", description = "Engages with the Pro QA workspace only." Permissions: only `pro_qa` set to `write`. All other sections None. Save.
4. **Edit role:** verify grid reflects saved state; toggle one row; save; refresh; persists.
5. **Assign to a test admin user:** create a throwaway admin (or temporarily reassign Bryan's own account on dev) and confirm:
   - Sidebar shows only Pro QA (Dashboard always visible to logged-in admins, plus any other sections granted)
   - `/admin/dashboard` 403s (not granted)
   - `/admin/tickets` 403s (not granted)
   - `/admin/pro-qa/checks` 200 (granted)
6. **Delete safety:** try to delete `pro_qa` while still assigned to a user → 409 with email list.
7. **Reassign + delete:** reassign user back to `super_admin`, retry delete → succeeds.
8. **Cache:** create role; assign to user; user immediately gets access (no 60-second wait — verifies invalidation hook).
9. **Prod deploy** via `scripts/deploy.sh`. Repeat 2/3/4 on prod against a separate test admin to confirm the same flow works there.
10. **QA tester account creation** post-deploy:
   - Bryan creates an `admin_users` row for the QA tester with `system_role = 'pro_qa'`.
   - QA tester logs in on dev OR prod and sees only the Pro QA workspace.

## File touch list

| File | Change |
|---|---|
| `migrations/00041_user_roles.sql` | new (~30 lines + grants if any) |
| `internal/auth/perm.go` | +20 lines: `PermResolver` interface, `SetCustomResolver`, Matrix() fallback. Add `pro_qa` to `Sections`. |
| `internal/repository/role_repository.go` | new (~200 lines) |
| `internal/service/role_service.go` | new (~150 lines incl. cache) |
| `internal/service/services.go` | wire `RoleService` into the container |
| `cmd/server/main.go` | call `auth.SetCustomResolver(services.Role)` |
| `internal/handler/admin/user_roles_handlers.go` | new (~250 lines: 6 handlers + view structs) |
| `internal/handler/admin/routes.go` | +8 lines (new group + Pro QA gate swap) |
| `internal/handler/admin/handler.go` | inject RoleService |
| `internal/handler/admin/admin_user_handlers.go` | extend the role dropdown to include custom roles |
| `templates/admin/user_roles.html` | new (~80 lines) |
| `templates/admin/user_role_form.html` | new (~120 lines) — used for both new + edit |
| `templates/admin/layout.html` | sidebar User Roles entry + Pro QA `canSee` swap |
| `templates/admin/admin_users.html` (or wherever the form lives) | dropdown additions |
| `docs/deploys/2026-05-28-user-roles-builder.md` | new — deploy notes (since this migration goes on main DB no out-of-band GRANTs needed) |

Estimated total: ~900 LOC added (mostly templates + boilerplate CRUD); ~20 LOC modified.

## Memory updates after ship

Append to `project_carecompanion_pro_qa.md`: "Pro QA gate now consults the role-builder; QA tester assigned `pro_qa` custom role. Built-in roles still the only thing in `auth/perm.go`'s matrix; custom roles in `custom_roles` table." Plus a new memory `project_carecompanion_role_builder.md` describing the resolver pattern + per-env role parity caveat.
