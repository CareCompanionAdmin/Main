# Pro QA — Check Threads (comments + attachments)

Adds a back-and-forth comment thread + attachments to each requested check.
Migration `00040_pro_qa_check_threads.sql` creates two new tables on the
shared support DB. Grants are applied manually (see 00039 deploy doc for
prior precedent — runner cannot handle GRANTs because it also runs against
dev's local postgres, which has no carecomp_support_dev role).

## Pre-deploy: grant carecomp_support_dev access on prod RDS

After migration 00040 lands on prod (via `scripts/deploy.sh` running the
runner), grant the dev role the same access it has on the existing pro_qa
tables. This is what lets dev read/write check comments + attachments via
`SUPPORT_DB_DSN`.

```bash
PGPASSWORD="$PROD_DB_PASSWORD" psql \
  -h carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com \
  -U carecompanion -d carecompanion <<'SQL'
GRANT SELECT, INSERT, UPDATE, DELETE ON
  pro_qa_check_comments, pro_qa_check_attachments
TO carecomp_support_dev;
SQL
```

Verify:

```sql
SET ROLE carecomp_support_dev;
SELECT count(*) FROM pro_qa_check_comments;   -- expect 0
SELECT count(*) FROM pro_qa_check_attachments; -- expect 0
RESET ROLE;
```

## Code deploy

1. Confirm master tip has the check-threads commits merged in.
2. `./scripts/deploy.sh` (three DEPLOY confirmations).
3. After ASG instance refresh completes, verify:
   - `https://www.mycarecompanion.net/admin/pro-qa/checks` shows the existing
     checks with comment/attachment count badges (0 if none yet).
   - Open a check detail at `/admin/pro-qa/checks/{id}` — page renders.
   - Post a markdown comment → renders correctly + thread updates.
   - Change status via the header dropdown → auto-status-change comment
     appears in thread.
   - Upload a small image → appears in attachments list, download link works.

## Rollback

1. **Code:** revert the feature commits and redeploy.
2. **Schema:** tables are isolated; safe to leave in place. Hard cleanup:
   ```sql
   DROP TABLE IF EXISTS pro_qa_check_attachments;
   DROP TABLE IF EXISTS pro_qa_check_comments;
   ```
   Apply against prod RDS as the `carecompanion` role.
3. **S3 objects:** check attachments share the existing pro_qa S3 prefix.
   Identify check-attachment keys via the `pro_qa_check_attachments.storage_path`
   column and delete those keys individually if a full cleanup is needed
   (do NOT recursive-delete the prefix — would also nuke issue attachments).
