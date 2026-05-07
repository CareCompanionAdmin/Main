# 2026-05-07 — Cross-env shared support tickets (Option A) + JWT 8h + silent refresh

## Summary

Three changes ship together:

1. **Cross-env shared support DB (Option A)** — both dev and prod read/write
   the same `support_tickets`, `ticket_messages`, `ticket_attachments` tables
   on prod's RDS. Dev opens a **second** connection pool to prod (set via
   `SUPPORT_DB_DSN`) for those three tables only; everything else still hits
   dev's local DB. Prod is unchanged: `SUPPORT_DB_DSN` is empty there, so the
   support repos fall through to the normal pool (same physical DB they
   already use).

2. **Migration 00027** — drops the FK constraints on `user_id`,
   `assigned_to`, `resolved_by`, `sender_id`, `uploader_id` (so a
   prod-side user_id pointing to a dev-only user doesn't violate FKs), and
   adds denormalized `*_email`, `*_first_name`, `*_last_name` columns on all
   three tables. Snapshots are written at INSERT time from the *creating
   env's* local users table, so the cross-env reader can render the original
   author without joining a foreign users table.

3. **JWT access expiry 1h → 8h** + **silent refresh in session_guard.js**
   (already pushed as `3143177`). Eliminates Joe Steinmetz's
   "logged-out mid-input" issue. Refresh tokens still 7d.

**Why now:** support tickets created in dev get reported by Bryan or
testers; replies and triage happen on prod. Today they live in two
databases and can't be cross-referenced. PHI tolerance is acceptable at
current user volume — Bryan explicitly approved the leak surface.

## Pre-deploy baseline (captured 2026-05-07 04:34 UTC)

| Item | Value |
|---|---|
| Live ECR `latest` digest (rollback target) | `sha256:df02eebb4af95193a2ffb1f612847c795664ae230f6a7e8fbcc6ee9085b333f6` |
| Pre-existing tagged rollback image | `rollback-safe` → `sha256:a85a6cded1ce778d…` (2026-05-03 known-good) |
| ASG | `carecompanion-asg`, desired=1 |
| origin/master HEAD | `3143177 fix(auth): silent refresh in session_guard.js` |
| Migration head on prod | `00026_change_type_interaction_alert` (00027 NOT yet applied) |
| Migration head on dev | `00027_support_share_across_envs` (applied 2026-05-07) |

## Commits being shipped (this batch)

`origin/master..HEAD` (oldest → newest, after this batch lands):

1. **(this commit)** `feat(support): cross-env shared support DB via SUPPORT_DB_DSN + denorm cols`
   - `migrations/00027_support_share_across_envs.sql` — drops 5 FK constraints
     on support tables, adds 9 denorm cols (3 per table × 3 tables), backfills
     from local `users`. Idempotent. Auto-applied on container boot.
   - `internal/config/config.go` — adds `DatabaseConfig.SupportDSN` from env
     `SUPPORT_DB_DSN`.
   - `internal/database/database.go` — extracts `NewWithDSN()` so a second
     pool can be opened against an arbitrary DSN.
   - `cmd/server/main.go` — opens the second pool when `SUPPORT_DB_DSN`
     non-empty, passes both pools to `repository.NewRepositories`.
   - `internal/repository/repository.go` — `NewRepositories(db, supportDB)`,
     wires both into Admin/UserSupport/TicketAttachment repos.
   - `internal/repository/admin_repository.go` — adds `supportDB` field +
     `lookupUserDenorm()` helper. All ticket/message/admin queries route
     through `r.supportDB`. Writes snapshot the actor's email/name into the
     denorm cols. Reads use `COALESCE(NULLIF(t.user_email,''), u.email,'')`
     so legacy rows (no denorm) still resolve via JOIN.
   - `internal/repository/user_support_repository.go` — same pattern for the
     parent-side `/support` ticket flow.
   - `internal/repository/ticket_attachment_repository.go` — same pattern
     for ticket attachments + `lookupUploaderDenorm()`.
   - **No behavior change** when `SUPPORT_DB_DSN` is empty — `supportDB`
     falls back to `db`, single-pool semantics preserved.

(Fix 1 silent refresh already pushed as `3143177`.)

## Database migration

Migration `00027_support_share_across_envs.sql` runs **idempotently** on
container boot via the migration runner shipped 2026-05-03 (`4b47397`). It
takes a `pg_advisory_lock(947328147)` so concurrent boots serialize.

Operations:
- `DROP CONSTRAINT IF EXISTS` on 5 FKs (no-op if already dropped).
- `ADD COLUMN IF NOT EXISTS` for 9 denorm cols (no-op if already added).
- `UPDATE … FROM users` backfill — only fills rows where the denorm col is
  empty, so a re-run is a no-op.
- `INSERT INTO schema_migrations`.

Expected runtime: <500ms on prod (small support tables).

## Post-deploy environment changes

### Prod (none initially)
Prod's `SUPPORT_DB_DSN` stays empty → support repos use the same RDS pool
they already use → identical behavior to today, plus the extra columns are
populated on every new write (used later when dev starts pointing here).

### Prod env vars to set on the EC2 instance
**Required for Fix 2:**
```
JWT_ACCESS_EXPIRY=8h
```
Set via `/etc/carecompanion/env` (or however the EC2 systemd unit reads
its env). If unset, defaults to 15m → users get logged out fast. The
silent-refresh JS handles this gracefully but 8h removes the refresh
chatter entirely for typical sessions.

### Dev (after prod RDS ingress is provisioned)
Set in `/home/carecomp/carecompanion/.env`:
```
SUPPORT_DB_DSN=host=carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com port=5432 user=carecomp_support_dev password=<from secrets> dbname=carecompanion sslmode=require
```
Then `sudo systemctl restart carecompanion`. On startup the log should read:
`Connected to separate support PostgreSQL (SUPPORT_DB_DSN set)`.

Until the cross-env DB role is provisioned (Task #44), dev keeps
`SUPPORT_DB_DSN` empty and operates in single-env mode.

## Rollback procedures

### Path A — Revert the code, keep the schema (preferred)

The denorm columns and dropped FKs are forward-compatible with the
previous code. Reverting just the code and leaving migration 00027
applied is the cleanest rollback:

```bash
cd /home/carecomp/carecompanion
git revert <this-commit-sha>
./scripts/deploy.sh
```

Result: prod goes back to 1h JWT (revert touches that only if Bryan
revoked the env var separately) and single-pool support repos. The denorm
columns sit unused on each row but cost nothing.

### Path B — Emergency image rollback (whole-deploy rollback, fastest)

If the new image is broken end-to-end:

```bash
# 1. Re-tag the previous image digest as :latest
aws ecr batch-get-image --region us-east-1 --repository-name carecompanion \
  --image-ids imageDigest=sha256:df02eebb4af95193a2ffb1f612847c795664ae230f6a7e8fbcc6ee9085b333f6 \
  --query 'images[0].imageManifest' --output text > /tmp/old.manifest

aws ecr put-image --region us-east-1 --repository-name carecompanion \
  --image-tag latest --image-manifest "$(cat /tmp/old.manifest)"

# 2. Trigger another ASG refresh
aws autoscaling start-instance-refresh \
  --auto-scaling-group-name carecompanion-asg \
  --preferences '{"MinHealthyPercentage":100,"MaxHealthyPercentage":200,"InstanceWarmup":180}' \
  --region us-east-1
```

Known-good rollback target:
`sha256:df02eebb4af95193a2ffb1f612847c795664ae230f6a7e8fbcc6ee9085b333f6`
(prod's `latest` immediately before this deploy)

### Path C — Schema rollback (only if migration causes a problem)

Migration 00027 is reversible. The down-migration SQL is included as a
comment block at the bottom of `migrations/00027_support_share_across_envs.sql`.
Apply manually against prod RDS only if the column additions or dropped
FKs cause an actual problem:

```bash
# Open a psql session against prod (requires bastion/IP allow-list)
PGPASSWORD="$PROD_PASSWORD" psql -h carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com \
  -U carecompanion -d carecompanion

# Then paste the rollback block from migrations/00027.
# The block:
#   1. Drops the 9 denorm columns
#   2. Re-adds 5 FK constraints with ON DELETE SET NULL
#   3. Deletes the 00027 row from schema_migrations
```

**Caveat:** re-adding the FKs will fail if any support row has a
user_id/sender_id/uploader_id that doesn't exist in prod's users table
(i.e. a dev-only user that was created cross-env after Path B was
introduced). If that happens, NULL out those columns on the affected
rows first.

## Monitor commands

```bash
# Instance refresh progress
aws autoscaling describe-instance-refreshes \
  --auto-scaling-group-name carecompanion-asg \
  --region us-east-1 \
  --query 'InstanceRefreshes[0].[Status,PercentageComplete]' --output table

# ALB target health
aws elbv2 describe-target-health --region us-east-1 \
  --target-group-arn arn:aws:elasticloadbalancing:us-east-1:943431294725:targetgroup/carecompanion-tg/bade3e56ae036ce7 \
  --query 'TargetHealthDescriptions[*].[Target.Id,TargetHealth.State]' --output table

# CloudWatch app logs — startup + migration line
aws logs tail /carecompanion/app --region us-east-1 --since 5m \
  --filter-pattern '"migration" "schema"'

# Confirm 00027 ran on prod
PGPASSWORD="$PROD_PASSWORD" psql -h carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com \
  -U carecompanion -d carecompanion \
  -c "SELECT version FROM schema_migrations WHERE version LIKE '00027%';"
```

## Smoke tests after deploy

1. **Prod ticket create + admin reply**: log in as a tester, file a ticket,
   verify the row has `user_email` populated. From admin portal, reply,
   verify `sender_email` populated on the message. Confirms denorm writes.

2. **Prod ticket list**: verify existing tickets still show user emails
   (the COALESCE fallback path).

3. **Open-count badge**: hit `/api/admin/support/tickets/open-count`,
   expect a number, no 500.

4. **JWT 8h**: log in, decode access token, verify `exp - iat = 28800`
   (8 × 3600). Idle for 10+ minutes, refresh dashboard, no logout.

5. **Silent refresh**: in DevTools, set
   `localStorage._sessionDebug='1'`, then watch console for
   `[session-guard] silent refresh ok` on a long-lived tab.

## Cross-env enablement (Task #44 — gated, not in this deploy)

Once #1+#2 land in prod:

1. Identify dev EC2's outbound IP.
2. Add a security-group ingress rule on prod's RDS for that IP, port 5432.
3. Create a limited DB role on prod RDS:
   ```sql
   CREATE ROLE carecomp_support_dev WITH LOGIN PASSWORD '<random>';
   GRANT SELECT, INSERT, UPDATE, DELETE ON support_tickets, ticket_messages, ticket_attachments TO carecomp_support_dev;
   GRANT SELECT ON users TO carecomp_support_dev;  -- read-only, for COALESCE fallback joins
   ```
4. Stash the password in `/home/carecomp/secrets/`.
5. Set `SUPPORT_DB_DSN` on dev's `.env`, restart, verify the startup log
   line `Connected to separate support PostgreSQL`.
6. End-to-end test: create a ticket on dev, verify it appears on prod's
   admin portal, reply on prod, verify the reply appears on dev.

## Open risks

- **PHI surface**: dev DB users authenticated to dev now read prod's
  ticket descriptions. Bryan accepted this at current user volume.
- **Dev-side DELETE on prod tables**: the `carecomp_support_dev` role has
  DELETE. Mitigation is "tester pool is small + audit log". Could be
  tightened to SELECT+INSERT+UPDATE if delete-from-dev becomes a problem.
- **FK loss visibility**: with the FKs dropped, an orphaned ticket
  (user deleted) is no longer auto-NULLed on user delete; the row's
  `user_id` will dangle. The denorm columns make this invisible to the
  reader (email/name still render). Cleanup script can be added later if
  needed.
