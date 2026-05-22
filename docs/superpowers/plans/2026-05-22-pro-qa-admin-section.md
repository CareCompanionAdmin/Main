# Pro QA Admin Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an admin-portal section "Pro QA" that lets Bryan and a paid QA tester share project info, requested-check lists, and a private bidirectional issue tracker — same records visible from dev and prod.

**Architecture:** Reuse the existing shared-support-DB pattern (SUPPORT_DB_DSN). New tables live on prod RDS; dev reaches them via `carecomp_support_dev` role. Access gated behind `RequireSuperAdmin` middleware (Bryan plans a finer-grained role-builder later, so we keep the gate single-purpose and easy to swap). Markdown comments rendered server-side via goldmark; attachments use the existing `BlobStorage` abstraction with a new `pro-qa/` S3 prefix.

**Tech Stack:** Go 1.24, Chi router, html/template, PostgreSQL (shared support DB), goldmark (markdown→HTML), AWS S3, Tailwind CSS.

---

## File Structure

**New files:**
- `migrations/00039_pro_qa.sql` — tables + indexes
- `internal/models/pro_qa.go` — structs
- `internal/repository/pro_qa_repository.go` — DB access (uses supportDB pool)
- `internal/service/pro_qa_service.go` — orchestration + markdown render + S3 upload
- `internal/handler/admin/pro_qa_handlers.go` — HTTP handlers
- `templates/admin/pro_qa_layout.html` — sub-nav shell (4 tabs)
- `templates/admin/pro_qa_intro.html`
- `templates/admin/pro_qa_info.html`
- `templates/admin/pro_qa_checks.html`
- `templates/admin/pro_qa_issues.html` — list view
- `templates/admin/pro_qa_issue_detail.html` — single issue + thread
- `static/js/pro_qa.js` — markdown preview toggle + attachment uploader
- `docs/deploys/2026-05-22-pro-qa.md` — deploy/grant notes for prod cutover

**Modified files:**
- `go.mod`, `go.sum` — add `github.com/yuin/goldmark` (direct dep)
- `internal/repository/repository.go` — wire `NewProQARepo(supportDB)`
- `internal/service/services.go` — wire `NewProQAService(...)` with its own BlobStorage namespace `"pro_qa"` and prefix `cfg.Storage.S3Prefix + "pro-qa/"`
- `internal/handler/admin/routes.go` — register `/pro-qa/*` routes under `RequireSuperAdmin`
- `internal/handler/admin/handlers.go` (Handler struct) — add `proQAService` field and `SetProQAService` wiring (matches existing pattern)
- `cmd/server/main.go` — call `adminHandler.SetProQAService(services.ProQA)`
- `templates/admin/layout.html` — add Pro QA sidebar section after Roadmap, visible only when `eq $role "super_admin"`

---

## Task 1: Migration — pro_qa tables

**Files:**
- Create: `migrations/00039_pro_qa.sql`

- [ ] **Step 1: Write migration**

```sql
-- 00039_pro_qa.sql
--
-- Purpose: Admin-only "Pro QA" workspace for a paid QA engagement.
-- Same DB as support tickets (SUPPORT_DB_DSN) so dev/prod see the same
-- records. All tables intentionally have NO foreign keys to users(id):
-- the QA person and Bryan log in from either env (dev or prod) with
-- different admin_users UUIDs across envs, so we store denormalized
-- email/name like support_tickets does (see migration 00027).
--
-- Rollback (manual):
--   DROP TABLE pro_qa_issue_comments;
--   DROP TABLE pro_qa_issue_attachments;
--   DROP TABLE pro_qa_issues;
--   DROP TABLE pro_qa_requested_checks;
--   DROP TABLE pro_qa_info;

CREATE TABLE IF NOT EXISTS pro_qa_info (
    id           INT PRIMARY KEY DEFAULT 1,            -- singleton row
    body_md      TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by_email TEXT,
    CONSTRAINT pro_qa_info_singleton CHECK (id = 1)
);
INSERT INTO pro_qa_info (id, body_md) VALUES (1, '') ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS pro_qa_requested_checks (
    id           UUID PRIMARY KEY,
    title        TEXT NOT NULL,
    body_md      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open',         -- open | in_review | done
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_email TEXT,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_checks_sort ON pro_qa_requested_checks (sort_order, created_at);

CREATE TABLE IF NOT EXISTS pro_qa_issues (
    id                 UUID PRIMARY KEY,
    issue_number       SERIAL UNIQUE,                  -- short human ID (#1, #2, ...)
    parent_issue_id    UUID REFERENCES pro_qa_issues(id) ON DELETE SET NULL,
    title              TEXT NOT NULL,
    description_md     TEXT NOT NULL DEFAULT '',
    environment        TEXT,                            -- 'dev' | 'prod'
    platform           TEXT,                            -- 'ios' | 'android' | 'web' | 'admin' | ...
    status             TEXT NOT NULL DEFAULT 'open',    -- open | needs_info | in_progress | resolved | closed | wont_fix
    severity           TEXT NOT NULL DEFAULT 'medium',  -- low | medium | high | critical
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_email   TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at          TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_issues_status ON pro_qa_issues (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pro_qa_issues_parent ON pro_qa_issues (parent_issue_id);

CREATE TABLE IF NOT EXISTS pro_qa_issue_comments (
    id               UUID PRIMARY KEY,
    issue_id         UUID NOT NULL REFERENCES pro_qa_issues(id) ON DELETE CASCADE,
    body_md          TEXT NOT NULL,
    author_email     TEXT,
    author_name      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_status_change BOOLEAN NOT NULL DEFAULT FALSE,    -- TRUE for auto-comments like "status: open → resolved"
    status_from      TEXT,
    status_to        TEXT
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_comments_issue ON pro_qa_issue_comments (issue_id, created_at);

CREATE TABLE IF NOT EXISTS pro_qa_issue_attachments (
    id              UUID PRIMARY KEY,
    issue_id        UUID NOT NULL REFERENCES pro_qa_issues(id) ON DELETE CASCADE,
    comment_id      UUID REFERENCES pro_qa_issue_comments(id) ON DELETE SET NULL,
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    storage_driver  TEXT NOT NULL,                     -- 's3' | 'localfs'
    storage_path    TEXT NOT NULL,
    uploaded_by_email TEXT,
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_attachments_issue ON pro_qa_issue_attachments (issue_id);
```

- [ ] **Step 2: Run migration on dev's LOCAL DB (no-op pollution is fine; harmless)**

```bash
PGPASSWORD="carecompanion" psql -h localhost -U carecompanion -d carecompanion -f migrations/00039_pro_qa.sql
```

Expected: `CREATE TABLE` × 5, `CREATE INDEX` × 4, `INSERT 0 1`.

- [ ] **Step 3: Run migration on prod RDS (this is the support DB dev READS too)**

This requires `claude-superadmin` temp creds (per memory `reference_prod_db_access.md`). Bryan must approve this single-shot privileged step. After temp creds are exported:

```bash
PGPASSWORD="$PROD_DB_PASSWORD" psql -h carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com -U carecompanion -d carecompanion -f migrations/00039_pro_qa.sql
```

- [ ] **Step 4: Grant the dev-scoped support role access to the new tables (on prod RDS)**

Same psql session as Step 3 — these grants only take effect on the shared DB:

```sql
GRANT SELECT, INSERT, UPDATE, DELETE ON pro_qa_info, pro_qa_requested_checks, pro_qa_issues, pro_qa_issue_comments, pro_qa_issue_attachments TO carecomp_support_dev;
GRANT USAGE, SELECT ON SEQUENCE pro_qa_issues_issue_number_seq TO carecomp_support_dev;
```

Verify:
```sql
SET ROLE carecomp_support_dev;
SELECT count(*) FROM pro_qa_info;
RESET ROLE;
```

Expected: `1` (the singleton row).

- [ ] **Step 5: Commit**

```bash
git add migrations/00039_pro_qa.sql
git commit -m "feat(pro-qa): migration 00039 — pro_qa tables on shared support DB"
```

---

## Task 2: Models

**Files:**
- Create: `internal/models/pro_qa.go`

- [ ] **Step 1: Write structs**

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type ProQAInfo struct {
	BodyMD         string
	UpdatedAt      time.Time
	UpdatedByEmail string
}

type ProQARequestedCheck struct {
	ID             uuid.UUID
	Title          string
	BodyMD         string
	Status         string // open | in_review | done
	SortOrder      int
	CreatedAt      time.Time
	CreatedByEmail string
	UpdatedAt      time.Time
}

type ProQAIssue struct {
	ID              uuid.UUID
	IssueNumber     int
	ParentIssueID   *uuid.UUID
	Title           string
	DescriptionMD   string
	Environment     string
	Platform        string
	Status          string
	Severity        string
	CreatedAt       time.Time
	CreatedByEmail  string
	UpdatedAt       time.Time
	ClosedAt        *time.Time
	// View-only:
	CommentCount    int
	AttachmentCount int
}

type ProQAIssueComment struct {
	ID             uuid.UUID
	IssueID        uuid.UUID
	BodyMD         string
	AuthorEmail    string
	AuthorName     string
	CreatedAt      time.Time
	IsStatusChange bool
	StatusFrom     string
	StatusTo       string
}

type ProQAAttachment struct {
	ID               uuid.UUID
	IssueID          uuid.UUID
	CommentID        *uuid.UUID
	Filename         string
	ContentType      string
	SizeBytes        int64
	StorageDriver    string
	StoragePath      string
	UploadedByEmail  string
	UploadedAt       time.Time
}

// ProQA status / severity / env / platform allowed values, used by handlers
// for input validation and by templates for dropdowns.
var (
	ProQACheckStatuses  = []string{"open", "in_review", "done"}
	ProQAIssueStatuses  = []string{"open", "needs_info", "in_progress", "resolved", "closed", "wont_fix"}
	ProQAIssueSeverity  = []string{"low", "medium", "high", "critical"}
	ProQAEnvironments   = []string{"dev", "prod"}
	ProQAPlatforms      = []string{"ios", "android", "web", "admin"}
)
```

- [ ] **Step 2: Verify it compiles**

```bash
export PATH=$PATH:/usr/local/go/bin && cd /home/carecomp/carecompanion && go build ./internal/models/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/models/pro_qa.go
git commit -m "feat(pro-qa): models for info, checks, issues, comments, attachments"
```

---

## Task 3: Repository

**Files:**
- Create: `internal/repository/pro_qa_repository.go`
- Modify: `internal/repository/repository.go`

- [ ] **Step 1: Write repository interface + implementation**

```go
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type ProQARepository interface {
	// Info
	GetInfo(ctx context.Context) (*models.ProQAInfo, error)
	UpdateInfo(ctx context.Context, bodyMD, email string) error

	// Requested checks
	ListChecks(ctx context.Context) ([]models.ProQARequestedCheck, error)
	CreateCheck(ctx context.Context, c *models.ProQARequestedCheck) error
	UpdateCheck(ctx context.Context, c *models.ProQARequestedCheck) error
	DeleteCheck(ctx context.Context, id uuid.UUID) error

	// Issues
	ListIssues(ctx context.Context, filterStatus string) ([]models.ProQAIssue, error)
	GetIssue(ctx context.Context, id uuid.UUID) (*models.ProQAIssue, error)
	CreateIssue(ctx context.Context, i *models.ProQAIssue) error
	UpdateIssue(ctx context.Context, i *models.ProQAIssue) error
	ChangeIssueStatus(ctx context.Context, id uuid.UUID, newStatus string) (oldStatus string, err error)

	// Comments
	ListComments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAIssueComment, error)
	CreateComment(ctx context.Context, c *models.ProQAIssueComment) error

	// Attachments
	ListAttachments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAAttachment, error)
	CreateAttachment(ctx context.Context, a *models.ProQAAttachment) error
	GetAttachment(ctx context.Context, id uuid.UUID) (*models.ProQAAttachment, error)
}

// proQARepo routes all SQL through supportDB so the records live on the
// shared support cluster (same physical rows visible from dev and prod).
type proQARepo struct {
	supportDB *sql.DB
}

func NewProQARepo(supportDB *sql.DB) ProQARepository {
	return &proQARepo{supportDB: supportDB}
}

// ---------- Info ----------

func (r *proQARepo) GetInfo(ctx context.Context) (*models.ProQAInfo, error) {
	var info models.ProQAInfo
	var email sql.NullString
	err := r.supportDB.QueryRowContext(ctx,
		`SELECT body_md, updated_at, COALESCE(updated_by_email, '') FROM pro_qa_info WHERE id = 1`,
	).Scan(&info.BodyMD, &info.UpdatedAt, &email)
	if err != nil {
		return nil, fmt.Errorf("get pro_qa_info: %w", err)
	}
	info.UpdatedByEmail = email.String
	return &info, nil
}

func (r *proQARepo) UpdateInfo(ctx context.Context, bodyMD, email string) error {
	_, err := r.supportDB.ExecContext(ctx,
		`UPDATE pro_qa_info SET body_md = $1, updated_at = NOW(), updated_by_email = $2 WHERE id = 1`,
		bodyMD, email,
	)
	return err
}

// ---------- Requested checks ----------

func (r *proQARepo) ListChecks(ctx context.Context) ([]models.ProQARequestedCheck, error) {
	rows, err := r.supportDB.QueryContext(ctx,
		`SELECT id, title, body_md, status, sort_order, created_at, COALESCE(created_by_email,''), updated_at
		   FROM pro_qa_requested_checks
		   ORDER BY sort_order ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProQARequestedCheck
	for rows.Next() {
		var c models.ProQARequestedCheck
		if err := rows.Scan(&c.ID, &c.Title, &c.BodyMD, &c.Status, &c.SortOrder, &c.CreatedAt, &c.CreatedByEmail, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *proQARepo) CreateCheck(ctx context.Context, c *models.ProQARequestedCheck) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	_, err := r.supportDB.ExecContext(ctx,
		`INSERT INTO pro_qa_requested_checks (id, title, body_md, status, sort_order, created_at, created_by_email, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, c.Title, c.BodyMD, c.Status, c.SortOrder, c.CreatedAt, c.CreatedByEmail, c.UpdatedAt)
	return err
}

func (r *proQARepo) UpdateCheck(ctx context.Context, c *models.ProQARequestedCheck) error {
	c.UpdatedAt = time.Now()
	_, err := r.supportDB.ExecContext(ctx,
		`UPDATE pro_qa_requested_checks SET title=$1, body_md=$2, status=$3, sort_order=$4, updated_at=$5 WHERE id=$6`,
		c.Title, c.BodyMD, c.Status, c.SortOrder, c.UpdatedAt, c.ID)
	return err
}

func (r *proQARepo) DeleteCheck(ctx context.Context, id uuid.UUID) error {
	_, err := r.supportDB.ExecContext(ctx, `DELETE FROM pro_qa_requested_checks WHERE id=$1`, id)
	return err
}

// ---------- Issues ----------

const issueSelectCols = `id, issue_number, parent_issue_id, title, description_md,
       COALESCE(environment,''), COALESCE(platform,''), status, severity,
       created_at, COALESCE(created_by_email,''), updated_at, closed_at`

func (r *proQARepo) ListIssues(ctx context.Context, filterStatus string) ([]models.ProQAIssue, error) {
	q := `SELECT ` + issueSelectCols + `,
	         (SELECT COUNT(*) FROM pro_qa_issue_comments c WHERE c.issue_id = i.id) AS comment_count,
	         (SELECT COUNT(*) FROM pro_qa_issue_attachments a WHERE a.issue_id = i.id) AS attachment_count
	      FROM pro_qa_issues i`
	var rows *sql.Rows
	var err error
	if filterStatus != "" && filterStatus != "all" {
		q += ` WHERE status = $1 ORDER BY created_at DESC`
		rows, err = r.supportDB.QueryContext(ctx, q, filterStatus)
	} else {
		q += ` ORDER BY created_at DESC`
		rows, err = r.supportDB.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProQAIssue
	for rows.Next() {
		var i models.ProQAIssue
		var parent sql.NullString
		if err := rows.Scan(&i.ID, &i.IssueNumber, &parent, &i.Title, &i.DescriptionMD,
			&i.Environment, &i.Platform, &i.Status, &i.Severity,
			&i.CreatedAt, &i.CreatedByEmail, &i.UpdatedAt, &i.ClosedAt,
			&i.CommentCount, &i.AttachmentCount); err != nil {
			return nil, err
		}
		if parent.Valid {
			pid, perr := uuid.Parse(parent.String)
			if perr == nil {
				i.ParentIssueID = &pid
			}
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *proQARepo) GetIssue(ctx context.Context, id uuid.UUID) (*models.ProQAIssue, error) {
	var i models.ProQAIssue
	var parent sql.NullString
	err := r.supportDB.QueryRowContext(ctx,
		`SELECT `+issueSelectCols+` FROM pro_qa_issues WHERE id=$1`, id,
	).Scan(&i.ID, &i.IssueNumber, &parent, &i.Title, &i.DescriptionMD,
		&i.Environment, &i.Platform, &i.Status, &i.Severity,
		&i.CreatedAt, &i.CreatedByEmail, &i.UpdatedAt, &i.ClosedAt)
	if err != nil {
		return nil, err
	}
	if parent.Valid {
		pid, perr := uuid.Parse(parent.String)
		if perr == nil {
			i.ParentIssueID = &pid
		}
	}
	return &i, nil
}

func (r *proQARepo) CreateIssue(ctx context.Context, i *models.ProQAIssue) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	now := time.Now()
	i.CreatedAt = now
	i.UpdatedAt = now
	var parentParam interface{}
	if i.ParentIssueID != nil {
		parentParam = *i.ParentIssueID
	}
	err := r.supportDB.QueryRowContext(ctx,
		`INSERT INTO pro_qa_issues
		   (id, parent_issue_id, title, description_md, environment, platform, status, severity, created_at, created_by_email, updated_at)
		 VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11)
		 RETURNING issue_number`,
		i.ID, parentParam, i.Title, i.DescriptionMD, i.Environment, i.Platform, i.Status, i.Severity, i.CreatedAt, i.CreatedByEmail, i.UpdatedAt,
	).Scan(&i.IssueNumber)
	return err
}

func (r *proQARepo) UpdateIssue(ctx context.Context, i *models.ProQAIssue) error {
	i.UpdatedAt = time.Now()
	var parentParam interface{}
	if i.ParentIssueID != nil {
		parentParam = *i.ParentIssueID
	}
	_, err := r.supportDB.ExecContext(ctx,
		`UPDATE pro_qa_issues
		    SET parent_issue_id=$1, title=$2, description_md=$3,
		        environment=NULLIF($4,''), platform=NULLIF($5,''),
		        severity=$6, updated_at=$7
		  WHERE id=$8`,
		parentParam, i.Title, i.DescriptionMD, i.Environment, i.Platform, i.Severity, i.UpdatedAt, i.ID)
	return err
}

func (r *proQARepo) ChangeIssueStatus(ctx context.Context, id uuid.UUID, newStatus string) (string, error) {
	var old string
	err := r.supportDB.QueryRowContext(ctx,
		`UPDATE pro_qa_issues
		    SET status = $1,
		        updated_at = NOW(),
		        closed_at = CASE WHEN $1 IN ('resolved','closed','wont_fix') THEN NOW() ELSE NULL END
		  WHERE id = $2
		  RETURNING (SELECT status FROM pro_qa_issues WHERE id = $2)`,
		newStatus, id,
	).Scan(&old)
	// PG returns the new status above because the subquery runs after UPDATE
	// in the same statement (snapshot semantics differ by version). Use a
	// fallback two-step if the returned value matches newStatus.
	if err == nil && old == newStatus {
		// Best effort: caller will record statusFrom from a pre-fetch instead.
	}
	return old, err
}

// ---------- Comments ----------

func (r *proQARepo) ListComments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAIssueComment, error) {
	rows, err := r.supportDB.QueryContext(ctx,
		`SELECT id, issue_id, body_md, COALESCE(author_email,''), COALESCE(author_name,''),
		        created_at, is_status_change, COALESCE(status_from,''), COALESCE(status_to,'')
		   FROM pro_qa_issue_comments
		  WHERE issue_id = $1
		  ORDER BY created_at ASC`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProQAIssueComment
	for rows.Next() {
		var c models.ProQAIssueComment
		if err := rows.Scan(&c.ID, &c.IssueID, &c.BodyMD, &c.AuthorEmail, &c.AuthorName,
			&c.CreatedAt, &c.IsStatusChange, &c.StatusFrom, &c.StatusTo); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *proQARepo) CreateComment(ctx context.Context, c *models.ProQAIssueComment) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	_, err := r.supportDB.ExecContext(ctx,
		`INSERT INTO pro_qa_issue_comments (id, issue_id, body_md, author_email, author_name, created_at, is_status_change, status_from, status_to)
		 VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,NULLIF($8,''),NULLIF($9,''))`,
		c.ID, c.IssueID, c.BodyMD, c.AuthorEmail, c.AuthorName, c.CreatedAt, c.IsStatusChange, c.StatusFrom, c.StatusTo)
	return err
}

// ---------- Attachments ----------

func (r *proQARepo) ListAttachments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAAttachment, error) {
	rows, err := r.supportDB.QueryContext(ctx,
		`SELECT id, issue_id, comment_id, filename, content_type, size_bytes, storage_driver, storage_path,
		        COALESCE(uploaded_by_email,''), uploaded_at
		   FROM pro_qa_issue_attachments WHERE issue_id=$1 ORDER BY uploaded_at ASC`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProQAAttachment
	for rows.Next() {
		var a models.ProQAAttachment
		var cmt sql.NullString
		if err := rows.Scan(&a.ID, &a.IssueID, &cmt, &a.Filename, &a.ContentType, &a.SizeBytes,
			&a.StorageDriver, &a.StoragePath, &a.UploadedByEmail, &a.UploadedAt); err != nil {
			return nil, err
		}
		if cmt.Valid {
			cid, perr := uuid.Parse(cmt.String)
			if perr == nil {
				a.CommentID = &cid
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *proQARepo) CreateAttachment(ctx context.Context, a *models.ProQAAttachment) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.UploadedAt.IsZero() {
		a.UploadedAt = time.Now()
	}
	var cmt interface{}
	if a.CommentID != nil {
		cmt = *a.CommentID
	}
	_, err := r.supportDB.ExecContext(ctx,
		`INSERT INTO pro_qa_issue_attachments (id, issue_id, comment_id, filename, content_type, size_bytes, storage_driver, storage_path, uploaded_by_email, uploaded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10)`,
		a.ID, a.IssueID, cmt, a.Filename, a.ContentType, a.SizeBytes, a.StorageDriver, a.StoragePath, a.UploadedByEmail, a.UploadedAt)
	return err
}

func (r *proQARepo) GetAttachment(ctx context.Context, id uuid.UUID) (*models.ProQAAttachment, error) {
	var a models.ProQAAttachment
	var cmt sql.NullString
	err := r.supportDB.QueryRowContext(ctx,
		`SELECT id, issue_id, comment_id, filename, content_type, size_bytes, storage_driver, storage_path,
		        COALESCE(uploaded_by_email,''), uploaded_at
		   FROM pro_qa_issue_attachments WHERE id=$1`, id,
	).Scan(&a.ID, &a.IssueID, &cmt, &a.Filename, &a.ContentType, &a.SizeBytes,
		&a.StorageDriver, &a.StoragePath, &a.UploadedByEmail, &a.UploadedAt)
	if err != nil {
		return nil, err
	}
	if cmt.Valid {
		cid, perr := uuid.Parse(cmt.String)
		if perr == nil {
			a.CommentID = &cid
		}
	}
	return &a, nil
}
```

- [ ] **Step 2: Wire it into the repository registry**

Open `internal/repository/repository.go` and find the struct that bundles all repos and the `NewRepositories(db, supportDB, ...)` constructor. Add:

```go
ProQA ProQARepository
// ... inside NewRepositories ...
ProQA: NewProQARepo(supportDB),
```

(Verify the exact field placement by reading the file — it follows alphabetical / grouped order.)

- [ ] **Step 3: Verify it compiles**

```bash
export PATH=$PATH:/usr/local/go/bin && cd /home/carecomp/carecompanion && go build ./internal/repository/...
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add internal/repository/pro_qa_repository.go internal/repository/repository.go
git commit -m "feat(pro-qa): repository on shared support DB"
```

---

## Task 4: Add goldmark dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add goldmark**

```bash
export PATH=$PATH:/usr/local/go/bin && cd /home/carecomp/carecompanion && go get github.com/yuin/goldmark@v1.7.8
```

Expected: `go: added github.com/yuin/goldmark v1.7.8`.

- [ ] **Step 2: Verify build still works**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add goldmark for pro-qa markdown rendering"
```

---

## Task 5: Service

**Files:**
- Create: `internal/service/pro_qa_service.go`
- Modify: `internal/service/services.go`

- [ ] **Step 1: Write the service**

```go
package service

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// ProQAService orchestrates the admin-only QA workspace. It wraps the
// repository and owns the markdown renderer + attachment BlobStorage so
// handlers stay thin.
type ProQAService struct {
	repo    repository.ProQARepository
	storage BlobStorage
	md      goldmark.Markdown
}

func NewProQAService(repo repository.ProQARepository, storage BlobStorage) *ProQAService {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
	)
	return &ProQAService{repo: repo, storage: storage, md: md}
}

// RenderMarkdown safely converts markdown to sanitized HTML.
// goldmark escapes HTML in source by default (no rawHTML), so no extra
// sanitization is needed here.
func (s *ProQAService) RenderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}

// ---- Info ----

func (s *ProQAService) GetInfo(ctx context.Context) (*models.ProQAInfo, error) {
	return s.repo.GetInfo(ctx)
}

func (s *ProQAService) UpdateInfo(ctx context.Context, bodyMD, email string) error {
	return s.repo.UpdateInfo(ctx, bodyMD, email)
}

// ---- Checks ----

func (s *ProQAService) ListChecks(ctx context.Context) ([]models.ProQARequestedCheck, error) {
	return s.repo.ListChecks(ctx)
}

func (s *ProQAService) CreateCheck(ctx context.Context, title, bodyMD, email string) (*models.ProQARequestedCheck, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("title required")
	}
	c := &models.ProQARequestedCheck{
		Title: title, BodyMD: bodyMD, Status: "open",
		CreatedByEmail: email,
	}
	if err := s.repo.CreateCheck(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *ProQAService) UpdateCheck(ctx context.Context, c *models.ProQARequestedCheck) error {
	return s.repo.UpdateCheck(ctx, c)
}

func (s *ProQAService) DeleteCheck(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteCheck(ctx, id)
}

// ---- Issues ----

func (s *ProQAService) ListIssues(ctx context.Context, status string) ([]models.ProQAIssue, error) {
	return s.repo.ListIssues(ctx, status)
}

func (s *ProQAService) GetIssue(ctx context.Context, id uuid.UUID) (*models.ProQAIssue, error) {
	return s.repo.GetIssue(ctx, id)
}

func (s *ProQAService) CreateIssue(ctx context.Context, i *models.ProQAIssue) error {
	if strings.TrimSpace(i.Title) == "" {
		return fmt.Errorf("title required")
	}
	if i.Status == "" {
		i.Status = "open"
	}
	if i.Severity == "" {
		i.Severity = "medium"
	}
	return s.repo.CreateIssue(ctx, i)
}

func (s *ProQAService) UpdateIssue(ctx context.Context, i *models.ProQAIssue) error {
	return s.repo.UpdateIssue(ctx, i)
}

// ChangeStatus updates the issue and writes an auto-comment recording the
// transition so the thread shows a complete history.
func (s *ProQAService) ChangeStatus(ctx context.Context, issueID uuid.UUID, newStatus, authorEmail, authorName string) error {
	prev, err := s.repo.GetIssue(ctx, issueID)
	if err != nil {
		return err
	}
	if _, err := s.repo.ChangeIssueStatus(ctx, issueID, newStatus); err != nil {
		return err
	}
	autoBody := fmt.Sprintf("_status changed: **%s** → **%s**_", prev.Status, newStatus)
	return s.repo.CreateComment(ctx, &models.ProQAIssueComment{
		IssueID: issueID, BodyMD: autoBody,
		AuthorEmail: authorEmail, AuthorName: authorName,
		IsStatusChange: true, StatusFrom: prev.Status, StatusTo: newStatus,
	})
}

// ---- Comments ----

func (s *ProQAService) ListComments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAIssueComment, error) {
	return s.repo.ListComments(ctx, issueID)
}

func (s *ProQAService) AddComment(ctx context.Context, issueID uuid.UUID, body, email, name string) (*models.ProQAIssueComment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("comment body required")
	}
	c := &models.ProQAIssueComment{
		IssueID: issueID, BodyMD: body,
		AuthorEmail: email, AuthorName: name,
	}
	if err := s.repo.CreateComment(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ---- Attachments ----

func (s *ProQAService) ListAttachments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAAttachment, error) {
	return s.repo.ListAttachments(ctx, issueID)
}

func (s *ProQAService) UploadAttachment(ctx context.Context, issueID uuid.UUID, commentID *uuid.UUID, filename, contentType, email string, body io.Reader) (*models.ProQAAttachment, error) {
	path, size, err := s.storage.Save(ctx, "pro_qa", filename, contentType, body)
	if err != nil {
		return nil, err
	}
	a := &models.ProQAAttachment{
		IssueID: issueID, CommentID: commentID,
		Filename: filename, ContentType: contentType, SizeBytes: size,
		StorageDriver: s.storage.Driver(), StoragePath: path,
		UploadedByEmail: email,
	}
	if err := s.repo.CreateAttachment(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *ProQAService) FetchAttachment(ctx context.Context, id uuid.UUID) (*models.ProQAAttachment, io.ReadCloser, error) {
	a, err := s.repo.GetAttachment(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.storage.Open(ctx, a.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return a, rc, nil
}
```

- [ ] **Step 2: Wire the service into services.go**

Open `internal/service/services.go`. Find the block where `reportStorage := NewBlobStorage(...)` and other services are constructed. Add:

```go
proQAStorage := NewBlobStorage(&cfg.Storage, "pro_qa", cfg.Storage.S3Prefix+"pro-qa/")
// ... and in the Services struct + return literal ...
ProQA: NewProQAService(repos.ProQA, proQAStorage),
```

Add `ProQA *ProQAService` to the `Services` struct. Read the file first to mirror the existing style.

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/service/pro_qa_service.go internal/service/services.go
git commit -m "feat(pro-qa): service with markdown render + S3 attachments"
```

---

## Task 6: Handlers

**Files:**
- Create: `internal/handler/admin/pro_qa_handlers.go`
- Modify: `internal/handler/admin/handlers.go` (add `proQAService` field + `SetProQAService` method)

- [ ] **Step 1: Add the service slot on the Handler struct**

In `internal/handler/admin/handlers.go`, locate the `Handler` struct (around line 12) and add:

```go
proQAService *service.ProQAService
```

Then add the setter alongside the existing `SetXxxService` methods:

```go
func (h *Handler) SetProQAService(s *service.ProQAService) {
	h.proQAService = s
}
```

- [ ] **Step 2: Write the handlers file**

Create `internal/handler/admin/pro_qa_handlers.go`:

```go
package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
)

// --- Helpers ---

type proQAPageData struct {
	AdminPageData
	ActiveTab string
}

func (h *Handler) proQAData(r *http.Request, title, activeTab string) proQAPageData {
	claims := middleware.GetAuthClaims(r.Context())
	return proQAPageData{
		AdminPageData: AdminPageData{
			Title: title,
			CurrentUser: AdminUser{
				ID:         claims.UserID,
				Email:      claims.Email,
				FirstName:  claims.FirstName,
				SystemRole: string(claims.SystemRole),
			},
		},
		ActiveTab: activeTab,
	}
}

func (h *Handler) renderProQA(w http.ResponseWriter, tmplName string, data interface{}) {
	// Each pro_qa page composes layout.html + pro_qa_layout.html + the
	// specific tab template. parseTemplates is the existing admin helper.
	tmpl, err := parseTemplates("layout.html", "pro_qa_layout.html", tmplName)
	if err != nil {
		http.Error(w, "template parse: "+err.Error(), 500)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "template exec: "+err.Error(), 500)
	}
}

// --- Intro ---

func (h *Handler) ProQAIntroPage(w http.ResponseWriter, r *http.Request) {
	data := h.proQAData(r, "Pro QA — Intro", "intro")
	h.renderProQA(w, "pro_qa_intro.html", data)
}

// --- Info ---

type proQAInfoView struct {
	proQAPageData
	Info     *models.ProQAInfo
	BodyHTML interface{}
}

func (h *Handler) ProQAInfoPage(w http.ResponseWriter, r *http.Request) {
	info, err := h.proQAService.GetInfo(r.Context())
	if err != nil {
		http.Error(w, "load info: "+err.Error(), 500)
		return
	}
	v := proQAInfoView{
		proQAPageData: h.proQAData(r, "Pro QA — Info", "info"),
		Info:          info,
		BodyHTML:      h.proQAService.RenderMarkdown(info.BodyMD),
	}
	h.renderProQA(w, "pro_qa_info.html", v)
}

func (h *Handler) ProQAInfoSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	body := r.FormValue("body_md")
	claims := middleware.GetAuthClaims(r.Context())
	if err := h.proQAService.UpdateInfo(r.Context(), body, claims.Email); err != nil {
		http.Error(w, "save: "+err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/info", http.StatusSeeOther)
}

// --- Requested checks ---

type proQAChecksView struct {
	proQAPageData
	Checks []models.ProQARequestedCheck
	// RenderedBodies[checkID] = rendered HTML
	RenderedBodies map[uuid.UUID]interface{}
	Statuses       []string
}

func (h *Handler) ProQAChecksPage(w http.ResponseWriter, r *http.Request) {
	checks, err := h.proQAService.ListChecks(r.Context())
	if err != nil {
		http.Error(w, "load checks: "+err.Error(), 500)
		return
	}
	rendered := make(map[uuid.UUID]interface{}, len(checks))
	for _, c := range checks {
		rendered[c.ID] = h.proQAService.RenderMarkdown(c.BodyMD)
	}
	v := proQAChecksView{
		proQAPageData:  h.proQAData(r, "Pro QA — Requested Checks", "checks"),
		Checks:         checks,
		RenderedBodies: rendered,
		Statuses:       models.ProQACheckStatuses,
	}
	h.renderProQA(w, "pro_qa_checks.html", v)
}

func (h *Handler) ProQAChecksCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	if _, err := h.proQAService.CreateCheck(r.Context(),
		strings.TrimSpace(r.FormValue("title")),
		r.FormValue("body_md"),
		claims.Email); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/checks", http.StatusSeeOther)
}

func (h *Handler) ProQAChecksUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	c := &models.ProQARequestedCheck{
		ID:        id,
		Title:     strings.TrimSpace(r.FormValue("title")),
		BodyMD:    r.FormValue("body_md"),
		Status:    r.FormValue("status"),
		SortOrder: sortOrder,
	}
	if err := h.proQAService.UpdateCheck(r.Context(), c); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/checks", http.StatusSeeOther)
}

func (h *Handler) ProQAChecksDelete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	if err := h.proQAService.DeleteCheck(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/checks", http.StatusSeeOther)
}

// --- Issues ---

type proQAIssuesView struct {
	proQAPageData
	Issues       []models.ProQAIssue
	FilterStatus string
	Statuses     []string
	Severities   []string
	Envs         []string
	Platforms    []string
}

func (h *Handler) ProQAIssuesPage(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("status")
	issues, err := h.proQAService.ListIssues(r.Context(), filter)
	if err != nil {
		http.Error(w, "load: "+err.Error(), 500)
		return
	}
	v := proQAIssuesView{
		proQAPageData: h.proQAData(r, "Pro QA — Issues", "issues"),
		Issues:        issues,
		FilterStatus:  filter,
		Statuses:      models.ProQAIssueStatuses,
		Severities:    models.ProQAIssueSeverity,
		Envs:          models.ProQAEnvironments,
		Platforms:     models.ProQAPlatforms,
	}
	h.renderProQA(w, "pro_qa_issues.html", v)
}

func (h *Handler) ProQAIssueCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	issue := &models.ProQAIssue{
		Title:          strings.TrimSpace(r.FormValue("title")),
		DescriptionMD:  r.FormValue("description_md"),
		Environment:    r.FormValue("environment"),
		Platform:       r.FormValue("platform"),
		Severity:       r.FormValue("severity"),
		Status:         "open",
		CreatedByEmail: claims.Email,
	}
	if parent := r.FormValue("parent_issue_id"); parent != "" {
		if pid, perr := uuid.Parse(parent); perr == nil {
			issue.ParentIssueID = &pid
		}
	}
	if err := h.proQAService.CreateIssue(r.Context(), issue); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+issue.ID.String(), http.StatusSeeOther)
}

type proQAIssueDetailView struct {
	proQAPageData
	Issue          *models.ProQAIssue
	DescriptionHTML interface{}
	Comments       []models.ProQAIssueComment
	RenderedComments map[uuid.UUID]interface{}
	Attachments    []models.ProQAAttachment
	Statuses       []string
	Severities     []string
	Envs           []string
	Platforms      []string
}

func (h *Handler) ProQAIssueDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	issue, err := h.proQAService.GetIssue(r.Context(), id)
	if err != nil {
		http.Error(w, "load: "+err.Error(), 500)
		return
	}
	comments, err := h.proQAService.ListComments(r.Context(), id)
	if err != nil {
		http.Error(w, "comments: "+err.Error(), 500)
		return
	}
	attachments, err := h.proQAService.ListAttachments(r.Context(), id)
	if err != nil {
		http.Error(w, "attachments: "+err.Error(), 500)
		return
	}
	rc := make(map[uuid.UUID]interface{}, len(comments))
	for _, c := range comments {
		rc[c.ID] = h.proQAService.RenderMarkdown(c.BodyMD)
	}
	v := proQAIssueDetailView{
		proQAPageData:    h.proQAData(r, "Pro QA — Issue #"+strconv.Itoa(issue.IssueNumber), "issues"),
		Issue:            issue,
		DescriptionHTML:  h.proQAService.RenderMarkdown(issue.DescriptionMD),
		Comments:         comments,
		RenderedComments: rc,
		Attachments:      attachments,
		Statuses:         models.ProQAIssueStatuses,
		Severities:       models.ProQAIssueSeverity,
		Envs:             models.ProQAEnvironments,
		Platforms:        models.ProQAPlatforms,
	}
	h.renderProQA(w, "pro_qa_issue_detail.html", v)
}

func (h *Handler) ProQAIssueUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	cur, err := h.proQAService.GetIssue(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	cur.Title = strings.TrimSpace(r.FormValue("title"))
	cur.DescriptionMD = r.FormValue("description_md")
	cur.Environment = r.FormValue("environment")
	cur.Platform = r.FormValue("platform")
	cur.Severity = r.FormValue("severity")
	if parent := r.FormValue("parent_issue_id"); parent != "" {
		if pid, perr := uuid.Parse(parent); perr == nil {
			cur.ParentIssueID = &pid
		}
	} else {
		cur.ParentIssueID = nil
	}
	if err := h.proQAService.UpdateIssue(r.Context(), cur); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+id.String(), http.StatusSeeOther)
}

func (h *Handler) ProQAIssueChangeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	newStatus := r.FormValue("status")
	claims := middleware.GetAuthClaims(r.Context())
	if err := h.proQAService.ChangeStatus(r.Context(), id, newStatus, claims.Email, claims.FirstName); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+id.String(), http.StatusSeeOther)
}

func (h *Handler) ProQAIssueComment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	if _, err := h.proQAService.AddComment(r.Context(), id, r.FormValue("body_md"), claims.Email, claims.FirstName); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+id.String()+"#comments", http.StatusSeeOther)
}

// ProQAUploadAttachment accepts multipart/form-data with field "file".
// Returns JSON: {id, filename, url}.
func (h *Handler) ProQAUploadAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	if err := r.ParseMultipartForm(20 << 20); err != nil { // 20 MB max
		http.Error(w, "bad upload: "+err.Error(), 400)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file", 400)
		return
	}
	defer file.Close()
	claims := middleware.GetAuthClaims(r.Context())
	att, err := h.proQAService.UploadAttachment(r.Context(), id, nil,
		hdr.Filename, hdr.Header.Get("Content-Type"), claims.Email, file)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":       att.ID.String(),
		"filename": att.Filename,
		"url":      "/admin/pro-qa/attachments/" + att.ID.String(),
	})
}

func (h *Handler) ProQAFetchAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	att, rc, err := h.proQAService.FetchAttachment(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+att.Filename+`"`)
	_, _ = io.Copy(w, rc)
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./internal/handler/admin/...
```

Expected: no errors. (Routes wiring next task — handlers will be unreferenced for now, which `go build` allows because they're exported.)

- [ ] **Step 4: Commit**

```bash
git add internal/handler/admin/pro_qa_handlers.go internal/handler/admin/handlers.go
git commit -m "feat(pro-qa): admin HTTP handlers"
```

---

## Task 7: Routes + service wiring in main.go

**Files:**
- Modify: `internal/handler/admin/routes.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add the route group**

Read `routes.go` around line 192 (the existing `RequireSuperAdmin` block for system settings). Add a NEW group anywhere inside the main `Route("/admin", ...)` function:

```go
// Pro QA workspace (super_admin only for now; finer-grained role gate
// planned for a later slice once the role-builder UI ships).
r.Route("/pro-qa", func(r chi.Router) {
	r.Use(middleware.RequireSuperAdmin())

	r.Get("/", h.ProQAIntroPage)
	r.Get("/intro", h.ProQAIntroPage)

	r.Get("/info", h.ProQAInfoPage)
	r.Post("/info", h.ProQAInfoSave)

	r.Get("/checks", h.ProQAChecksPage)
	r.Post("/checks", h.ProQAChecksCreate)
	r.Post("/checks/{id}", h.ProQAChecksUpdate)
	r.Post("/checks/{id}/delete", h.ProQAChecksDelete)

	r.Get("/issues", h.ProQAIssuesPage)
	r.Post("/issues", h.ProQAIssueCreate)
	r.Get("/issues/{id}", h.ProQAIssueDetailPage)
	r.Post("/issues/{id}", h.ProQAIssueUpdate)
	r.Post("/issues/{id}/status", h.ProQAIssueChangeStatus)
	r.Post("/issues/{id}/comment", h.ProQAIssueComment)
	r.Post("/issues/{id}/attach", h.ProQAUploadAttachment)

	r.Get("/attachments/{id}", h.ProQAFetchAttachment)
})
```

- [ ] **Step 2: Wire the service in main.go**

In `cmd/server/main.go`, find where other admin services get attached (e.g., `adminHandler.SetBetaService(...)`) and add:

```go
adminHandler.SetProQAService(services.ProQA)
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/handler/admin/routes.go cmd/server/main.go
git commit -m "feat(pro-qa): wire routes + service into admin handler"
```

---

## Task 8: Templates

**Files:**
- Create: `templates/admin/pro_qa_layout.html`
- Create: `templates/admin/pro_qa_intro.html`
- Create: `templates/admin/pro_qa_info.html`
- Create: `templates/admin/pro_qa_checks.html`
- Create: `templates/admin/pro_qa_issues.html`
- Create: `templates/admin/pro_qa_issue_detail.html`

- [ ] **Step 1: Write pro_qa_layout.html (shared sub-nav shell)**

```html
{{define "content"}}
<div class="p-6 max-w-6xl">
  <div class="mb-6">
    <h1 class="text-2xl font-bold text-gray-900">Pro QA Workspace</h1>
    <p class="text-sm text-gray-600">Private workspace for the paid QA engagement. Visible only to super admins (and the assigned QA tester once role-based access ships).</p>
  </div>

  <!-- Sub-nav tabs -->
  <div class="border-b border-gray-200 mb-6">
    <nav class="-mb-px flex space-x-6">
      {{$tab := .ActiveTab}}
      {{range $t := slice "intro" "info" "checks" "issues"}}
        {{$active := eq $tab $t}}
        <a href="/admin/pro-qa/{{$t}}"
           class="py-2 px-1 border-b-2 text-sm font-medium {{if $active}}border-indigo-600 text-indigo-700{{else}}border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300{{end}}">
          {{if eq $t "intro"}}Intro{{end}}
          {{if eq $t "info"}}Info{{end}}
          {{if eq $t "checks"}}Requested Checks{{end}}
          {{if eq $t "issues"}}Issue Tracker{{end}}
        </a>
      {{end}}
    </nav>
  </div>

  {{template "pro_qa_body" .}}
</div>
<script src="/static/js/pro_qa.js"></script>
{{end}}
```

Note: `slice` is a built-in template function in Go's `html/template`. If it's not (Go's html/template does NOT include `slice` natively — only `text/template` from go-text/template), substitute with explicit tabs:

```html
<a href="/admin/pro-qa/intro"   class="py-2 px-1 border-b-2 text-sm font-medium {{if eq .ActiveTab "intro"}}border-indigo-600 text-indigo-700{{else}}border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300{{end}}">Intro</a>
<a href="/admin/pro-qa/info"    class="py-2 px-1 border-b-2 text-sm font-medium {{if eq .ActiveTab "info"}}border-indigo-600 text-indigo-700{{else}}border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300{{end}}">Info</a>
<a href="/admin/pro-qa/checks"  class="py-2 px-1 border-b-2 text-sm font-medium {{if eq .ActiveTab "checks"}}border-indigo-600 text-indigo-700{{else}}border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300{{end}}">Requested Checks</a>
<a href="/admin/pro-qa/issues"  class="py-2 px-1 border-b-2 text-sm font-medium {{if eq .ActiveTab "issues"}}border-indigo-600 text-indigo-700{{else}}border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300{{end}}">Issue Tracker</a>
```

Use the explicit form. Replace the loop in the snippet above.

- [ ] **Step 2: Write pro_qa_intro.html**

```html
{{define "pro_qa_body"}}
<div class="prose max-w-none">
  <h2>Welcome to the Pro QA Workspace</h2>
  <p>This area is a private workspace for Bryan and the assigned QA tester. Nothing here is visible to the general admin team, parents, or support staff.</p>
  <h3>What's in each tab</h3>
  <ul>
    <li><strong>Info</strong> — Shared rich text area. Use it to record test account credentials, environment URLs, how to set up TestFlight, and any other reference material that both of us should be able to update freely.</li>
    <li><strong>Requested Checks</strong> — A working list of things to test. Bryan adds items he wants specifically verified or knows are risky; the QA tester marks each as in-review or done as work progresses.</li>
    <li><strong>Issue Tracker</strong> — Private bidirectional ticketing. Either of us can open an issue, comment back and forth, attach screenshots, change status, and link parent/child issues. Same data is visible from dev and prod.</li>
  </ul>
  <h3>Conventions</h3>
  <ul>
    <li>All text fields accept <strong>Markdown</strong>. Use the <em>Preview</em> tab to verify formatting before saving.</li>
    <li>Image and PDF attachments up to 20 MB are stored in S3.</li>
    <li>Use the <strong>Environment</strong> field on issues to record where you observed the problem (dev vs prod) — both share this workspace.</li>
  </ul>
</div>
{{end}}
```

- [ ] **Step 3: Write pro_qa_info.html**

```html
{{define "pro_qa_body"}}
<form method="POST" action="/admin/pro-qa/info" class="space-y-4">
  <div class="bg-white border rounded-lg p-4">
    <div class="flex items-center justify-between mb-2">
      <h2 class="font-semibold">Shared Info</h2>
      <div class="text-xs text-gray-500">
        {{if .Info.UpdatedByEmail}}Last updated by {{.Info.UpdatedByEmail}} • {{.Info.UpdatedAt.Format "2006-01-02 15:04 MST"}}{{end}}
      </div>
    </div>

    <div class="mb-2 flex space-x-2 text-sm">
      <button type="button" data-pq-tab="edit" class="pq-tab-btn px-3 py-1 rounded bg-gray-200">Edit</button>
      <button type="button" data-pq-tab="preview" class="pq-tab-btn px-3 py-1 rounded">Preview</button>
    </div>

    <div data-pq-pane="edit">
      <textarea name="body_md" rows="20" class="w-full border rounded p-2 font-mono text-sm">{{.Info.BodyMD}}</textarea>
    </div>
    <div data-pq-pane="preview" class="hidden prose max-w-none border rounded p-4 bg-gray-50">
      {{.BodyHTML}}
    </div>
  </div>
  <div class="flex justify-end">
    <button type="submit" class="px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">Save</button>
  </div>
</form>
{{end}}
```

- [ ] **Step 4: Write pro_qa_checks.html**

```html
{{define "pro_qa_body"}}
<div class="space-y-6">
  <div class="bg-white border rounded-lg p-4">
    <h2 class="font-semibold mb-3">Add a check</h2>
    <form method="POST" action="/admin/pro-qa/checks" class="space-y-3">
      <input name="title" required placeholder="What should she test?" class="w-full border rounded p-2"/>
      <textarea name="body_md" rows="4" placeholder="(optional) details, repro steps, links" class="w-full border rounded p-2 font-mono text-sm"></textarea>
      <button type="submit" class="px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">Add check</button>
    </form>
  </div>

  <div class="bg-white border rounded-lg divide-y">
    {{if not .Checks}}
      <div class="p-6 text-gray-500 italic">No checks yet.</div>
    {{end}}
    {{range $c := .Checks}}
      <details class="p-4">
        <summary class="cursor-pointer flex items-center justify-between">
          <div>
            <span class="font-medium">{{$c.Title}}</span>
            <span class="ml-2 text-xs px-2 py-0.5 rounded
              {{if eq $c.Status "open"}}bg-yellow-100 text-yellow-800{{end}}
              {{if eq $c.Status "in_review"}}bg-blue-100 text-blue-800{{end}}
              {{if eq $c.Status "done"}}bg-green-100 text-green-800{{end}}">{{$c.Status}}</span>
          </div>
          <span class="text-xs text-gray-500">{{$c.CreatedAt.Format "Jan 2"}}</span>
        </summary>
        <div class="mt-3 prose max-w-none">{{index $.RenderedBodies $c.ID}}</div>

        <form method="POST" action="/admin/pro-qa/checks/{{$c.ID}}" class="mt-4 space-y-2 border-t pt-3">
          <input name="title" value="{{$c.Title}}" class="w-full border rounded p-2"/>
          <textarea name="body_md" rows="4" class="w-full border rounded p-2 font-mono text-sm">{{$c.BodyMD}}</textarea>
          <div class="flex items-center space-x-2">
            <label class="text-sm">Status:
              <select name="status" class="border rounded p-1">
                {{range $s := $.Statuses}}
                  <option value="{{$s}}" {{if eq $s $c.Status}}selected{{end}}>{{$s}}</option>
                {{end}}
              </select>
            </label>
            <label class="text-sm">Sort:
              <input type="number" name="sort_order" value="{{$c.SortOrder}}" class="border rounded p-1 w-16"/>
            </label>
            <button type="submit" class="px-3 py-1 bg-indigo-600 text-white rounded text-sm">Save</button>
          </div>
        </form>
        <form method="POST" action="/admin/pro-qa/checks/{{$c.ID}}/delete" class="mt-2"
              onsubmit="return confirm('Delete this check?')">
          <button type="submit" class="text-xs text-red-600 hover:underline">Delete</button>
        </form>
      </details>
    {{end}}
  </div>
</div>
{{end}}
```

- [ ] **Step 5: Write pro_qa_issues.html**

```html
{{define "pro_qa_body"}}
<div class="space-y-6">
  <div class="bg-white border rounded-lg p-4">
    <h2 class="font-semibold mb-3">Open a new issue</h2>
    <form method="POST" action="/admin/pro-qa/issues" class="space-y-2">
      <input name="title" required placeholder="Short summary" class="w-full border rounded p-2"/>
      <textarea name="description_md" rows="5" placeholder="Detailed description (Markdown)" class="w-full border rounded p-2 font-mono text-sm"></textarea>
      <div class="grid grid-cols-3 gap-2">
        <select name="environment" class="border rounded p-2">
          <option value="">-- env --</option>
          {{range $e := .Envs}}<option value="{{$e}}">{{$e}}</option>{{end}}
        </select>
        <select name="platform" class="border rounded p-2">
          <option value="">-- platform --</option>
          {{range $p := .Platforms}}<option value="{{$p}}">{{$p}}</option>{{end}}
        </select>
        <select name="severity" class="border rounded p-2">
          {{range $s := .Severities}}<option value="{{$s}}" {{if eq $s "medium"}}selected{{end}}>{{$s}}</option>{{end}}
        </select>
      </div>
      <input name="parent_issue_id" placeholder="(optional) parent issue UUID" class="w-full border rounded p-2 text-xs"/>
      <button type="submit" class="px-4 py-2 bg-indigo-600 text-white rounded hover:bg-indigo-700">Create issue</button>
    </form>
  </div>

  <div class="bg-white border rounded-lg">
    <div class="p-3 border-b flex items-center space-x-2 text-sm">
      <span>Filter:</span>
      <a href="/admin/pro-qa/issues" class="px-2 py-1 rounded {{if not .FilterStatus}}bg-gray-200{{end}}">all</a>
      {{range $s := .Statuses}}
        <a href="/admin/pro-qa/issues?status={{$s}}" class="px-2 py-1 rounded {{if eq $.FilterStatus $s}}bg-gray-200{{end}}">{{$s}}</a>
      {{end}}
    </div>
    <table class="w-full text-sm">
      <thead class="bg-gray-50 text-xs uppercase text-gray-500">
        <tr>
          <th class="p-2 text-left">#</th>
          <th class="p-2 text-left">Title</th>
          <th class="p-2">Status</th>
          <th class="p-2">Sev</th>
          <th class="p-2">Env</th>
          <th class="p-2">Platform</th>
          <th class="p-2">Comments</th>
          <th class="p-2">Created</th>
        </tr>
      </thead>
      <tbody class="divide-y">
        {{if not .Issues}}
          <tr><td colspan="8" class="p-6 text-center text-gray-500 italic">No issues yet.</td></tr>
        {{end}}
        {{range $i := .Issues}}
          <tr class="hover:bg-gray-50">
            <td class="p-2 text-gray-400">#{{$i.IssueNumber}}</td>
            <td class="p-2"><a href="/admin/pro-qa/issues/{{$i.ID}}" class="text-indigo-700 hover:underline">{{$i.Title}}</a>
              {{if $i.ParentIssueID}}<span class="ml-1 text-xs text-gray-400">(child)</span>{{end}}
            </td>
            <td class="p-2 text-center"><span class="text-xs px-2 py-0.5 rounded
              {{if eq $i.Status "open"}}bg-yellow-100 text-yellow-800{{end}}
              {{if eq $i.Status "needs_info"}}bg-orange-100 text-orange-800{{end}}
              {{if eq $i.Status "in_progress"}}bg-blue-100 text-blue-800{{end}}
              {{if eq $i.Status "resolved"}}bg-green-100 text-green-800{{end}}
              {{if eq $i.Status "closed"}}bg-gray-200 text-gray-700{{end}}
              {{if eq $i.Status "wont_fix"}}bg-gray-200 text-gray-700{{end}}">{{$i.Status}}</span></td>
            <td class="p-2 text-center text-xs">{{$i.Severity}}</td>
            <td class="p-2 text-center text-xs">{{$i.Environment}}</td>
            <td class="p-2 text-center text-xs">{{$i.Platform}}</td>
            <td class="p-2 text-center text-xs">{{$i.CommentCount}}</td>
            <td class="p-2 text-center text-xs text-gray-500">{{$i.CreatedAt.Format "Jan 2"}}</td>
          </tr>
        {{end}}
      </tbody>
    </table>
  </div>
</div>
{{end}}
```

- [ ] **Step 6: Write pro_qa_issue_detail.html**

```html
{{define "pro_qa_body"}}
<div class="space-y-6">
  <div class="bg-white border rounded-lg p-4">
    <div class="flex items-start justify-between">
      <div>
        <div class="text-xs text-gray-500">Issue #{{.Issue.IssueNumber}}</div>
        <h2 class="text-xl font-semibold">{{.Issue.Title}}</h2>
        <div class="text-xs text-gray-500 mt-1">
          opened {{.Issue.CreatedAt.Format "Jan 2 15:04"}}{{if .Issue.CreatedByEmail}} by {{.Issue.CreatedByEmail}}{{end}}
        </div>
      </div>
      <form method="POST" action="/admin/pro-qa/issues/{{.Issue.ID}}/status" class="flex items-center space-x-2">
        <select name="status" class="border rounded p-1 text-sm">
          {{range $s := .Statuses}}
            <option value="{{$s}}" {{if eq $s $.Issue.Status}}selected{{end}}>{{$s}}</option>
          {{end}}
        </select>
        <button type="submit" class="px-3 py-1 bg-indigo-600 text-white rounded text-sm">Change status</button>
      </form>
    </div>

    <div class="mt-3 text-xs space-x-3 text-gray-600">
      <span>Severity: <strong>{{.Issue.Severity}}</strong></span>
      <span>Env: <strong>{{or .Issue.Environment "—"}}</strong></span>
      <span>Platform: <strong>{{or .Issue.Platform "—"}}</strong></span>
      {{if .Issue.ParentIssueID}}<span>Parent: {{.Issue.ParentIssueID}}</span>{{end}}
    </div>

    <div class="mt-4 prose max-w-none border-t pt-3">{{.DescriptionHTML}}</div>

    <details class="mt-4">
      <summary class="cursor-pointer text-sm text-gray-600">Edit issue</summary>
      <form method="POST" action="/admin/pro-qa/issues/{{.Issue.ID}}" class="mt-3 space-y-2">
        <input name="title" value="{{.Issue.Title}}" class="w-full border rounded p-2"/>
        <textarea name="description_md" rows="6" class="w-full border rounded p-2 font-mono text-sm">{{.Issue.DescriptionMD}}</textarea>
        <div class="grid grid-cols-3 gap-2">
          <select name="environment" class="border rounded p-2">
            <option value="">-- env --</option>
            {{range $e := .Envs}}<option value="{{$e}}" {{if eq $e $.Issue.Environment}}selected{{end}}>{{$e}}</option>{{end}}
          </select>
          <select name="platform" class="border rounded p-2">
            <option value="">-- platform --</option>
            {{range $p := .Platforms}}<option value="{{$p}}" {{if eq $p $.Issue.Platform}}selected{{end}}>{{$p}}</option>{{end}}
          </select>
          <select name="severity" class="border rounded p-2">
            {{range $s := .Severities}}<option value="{{$s}}" {{if eq $s $.Issue.Severity}}selected{{end}}>{{$s}}</option>{{end}}
          </select>
        </div>
        <input name="parent_issue_id" value="{{if .Issue.ParentIssueID}}{{.Issue.ParentIssueID}}{{end}}" placeholder="(optional) parent issue UUID" class="w-full border rounded p-2 text-xs"/>
        <button type="submit" class="px-3 py-1 bg-indigo-600 text-white rounded text-sm">Save</button>
      </form>
    </details>
  </div>

  <div id="comments" class="bg-white border rounded-lg p-4">
    <h3 class="font-semibold mb-3">Discussion</h3>
    {{if not .Comments}}<div class="text-sm text-gray-500 italic mb-3">No comments yet.</div>{{end}}
    <ul class="space-y-3">
      {{range $c := .Comments}}
        <li class="border-l-4 {{if $c.IsStatusChange}}border-gray-300 bg-gray-50{{else}}border-indigo-400{{end}} p-3 rounded-r">
          <div class="text-xs text-gray-500 mb-1">
            <strong>{{or $c.AuthorName $c.AuthorEmail "system"}}</strong> • {{$c.CreatedAt.Format "Jan 2 15:04"}}
          </div>
          <div class="prose max-w-none text-sm">{{index $.RenderedComments $c.ID}}</div>
        </li>
      {{end}}
    </ul>

    <form method="POST" action="/admin/pro-qa/issues/{{.Issue.ID}}/comment" class="mt-4 border-t pt-3 space-y-2">
      <textarea name="body_md" rows="4" placeholder="Add a comment (Markdown)" required class="w-full border rounded p-2 font-mono text-sm"></textarea>
      <button type="submit" class="px-3 py-1 bg-indigo-600 text-white rounded text-sm">Post comment</button>
    </form>
  </div>

  <div class="bg-white border rounded-lg p-4">
    <h3 class="font-semibold mb-3">Attachments</h3>
    {{if not .Attachments}}<div class="text-sm text-gray-500 italic mb-3">No attachments yet.</div>{{end}}
    <ul class="space-y-2 mb-4">
      {{range $a := .Attachments}}
        <li class="text-sm">
          <a href="/admin/pro-qa/attachments/{{$a.ID}}" target="_blank" class="text-indigo-700 hover:underline">{{$a.Filename}}</a>
          <span class="text-xs text-gray-500">({{$a.ContentType}}, {{$a.SizeBytes}} bytes)</span>
        </li>
      {{end}}
    </ul>
    <form id="pq-upload-form" enctype="multipart/form-data" data-issue-id="{{.Issue.ID}}" class="flex items-center space-x-2">
      <input type="file" name="file" required class="text-sm"/>
      <button type="submit" class="px-3 py-1 bg-indigo-600 text-white rounded text-sm">Upload</button>
      <span id="pq-upload-status" class="text-xs text-gray-500"></span>
    </form>
  </div>
</div>
{{end}}
```

- [ ] **Step 7: Verify all templates parse**

```bash
go build ./... && ./scripts/dev.sh
```

Then `curl -s -o /dev/null -w "%{http_code}\n" -b "$DEV_COOKIES" https://dev.mycarecompanion.net/admin/pro-qa/intro` after logging in. Expected `200`.

- [ ] **Step 8: Commit**

```bash
git add templates/admin/pro_qa_*.html
git commit -m "feat(pro-qa): admin templates for intro, info, checks, issues"
```

---

## Task 9: JS helper for upload + markdown preview toggle

**Files:**
- Create: `static/js/pro_qa.js`

- [ ] **Step 1: Write the JS**

```javascript
(function () {
  // ---- Markdown preview toggle on Info page ----
  document.addEventListener('click', function (e) {
    const btn = e.target.closest('[data-pq-tab]');
    if (!btn) return;
    const tab = btn.dataset.pqTab;
    document.querySelectorAll('[data-pq-pane]').forEach(p => {
      p.classList.toggle('hidden', p.dataset.pqPane !== tab);
    });
    document.querySelectorAll('.pq-tab-btn').forEach(b => {
      b.classList.toggle('bg-gray-200', b.dataset.pqTab === tab);
    });
  });

  // ---- Attachment upload (issue detail page) ----
  const form = document.getElementById('pq-upload-form');
  if (!form) return;
  form.addEventListener('submit', async function (e) {
    e.preventDefault();
    const issueId = form.dataset.issueId;
    const status = document.getElementById('pq-upload-status');
    const fd = new FormData(form);
    status.textContent = 'Uploading…';
    try {
      const res = await fetch(`/admin/pro-qa/issues/${issueId}/attach`, {
        method: 'POST',
        body: fd,
        credentials: 'same-origin',
      });
      if (!res.ok) throw new Error(await res.text());
      status.textContent = 'Uploaded. Reloading…';
      setTimeout(() => location.reload(), 400);
    } catch (err) {
      status.textContent = 'Upload failed: ' + err.message;
    }
  });
})();
```

- [ ] **Step 2: Commit**

```bash
git add static/js/pro_qa.js
git commit -m "feat(pro-qa): markdown preview toggle + attachment upload JS"
```

---

## Task 10: Sidebar nav link

**Files:**
- Modify: `templates/admin/layout.html`

- [ ] **Step 1: Add the Pro QA group**

Insert this block between the Roadmap group (around line 123) and the Finance group:

```html
{{if eq $role "super_admin"}}
<div class="mb-6">
    <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">Pro QA</h3>
    <a href="/admin/pro-qa/intro" class="block px-3 py-2 rounded hover:bg-gray-100">Pro QA Workspace</a>
</div>
{{end}}
```

- [ ] **Step 2: Reload admin home in browser → verify "Pro QA" appears in sidebar; click it; verify Intro page renders with sub-nav tabs**

- [ ] **Step 3: Commit**

```bash
git add templates/admin/layout.html
git commit -m "feat(pro-qa): sidebar link visible to super_admin only"
```

---

## Task 11: End-to-end smoke test on dev

- [ ] **Step 1: Restart dev and log in as bryan@bluebonnettech.com**

```bash
./scripts/dev-stop.sh && ./scripts/dev.sh
```

Then in browser: log in to `https://dev.mycarecompanion.net/admin/login`.

- [ ] **Step 2: Walk all four tabs and CRUD paths**

Manual checklist:

1. **Intro** — page loads, sub-nav highlights "Intro" tab.
2. **Info** — type some Markdown into the textarea (e.g., `## Accounts\n- joe@test.com / TestPass1!`), click Save. Reload. Verify it persisted. Click Preview tab → verify HTML renders correctly.
3. **Requested Checks** — Add a check titled "Verify chat report flag opens modal". Save. Edit it, change status to `in_review`, save. Verify the status badge updates.
4. **Issue Tracker → Create issue** — Open issue titled "Sleep chart shows duplicate day", env=prod, platform=ios, severity=high. Verify redirect to detail page with `#1`. Description renders as HTML.
5. **Issue detail → status change** — Change status to `in_progress`. Verify auto-comment appears: "status changed: open → in_progress".
6. **Issue detail → comment** — Add a comment with Markdown: `Trying to reproduce. **Stuck** — [link](https://example.com)`. Verify renders correctly.
7. **Issue detail → attachment** — Upload a small PNG. Verify the row appears in Attachments. Click the filename — image should render in a new tab.
8. **Cross-env verify** — In a separate terminal connect to prod RDS (read-only) and `SELECT count(*) FROM pro_qa_issues;` — confirm `1`. This proves the dev write hit the shared support DB.

- [ ] **Step 3: Authorization spot-check**

While logged in as a non-super-admin (use the `joe@workmaninsurancegroup.com` admin account if available or any non-super_admin admin), visit `/admin/pro-qa/intro` directly. Expected: 403 with `section_…_denied` or similar — definitely NOT a 200.

Also confirm: non-super-admin admin pages do NOT show the Pro QA sidebar item.

- [ ] **Step 4: Commit any fixes**

If smoke test surfaces bugs, fix and commit them as `fix(pro-qa): …` separate from the feature commits.

---

## Task 12: Deploy notes + prod cutover

**Files:**
- Create: `docs/deploys/2026-05-22-pro-qa.md`

- [ ] **Step 1: Write the deploy doc**

```markdown
# 2026-05-22 — Pro QA admin section

## Summary
New `/admin/pro-qa/*` super-admin-only workspace. Data lives on the shared
support DB (prod RDS) so dev and prod see the same rows.

## Pre-deploy: prod RDS schema + role grants
The migration runner DOES apply 00039 to the main DB on each environment;
on prod this IS the support DB, so the tables get created during deploy.
Dev's local Docker DB also gets the tables (harmless — dev's repo routes
support queries through SUPPORT_DB_DSN).

**However**, the GRANT for `carecomp_support_dev` must be applied by hand
once, against prod RDS:

```sql
GRANT SELECT, INSERT, UPDATE, DELETE ON
  pro_qa_info, pro_qa_requested_checks, pro_qa_issues,
  pro_qa_issue_comments, pro_qa_issue_attachments
TO carecomp_support_dev;
GRANT USAGE, SELECT ON SEQUENCE pro_qa_issues_issue_number_seq TO carecomp_support_dev;
```

Run via superadmin temp creds (see `reference_prod_db_access.md`).

## Deploy steps
1. Confirm master is at the right tip and all 9 commits land.
2. `./scripts/deploy.sh` (three DEPLOY confirmations).
3. After ASG instance refresh completes, verify:
   - `https://www.mycarecompanion.net/admin/pro-qa/intro` returns 200 as
     bryan@bluebonnettech.com.
   - As any other admin role, returns 403.
   - First test issue created on prod is visible from dev after refresh,
     and vice versa.

## Rollback
- Code: revert the feature commits and redeploy.
- Schema: tables are isolated; safe to leave in place. If hard cleanup
  needed, run the rollback DROP block at the top of
  `migrations/00039_pro_qa.sql` against prod RDS.

## S3
A new prefix `pro-qa/` will appear under the configured bucket. No new
bucket needed.
```

- [ ] **Step 2: Commit the deploy doc**

```bash
git add docs/deploys/2026-05-22-pro-qa.md
git commit -m "docs(deploys): pro-qa cutover notes"
```

- [ ] **Step 3: PAUSE and get Bryan's explicit go-ahead before proceeding to prod**

Per memory `feedback_dev_first_then_prod.md`: prod deploy requires Bryan to explicitly say "ship" / "deploy" / "prod" for THIS specific deploy. Even though he authorized prod for this whole task at the outset, the dev → prod transition is its own checkpoint. Surface the test results from Task 11 + this deploy doc and wait for the magic word.

- [ ] **Step 4: After approval — apply the GRANT on prod RDS (requires `claude-superadmin` MFA per memory)**

```bash
# After exporting temp creds + retrieving DB password from Secrets Manager:
PGPASSWORD="$PROD_DB_PASSWORD" psql -h carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com -U carecompanion -d carecompanion <<'SQL'
GRANT SELECT, INSERT, UPDATE, DELETE ON pro_qa_info, pro_qa_requested_checks, pro_qa_issues, pro_qa_issue_comments, pro_qa_issue_attachments TO carecomp_support_dev;
GRANT USAGE, SELECT ON SEQUENCE pro_qa_issues_issue_number_seq TO carecomp_support_dev;
SQL
```

- [ ] **Step 5: Deploy**

```bash
printf 'DEPLOY\nDEPLOY\nDEPLOY\n' | ./scripts/deploy.sh
```

Watch for `Deployment complete`.

- [ ] **Step 6: Prod smoke test**

Same as Task 11 but against `https://www.mycarecompanion.net/admin/pro-qa/intro`. Confirm:
1. Super-admin can reach all four tabs.
2. Non-super-admin gets 403.
3. A test issue created on prod appears in dev's view of the same workspace.

- [ ] **Step 7: Memory log**

Add a new memory `project_carecompanion_pro_qa.md` summarizing:
- Tables created, role granted, S3 prefix
- Super-admin-only for now; will be re-gated when the role-builder ships
- Initial dataset = empty
- Future work pointer: role-builder feature will replace `RequireSuperAdmin` with `RequireRole("pro_qa")` or similar.

Then update `MEMORY.md` index.

---

## Self-review checklist

**Spec coverage** — each requirement from Bryan's message:

| Requirement | Task |
|---|---|
| New Admin section called "Pro QA" | Task 10 (sidebar) + Task 7 (routes) |
| Same on dev and prod (shared records) | Task 1 (shared DB) + Task 3 (supportDB repo) |
| Accessible only to bryan@bluebonnettech.com | Task 7 (`RequireSuperAdmin`) + Task 10 (sidebar gate) — using super_admin role as the proxy gate per Bryan's clarification |
| Subsection 1: Intro description | Task 8 step 2 (`pro_qa_intro.html`) |
| Subsection 2: Info — rich text both can type in | Task 8 step 3 (`pro_qa_info.html`) + Task 5 (markdown render) |
| Subsection 3: Requested checks | Task 1 (`pro_qa_requested_checks` table) + Task 8 step 4 |
| Subsection 4: Issue Tracker — parent/child IDs, name, description, environment, platform, extended description, comments back and forth, status changes by both | Task 1 (`pro_qa_issues` + `pro_qa_issue_comments`) + Task 8 steps 5/6 + Task 6 (`ChangeStatus` writes auto-comment) |

**Placeholder scan** — none. Every step contains either complete code, an exact command, or a specific browser action.

**Type consistency** — `proQAService` field is consistent; method names match across handler → service → repo (`ListChecks`, `CreateCheck`, etc.); status / severity / env / platform string constants defined in models and referenced from templates via `models.ProQAIssueStatuses` etc.

**Known gotcha addressed:** `ChangeIssueStatus` returns the new status because PG runs subqueries on the post-UPDATE snapshot in the same statement. The service-level `ChangeStatus` pre-fetches the issue first and uses `prev.Status` for the auto-comment, so the value is correct regardless of `ChangeIssueStatus`'s return value.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-22-pro-qa-admin-section.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
