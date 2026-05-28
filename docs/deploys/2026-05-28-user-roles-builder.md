# User Roles — Custom Role Builder

Adds the `/admin/user-roles` section under Administration. A super-admin
can create custom roles with per-section None/Read/Write permissions,
assign them to admin users via the existing admin-user form, and the
new roles immediately gate routes + sidebar via the existing
`RequireSection` middleware + `canSee` template helper.

Bundles the long-planned swap of Pro QA's temporary `RequireSuperAdmin()`
gate to `RequireSection("pro_qa")` so QA testers can be granted access
without elevation to super_admin.

## Migration

Migration `00041_user_roles.sql` runs on the **main DB** (not shared
support DB). No out-of-band GRANTs are needed — the carecompanion role
owns the schema. The runner applies it idempotently on prod startup.

## Code deploy

1. Confirm master tip has the user-roles commits merged in.
2. `./scripts/deploy.sh` (three DEPLOY confirmations).
3. After ASG instance refresh completes, verify:
   - `https://www.mycarecompanion.net/admin/user-roles/` renders with
     the 4 built-in roles (locked) and "No custom roles yet."
   - `/admin/pro-qa/intro` still 200s for super_admin (gate swap
     transparent — super_admin retains Full implicitly).
   - Create a test role through the UI, assign to a throwaway admin,
     log in as that admin, confirm sidebar shows only granted sections.

## Notable behavior

- **Per-env roles.** custom_roles + custom_role_permissions are NOT
  replicated between dev and prod. Each environment maintains its own
  list. Bryan accepts this for now; future slice could extend mirror
  coverage if needed.
- **Cache.** Matrix() consults an in-process cache (60s TTL) on
  custom-role lookups. Role mutations invalidate the cache. Stale
  permissions never persist longer than 60s + propagation across ASG
  instances.
- **Locked built-ins.** super_admin / support / marketing / partner
  stay hardcoded in `internal/auth/perm.go`. The UI renders them
  read-only.

## Rollback

1. **Code:** `git revert <commits>` + redeploy.
2. **Schema:** additive only. If hard cleanup needed:
   ```sql
   DROP TABLE IF EXISTS custom_role_permissions;
   DROP TABLE IF EXISTS custom_roles;
   ```
3. **Admin users assigned custom roles:** after rollback, any
   `admin_users.system_role` set to a custom-role name will fail
   `models.IsValidSystemRole()` and Matrix() will fall back to
   LevelNone — those users can still log in (the role string itself
   is just a tag) but every admin section will 403. Pre-rollback
   step: reassign affected users to a built-in role.
