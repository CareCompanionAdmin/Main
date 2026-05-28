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
	GetInfo(ctx context.Context) (*models.ProQAInfo, error)
	UpdateInfo(ctx context.Context, bodyMD, email string) error

	ListChecks(ctx context.Context) ([]models.ProQARequestedCheck, error)
	GetCheck(ctx context.Context, id uuid.UUID) (*models.ProQARequestedCheck, error)
	CreateCheck(ctx context.Context, c *models.ProQARequestedCheck) error
	UpdateCheck(ctx context.Context, c *models.ProQARequestedCheck) error
	ChangeCheckStatus(ctx context.Context, id uuid.UUID, newStatus string) error
	DeleteCheck(ctx context.Context, id uuid.UUID) error

	ListCheckComments(ctx context.Context, checkID uuid.UUID) ([]models.ProQACheckComment, error)
	CreateCheckComment(ctx context.Context, c *models.ProQACheckComment) error

	ListCheckAttachments(ctx context.Context, checkID uuid.UUID) ([]models.ProQACheckAttachment, error)
	CreateCheckAttachment(ctx context.Context, a *models.ProQACheckAttachment) error
	GetCheckAttachment(ctx context.Context, id uuid.UUID) (*models.ProQACheckAttachment, error)

	ListIssues(ctx context.Context, filterStatus string) ([]models.ProQAIssue, error)
	GetIssue(ctx context.Context, id uuid.UUID) (*models.ProQAIssue, error)
	CreateIssue(ctx context.Context, i *models.ProQAIssue) error
	UpdateIssue(ctx context.Context, i *models.ProQAIssue) error
	ChangeIssueStatus(ctx context.Context, id uuid.UUID, newStatus string) error

	ListComments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAIssueComment, error)
	CreateComment(ctx context.Context, c *models.ProQAIssueComment) error

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

const checkSelectCols = `id, title, body_md, status, sort_order, created_at, COALESCE(created_by_email,''), updated_at`

func (r *proQARepo) ListChecks(ctx context.Context) ([]models.ProQARequestedCheck, error) {
	rows, err := r.supportDB.QueryContext(ctx,
		`SELECT `+checkSelectCols+`,
		         (SELECT COUNT(*) FROM pro_qa_check_comments cc WHERE cc.check_id = c.id) AS comment_count,
		         (SELECT COUNT(*) FROM pro_qa_check_attachments ca WHERE ca.check_id = c.id) AS attachment_count
		    FROM pro_qa_requested_checks c
		   ORDER BY sort_order ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProQARequestedCheck
	for rows.Next() {
		var c models.ProQARequestedCheck
		if err := rows.Scan(&c.ID, &c.Title, &c.BodyMD, &c.Status, &c.SortOrder, &c.CreatedAt, &c.CreatedByEmail, &c.UpdatedAt,
			&c.CommentCount, &c.AttachmentCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *proQARepo) GetCheck(ctx context.Context, id uuid.UUID) (*models.ProQARequestedCheck, error) {
	var c models.ProQARequestedCheck
	err := r.supportDB.QueryRowContext(ctx,
		`SELECT `+checkSelectCols+` FROM pro_qa_requested_checks WHERE id=$1`, id,
	).Scan(&c.ID, &c.Title, &c.BodyMD, &c.Status, &c.SortOrder, &c.CreatedAt, &c.CreatedByEmail, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
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

func (r *proQARepo) ChangeCheckStatus(ctx context.Context, id uuid.UUID, newStatus string) error {
	_, err := r.supportDB.ExecContext(ctx,
		`UPDATE pro_qa_requested_checks SET status=$1, updated_at=NOW() WHERE id=$2`,
		newStatus, id)
	return err
}

func (r *proQARepo) DeleteCheck(ctx context.Context, id uuid.UUID) error {
	_, err := r.supportDB.ExecContext(ctx, `DELETE FROM pro_qa_requested_checks WHERE id=$1`, id)
	return err
}

// ---------- Check comments ----------

func (r *proQARepo) ListCheckComments(ctx context.Context, checkID uuid.UUID) ([]models.ProQACheckComment, error) {
	rows, err := r.supportDB.QueryContext(ctx,
		`SELECT id, check_id, body_md, COALESCE(author_email,''), COALESCE(author_name,''),
		        created_at, is_status_change, COALESCE(status_from,''), COALESCE(status_to,'')
		   FROM pro_qa_check_comments
		  WHERE check_id = $1
		  ORDER BY created_at ASC`, checkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProQACheckComment
	for rows.Next() {
		var c models.ProQACheckComment
		if err := rows.Scan(&c.ID, &c.CheckID, &c.BodyMD, &c.AuthorEmail, &c.AuthorName,
			&c.CreatedAt, &c.IsStatusChange, &c.StatusFrom, &c.StatusTo); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *proQARepo) CreateCheckComment(ctx context.Context, c *models.ProQACheckComment) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	_, err := r.supportDB.ExecContext(ctx,
		`INSERT INTO pro_qa_check_comments (id, check_id, body_md, author_email, author_name, created_at, is_status_change, status_from, status_to)
		 VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,NULLIF($8,''),NULLIF($9,''))`,
		c.ID, c.CheckID, c.BodyMD, c.AuthorEmail, c.AuthorName, c.CreatedAt, c.IsStatusChange, c.StatusFrom, c.StatusTo)
	return err
}

// ---------- Check attachments ----------

func (r *proQARepo) ListCheckAttachments(ctx context.Context, checkID uuid.UUID) ([]models.ProQACheckAttachment, error) {
	rows, err := r.supportDB.QueryContext(ctx,
		`SELECT id, check_id, comment_id, filename, content_type, size_bytes, storage_driver, storage_path,
		        COALESCE(uploaded_by_email,''), uploaded_at
		   FROM pro_qa_check_attachments WHERE check_id=$1 ORDER BY uploaded_at ASC`, checkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProQACheckAttachment
	for rows.Next() {
		var a models.ProQACheckAttachment
		var cmt sql.NullString
		if err := rows.Scan(&a.ID, &a.CheckID, &cmt, &a.Filename, &a.ContentType, &a.SizeBytes,
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

func (r *proQARepo) CreateCheckAttachment(ctx context.Context, a *models.ProQACheckAttachment) error {
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
		`INSERT INTO pro_qa_check_attachments (id, check_id, comment_id, filename, content_type, size_bytes, storage_driver, storage_path, uploaded_by_email, uploaded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10)`,
		a.ID, a.CheckID, cmt, a.Filename, a.ContentType, a.SizeBytes, a.StorageDriver, a.StoragePath, a.UploadedByEmail, a.UploadedAt)
	return err
}

func (r *proQARepo) GetCheckAttachment(ctx context.Context, id uuid.UUID) (*models.ProQACheckAttachment, error) {
	var a models.ProQACheckAttachment
	var cmt sql.NullString
	err := r.supportDB.QueryRowContext(ctx,
		`SELECT id, check_id, comment_id, filename, content_type, size_bytes, storage_driver, storage_path,
		        COALESCE(uploaded_by_email,''), uploaded_at
		   FROM pro_qa_check_attachments WHERE id=$1`, id,
	).Scan(&a.ID, &a.CheckID, &cmt, &a.Filename, &a.ContentType, &a.SizeBytes,
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

func (r *proQARepo) ChangeIssueStatus(ctx context.Context, id uuid.UUID, newStatus string) error {
	_, err := r.supportDB.ExecContext(ctx,
		`UPDATE pro_qa_issues
		    SET status = $1,
		        updated_at = NOW(),
		        closed_at = CASE WHEN $1 IN ('resolved','closed','wont_fix') THEN NOW() ELSE NULL END
		  WHERE id = $2`,
		newStatus, id)
	return err
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
