package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"carecompanion/internal/models"
)

// ============================================================================
// ADMIN REPOSITORY - PHI ISOLATION CRITICAL
// ============================================================================
// This repository MUST NEVER access tables containing Protected Health Information.
// The following tables are OFF-LIMITS:
// - children, child_conditions
// - behavior_entries, diet_entries, sleep_entries, bowel_entries
// - speech_entries, sensory_entries, social_entries, therapy_entries
// - seizure_entries, weight_entries, medication_log_entries
// - medications, medication_interactions
// - pattern_analysis, correlation_analysis, health_alerts, alert_correlations
// - chat_threads, chat_messages, chat_participants
// - daily_summary_cache
// ============================================================================

// AdminUserView is a safe view of user data (no PHI)
type AdminUserView struct {
	ID          uuid.UUID         `json:"id"`
	Email       string            `json:"email"`
	FirstName   string            `json:"first_name"`
	LastName    string            `json:"last_name"`
	Phone       models.NullString `json:"phone,omitempty"`
	Status      models.UserStatus `json:"status"`
	SystemRole  models.NullString `json:"system_role,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	LastLoginAt models.NullTime   `json:"last_login_at,omitempty"`
	FamilyCount int               `json:"family_count"` // COUNT only, no details
}

// AdminFamilyView is a safe view of family data (no PHI)
type AdminFamilyView struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	MemberCount int       `json:"member_count"` // COUNT only
	ChildCount  int       `json:"child_count"`  // COUNT only, no names/details
}

// SupportTicket represents a support ticket
type SupportTicket struct {
	ID                   uuid.UUID         `json:"id"`
	UserID               models.NullUUID   `json:"user_id,omitempty"`
	Subject              string            `json:"subject"`
	Description          string            `json:"description"`
	Status               string            `json:"status"`
	Priority             string            `json:"priority"`
	Type                 string            `json:"type"`
	AssignedTo           models.NullUUID   `json:"assigned_to,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	ResolvedAt           models.NullTime   `json:"resolved_at,omitempty"`
	ResolvedBy           models.NullUUID   `json:"resolved_by,omitempty"`
	DuplicateOfTicketID  models.NullUUID   `json:"duplicate_of_ticket_id,omitempty"`
	DuplicateOfRoadmapID models.NullUUID   `json:"duplicate_of_roadmap_id,omitempty"`
	// Populated when needed
	UserEmail      string `json:"user_email,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	DuplicateCount int    `json:"duplicate_count,omitempty"`
}

// TicketMessage represents a message in a support ticket
type TicketMessage struct {
	ID         uuid.UUID `json:"id"`
	TicketID   uuid.UUID `json:"ticket_id"`
	SenderID   uuid.UUID `json:"sender_id"`
	Message    string    `json:"message"`
	IsInternal bool      `json:"is_internal"`
	CreatedAt  time.Time `json:"created_at"`
	// Populated
	SenderName  string `json:"sender_name,omitempty"`
	SenderEmail string `json:"sender_email,omitempty"`
}

// AuditEntry represents an admin audit log entry
type AuditEntry struct {
	ID         uuid.UUID              `json:"id"`
	AdminID    uuid.UUID              `json:"admin_id"`
	Action     string                 `json:"action"`
	TargetType string                 `json:"target_type,omitempty"`
	TargetID   models.NullUUID        `json:"target_id,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	IPAddress  string                 `json:"ip_address,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	// Populated
	AdminEmail string `json:"admin_email,omitempty"`
}

// SystemMetrics represents cached system metrics for marketing
type SystemMetrics struct {
	CachedAt         time.Time `json:"cached_at"`
	TotalUsers       int       `json:"total_users"`
	ActiveUsers24h   int       `json:"active_users_24h"`
	ActiveUsers7d    int       `json:"active_users_7d"`
	TotalFamilies    int       `json:"total_families"`
	TotalEntries     int       `json:"total_entries"`
	EntriesThisWeek  int       `json:"entries_this_week"`
	AvgEntriesPerDay float64   `json:"avg_entries_per_day"`
	NewUsersThisWeek int       `json:"new_users_this_week"`
	NewUsersLastWeek int       `json:"new_users_last_week"`
	UserGrowthPct    float64   `json:"user_growth_percent"`
	// System health metrics (super admin only)
	CPUUtilization       float64 `json:"cpu_utilization"`
	DBStorageUtilization float64 `json:"db_storage_utilization"`
	AvgResponseTimeMs    float64 `json:"avg_response_time_ms"`
	ErrorCount24h        int     `json:"error_count_24h"`
}

// AdminRepository defines the interface for admin data operations
// CRITICAL: No methods in this interface should access PHI tables
type AdminRepository interface {
	// User management (profile data only, NO PHI)
	GetUserByID(ctx context.Context, id uuid.UUID) (*AdminUserView, error)
	SearchUsers(ctx context.Context, query string, page, limit int) ([]AdminUserView, int, error)
	UpdateUserStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error
	ResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error
	ResetUserMFA(ctx context.Context, id uuid.UUID) error

	// Admin user management
	ListAdminUsers(ctx context.Context) ([]AdminUserView, error)
	CreateAdminUser(ctx context.Context, email, passwordHash, firstName, lastName string, role models.SystemRole) (*AdminUserView, error)
	UpdateAdminRole(ctx context.Context, id uuid.UUID, role models.SystemRole) error
	RemoveAdminRole(ctx context.Context, id uuid.UUID) error

	// Family management (metadata only, NO child names/details)
	ListFamilies(ctx context.Context, page, limit int) ([]AdminFamilyView, int, error)
	GetFamilyByID(ctx context.Context, id uuid.UUID) (*AdminFamilyView, error)

	// Support tickets
	CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority, ticketType string) (*SupportTicket, error)
	GetTickets(ctx context.Context, status, ticketType string, page, limit int) ([]SupportTicket, int, error)
	GetTicketByID(ctx context.Context, id uuid.UUID) (*SupportTicket, error)
	GetOpenTicketCount(ctx context.Context) (int, error)
	UpdateTicketStatus(ctx context.Context, id uuid.UUID, status string) error
	AssignTicket(ctx context.Context, ticketID, assigneeID uuid.UUID) error
	ResolveTicket(ctx context.Context, ticketID, resolverID uuid.UUID) error
	DeleteTickets(ctx context.Context, ids []uuid.UUID) (int64, error)
	GetTicketMessages(ctx context.Context, ticketID uuid.UUID) ([]TicketMessage, error)
	AddTicketMessage(ctx context.Context, ticketID, senderID uuid.UUID, message string, isInternal bool) error

	// Duplicate handling
	SetTicketDuplicate(ctx context.Context, ticketID uuid.UUID, dupTicketID, dupRoadmapID *uuid.UUID) error
	GetTicketDuplicates(ctx context.Context, canonicalTicketID uuid.UUID) ([]SupportTicket, error)
	GetTicketsDuplicatedToRoadmap(ctx context.Context, roadmapID uuid.UUID) ([]SupportTicket, error)
	SearchTicketsByText(ctx context.Context, query string, limit int) ([]SupportTicket, error)

	// Metrics (aggregates only, NO individual PHI data)
	GetCachedMetrics(ctx context.Context) (*SystemMetrics, error)
	RefreshMetrics(ctx context.Context) error
	UpdateSystemHealthMetrics(ctx context.Context, cpuUtil, dbStorageUtil float64) error

	// System settings
	GetSetting(ctx context.Context, key string) (interface{}, error)
	GetAllSettings(ctx context.Context) (map[string]interface{}, error)
	UpdateSetting(ctx context.Context, key string, value interface{}, updatedBy uuid.UUID) error

	// Audit log
	LogAction(ctx context.Context, adminID uuid.UUID, action, targetType string, targetID uuid.UUID, details map[string]interface{}, ip, userAgent string) error
	GetAuditLog(ctx context.Context, adminID uuid.UUID, action string, page, limit int) ([]AuditEntry, int, error)

	// Error Log Management
	GetErrorLogs(ctx context.Context, page, limit int, errorType string, acknowledged *bool, sources []models.ErrorSource, includeNoise bool) ([]models.ErrorLogView, int, error)
	GetErrorLogByID(ctx context.Context, id uuid.UUID) (*models.ErrorLogView, error)
	AcknowledgeErrorLog(ctx context.Context, id, acknowledgedBy uuid.UUID, notes string) error
	AcknowledgeErrorLogsBulk(ctx context.Context, ids []uuid.UUID, acknowledgedBy uuid.UUID, notes string) error
	DeleteErrorLog(ctx context.Context, id, deletedBy uuid.UUID) error
	DeleteErrorLogsBulk(ctx context.Context, ids []uuid.UUID, deletedBy uuid.UUID) error
	CreateTicketFromError(ctx context.Context, errorID, adminID uuid.UUID, priority, notes string) (*SupportTicket, error)
	GetUnacknowledgedErrorCount(ctx context.Context) (int, error)
	GetErrorLogSourceCounts(ctx context.Context) (map[models.ErrorSource]int, error)
	CleanupExpiredErrorLogs(ctx context.Context) (int, error)

	// Promo Code Management
	ListPromoCodes(ctx context.Context, page, limit int, activeOnly bool, search string) ([]models.PromoCode, int, error)
	GetPromoCodeByID(ctx context.Context, id uuid.UUID) (*models.PromoCode, error)
	GetPromoCodeByCode(ctx context.Context, code string) (*models.PromoCode, error)
	CreatePromoCode(ctx context.Context, promo *models.PromoCode) (*models.PromoCode, error)
	UpdatePromoCode(ctx context.Context, promo *models.PromoCode) error
	DeactivatePromoCode(ctx context.Context, id, deactivatedBy uuid.UUID, reason string) error
	GetPromoCodeUsages(ctx context.Context, promoCodeID uuid.UUID, page, limit int) ([]models.PromoCodeUsage, int, error)

	// Subscription Plan Management
	ListSubscriptionPlans(ctx context.Context, activeOnly bool) ([]models.SubscriptionPlan, error)
	GetSubscriptionPlanByID(ctx context.Context, id uuid.UUID) (*models.SubscriptionPlan, error)

	// Financial Management
	GetFinancialOverview(ctx context.Context) (*models.FinancialOverview, error)
	GetExpectedRevenueCalendar(ctx context.Context, startDate, endDate time.Time) ([]models.ExpectedRevenueDay, error)
	GetRecentPayments(ctx context.Context, page, limit int) ([]models.Payment, int, error)
	GetRecentSubscriptions(ctx context.Context, page, limit int) ([]models.UserSubscription, int, error)
	GetDailyRevenueSnapshots(ctx context.Context, startDate, endDate time.Time) ([]models.DailyRevenueSnapshot, error)

	// Family-subscription admin tooling (Phase 1 of billing build).
	// Distinct from GetRecentSubscriptions above, which queries the legacy
	// per-user user_subscriptions table.
	ListFamilySubscriptions(ctx context.Context, statusFilter, planFilter, search string, page, limit int) ([]models.FamilySubscription, int, error)
	GetFamilySubscriptionByID(ctx context.Context, id uuid.UUID) (*models.FamilySubscription, error)
	GetFamilySubscriptionByFamilyID(ctx context.Context, familyID uuid.UUID) (*models.FamilySubscription, error)
	UpdateFamilySubscription(ctx context.Context, sub *models.FamilySubscription) error
	CompFamilySubscription(ctx context.Context, familyID, planID, compedBy uuid.UUID, reason string, until time.Time) (*models.FamilySubscription, error)
	CancelFamilySubscription(ctx context.Context, familyID, cancelledBy uuid.UUID, immediate bool) error
}

// adminRepo implements AdminRepository
type adminRepo struct {
	db        *sql.DB // main DB — used for everything except support tables
	supportDB *sql.DB // support_tickets / ticket_messages / ticket_attachments
}

// NewAdminRepo creates a new admin repository.
// supportDB may be the same handle as db (default) or a separate pool when
// dev is configured to share prod's support tickets via SUPPORT_DB_DSN.
func NewAdminRepo(db, supportDB *sql.DB) AdminRepository {
	if supportDB == nil {
		supportDB = db
	}
	return &adminRepo{db: db, supportDB: supportDB}
}

// lookupUserDenorm fetches a user's email + name from the LOCAL users table
// for denormalizing into a support-ticket row. The lookup always hits r.db
// (not r.supportDB), so on dev the denorm reflects the dev-side identity of
// the current actor — that's the right answer for "who created this ticket"
// when the row is then visible from prod via the shared support DB.
func (r *adminRepo) lookupUserDenorm(ctx context.Context, userID uuid.UUID) (email, firstName, lastName string) {
	if userID == uuid.Nil {
		return "", "", ""
	}
	_ = r.db.QueryRowContext(ctx,
		"SELECT COALESCE(email,''), COALESCE(first_name,''), COALESCE(last_name,'') FROM users WHERE id = $1",
		userID,
	).Scan(&email, &firstName, &lastName)
	return
}

// ============================================================================
// USER MANAGEMENT (NO PHI)
// ============================================================================

func (r *adminRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*AdminUserView, error) {
	query := `
		SELECT u.id, u.email, u.first_name, u.last_name, u.phone, u.status, u.system_role,
		       u.created_at, u.last_login_at,
		       (SELECT COUNT(*) FROM family_memberships WHERE user_id = u.id AND is_active = true) as family_count
		FROM users u
		WHERE u.id = $1
	`
	user := &AdminUserView{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.Phone,
		&user.Status, &user.SystemRole, &user.CreatedAt, &user.LastLoginAt, &user.FamilyCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *adminRepo) SearchUsers(ctx context.Context, query string, page, limit int) ([]AdminUserView, int, error) {
	offset := (page - 1) * limit
	searchQuery := "%" + query + "%"

	// Count total
	countSQL := `
		SELECT COUNT(*) FROM users
		WHERE email ILIKE $1 OR first_name ILIKE $1 OR last_name ILIKE $1
	`
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, searchQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get users
	usersSQL := `
		SELECT u.id, u.email, u.first_name, u.last_name, u.phone, u.status, u.system_role,
		       u.created_at, u.last_login_at,
		       (SELECT COUNT(*) FROM family_memberships WHERE user_id = u.id AND is_active = true) as family_count
		FROM users u
		WHERE email ILIKE $1 OR first_name ILIKE $1 OR last_name ILIKE $1
		ORDER BY u.created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, usersSQL, searchQuery, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []AdminUserView
	for rows.Next() {
		var u AdminUserView
		if err := rows.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.Phone,
			&u.Status, &u.SystemRole, &u.CreatedAt, &u.LastLoginAt, &u.FamilyCount); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *adminRepo) UpdateUserStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error {
	// Post-00032: row may live in either admin_users or app_users. Update both;
	// only one will match.
	if _, err := r.db.ExecContext(ctx, `UPDATE app_users SET status = $2, updated_at = NOW() WHERE id = $1`, id, status); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE admin_users SET status = $2, updated_at = NOW() WHERE id = $1`, id, status)
	return err
}

func (r *adminRepo) ResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error {
	// Same fan-out pattern as UpdateUserStatus.
	if _, err := r.db.ExecContext(ctx, `UPDATE app_users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, newHash); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE admin_users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, newHash)
	return err
}

func (r *adminRepo) ResetUserMFA(ctx context.Context, id uuid.UUID) error {
	// For now, we don't have MFA implemented, so this is a placeholder
	// When MFA is added, this would clear the MFA secret/settings
	return nil
}

// ============================================================================
// ADMIN USER MANAGEMENT
// ============================================================================

func (r *adminRepo) ListAdminUsers(ctx context.Context) ([]AdminUserView, error) {
	// Post-00032: admin rows live in admin_users. Phone column doesn't exist
	// on admin_users (admins don't need it), so project NULL.
	// family_count is admin's parent-side family memberships — but admin_users
	// rows have no family memberships by construction, so just project 0.
	query := `
		SELECT u.id, u.email, u.first_name, u.last_name, NULL::varchar AS phone, u.status, u.system_role,
		       u.created_at, u.last_login_at,
		       0 AS family_count
		FROM admin_users u
		ORDER BY u.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []AdminUserView
	for rows.Next() {
		var u AdminUserView
		if err := rows.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.Phone,
			&u.Status, &u.SystemRole, &u.CreatedAt, &u.LastLoginAt, &u.FamilyCount); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *adminRepo) CreateAdminUser(ctx context.Context, email, passwordHash, firstName, lastName string, role models.SystemRole) (*AdminUserView, error) {
	id := uuid.New()
	now := time.Now()
	query := `
		INSERT INTO admin_users (id, email, password_hash, first_name, last_name, system_role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query, id, email, passwordHash, firstName, lastName, role, models.UserStatusActive, now).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.GetUserByID(ctx, id)
}

func (r *adminRepo) UpdateAdminRole(ctx context.Context, id uuid.UUID, role models.SystemRole) error {
	query := `UPDATE admin_users SET system_role = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, role)
	return err
}

// RemoveAdminRole DELETES the admin row entirely (post-00032 — there is no
// "set system_role to NULL" path because admin_users.system_role is NOT NULL,
// and an email that already exists in app_users would otherwise collide if
// we tried to recreate the user as a parent here. If a person needs both
// admin and parent identity, they are TWO ROWS sharing an email.)
func (r *adminRepo) RemoveAdminRole(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM admin_users WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// ============================================================================
// FAMILY MANAGEMENT (METADATA ONLY - NO PHI)
// ============================================================================

func (r *adminRepo) ListFamilies(ctx context.Context, page, limit int) ([]AdminFamilyView, int, error) {
	offset := (page - 1) * limit

	// Count total
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM families").Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get families with counts (NO child names or details)
	query := `
		SELECT f.id, f.name, f.created_at,
		       (SELECT COUNT(*) FROM family_memberships WHERE family_id = f.id AND is_active = true) as member_count,
		       (SELECT COUNT(*) FROM children WHERE family_id = f.id) as child_count
		FROM families f
		ORDER BY f.created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var families []AdminFamilyView
	for rows.Next() {
		var f AdminFamilyView
		if err := rows.Scan(&f.ID, &f.Name, &f.CreatedAt, &f.MemberCount, &f.ChildCount); err != nil {
			return nil, 0, err
		}
		families = append(families, f)
	}
	return families, total, rows.Err()
}

func (r *adminRepo) GetFamilyByID(ctx context.Context, id uuid.UUID) (*AdminFamilyView, error) {
	query := `
		SELECT f.id, f.name, f.created_at,
		       (SELECT COUNT(*) FROM family_memberships WHERE family_id = f.id AND is_active = true) as member_count,
		       (SELECT COUNT(*) FROM children WHERE family_id = f.id) as child_count
		FROM families f
		WHERE f.id = $1
	`
	f := &AdminFamilyView{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(&f.ID, &f.Name, &f.CreatedAt, &f.MemberCount, &f.ChildCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// ============================================================================
// SUPPORT TICKETS
// ============================================================================

func (r *adminRepo) CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority, ticketType string) (*SupportTicket, error) {
	id := uuid.New()
	now := time.Now()
	if priority == "" {
		priority = "normal"
	}
	if ticketType == "" {
		ticketType = "general"
	}
	// Resolve denorm fields from the LOCAL users table so cross-env viewers
	// can render the original creator without joining a foreign users table.
	email, firstName, lastName := r.lookupUserDenorm(ctx, userID)
	query := `
		INSERT INTO support_tickets (id, user_id, subject, description, priority, type, created_at, updated_at, user_email, user_first_name, user_last_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7, $8, $9, $10)
		RETURNING id
	`
	var userIDPtr *uuid.UUID
	if userID != uuid.Nil {
		userIDPtr = &userID
	}
	err := r.supportDB.QueryRowContext(ctx, query, id, userIDPtr, subject, description, priority, ticketType, now, email, firstName, lastName).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.GetTicketByID(ctx, id)
}

func (r *adminRepo) GetTickets(ctx context.Context, status, ticketType string, page, limit int) ([]SupportTicket, int, error) {
	offset := (page - 1) * limit

	// Build WHERE clause and args (shared by count + select).
	var whereParts []string
	var filterArgs []interface{}
	if status != "" {
		filterArgs = append(filterArgs, status)
		whereParts = append(whereParts, fmt.Sprintf("status = $%d", len(filterArgs)))
	}
	if ticketType != "" {
		filterArgs = append(filterArgs, ticketType)
		whereParts = append(whereParts, fmt.Sprintf("type = $%d", len(filterArgs)))
	}
	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = " WHERE " + strings.Join(whereParts, " AND ")
	}

	countSQL := "SELECT COUNT(*) FROM support_tickets" + whereClause
	var total int
	if err := r.supportDB.QueryRowContext(ctx, countSQL, filterArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get tickets — reuse the same WHERE filters built above, then append paging args.
	selectWhere := ""
	if len(whereParts) > 0 {
		// Re-prefix the WHERE conditions with the table alias used in the SELECT.
		var aliased []string
		for _, p := range whereParts {
			aliased = append(aliased, "t."+p)
		}
		selectWhere = " WHERE " + strings.Join(aliased, " AND ")
	}
	args := append([]interface{}{}, filterArgs...)
	args = append(args, limit, offset)
	limitPlaceholder := fmt.Sprintf("$%d", len(args)-1)
	offsetPlaceholder := fmt.Sprintf("$%d", len(args))
	// COALESCE chain prefers the denorm column (post-migration 00027), then
	// the JOIN result for legacy rows, then empty. The JOIN runs against the
	// support DB's users table — when dev shares prod's tickets, that's
	// prod's users and only resolves users who exist on prod. Either way the
	// denorm column carries the correct email regardless of which env the
	// row originated from.
	query := `
		SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority, t.type,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       t.duplicate_of_ticket_id, t.duplicate_of_roadmap_id,
		       COALESCE(NULLIF(t.user_email, ''), u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name,
		       (SELECT COUNT(*) FROM support_tickets d WHERE d.duplicate_of_ticket_id = t.id) AS duplicate_count
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id` + selectWhere +
		" ORDER BY t.created_at DESC LIMIT " + limitPlaceholder + " OFFSET " + offsetPlaceholder

	rows, err := r.supportDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tickets []SupportTicket
	for rows.Next() {
		var t SupportTicket
		if err := rows.Scan(&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority, &t.Type,
			&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
			&t.DuplicateOfTicketID, &t.DuplicateOfRoadmapID,
			&t.UserEmail, &t.AssigneeName, &t.DuplicateCount); err != nil {
			return nil, 0, err
		}
		tickets = append(tickets, t)
	}
	return tickets, total, rows.Err()
}

func (r *adminRepo) GetTicketByID(ctx context.Context, id uuid.UUID) (*SupportTicket, error) {
	query := `
		SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority, t.type,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       t.duplicate_of_ticket_id, t.duplicate_of_roadmap_id,
		       COALESCE(NULLIF(t.user_email, ''), u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name,
		       (SELECT COUNT(*) FROM support_tickets d WHERE d.duplicate_of_ticket_id = t.id) AS duplicate_count
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id
		WHERE t.id = $1
	`
	t := &SupportTicket{}
	err := r.supportDB.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority, &t.Type,
		&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
		&t.DuplicateOfTicketID, &t.DuplicateOfRoadmapID,
		&t.UserEmail, &t.AssigneeName, &t.DuplicateCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *adminRepo) UpdateTicketStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE support_tickets SET status = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.supportDB.ExecContext(ctx, query, id, status)
	return err
}

func (r *adminRepo) AssignTicket(ctx context.Context, ticketID, assigneeID uuid.UUID) error {
	query := `UPDATE support_tickets SET assigned_to = $2, status = 'in_progress', updated_at = NOW() WHERE id = $1`
	_, err := r.supportDB.ExecContext(ctx, query, ticketID, assigneeID)
	return err
}

func (r *adminRepo) ResolveTicket(ctx context.Context, ticketID, resolverID uuid.UUID) error {
	query := `UPDATE support_tickets SET status = 'resolved', resolved_at = NOW(), resolved_by = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.supportDB.ExecContext(ctx, query, ticketID, resolverID)
	return err
}

// DeleteTickets removes the given tickets. ticket_messages and
// ticket_attachments cascade automatically; error_logs / roadmap_items /
// bounty_awards / sibling-duplicate references are SET NULL by FK rules.
// Returns the number of rows actually deleted (caller can compare to len(ids)).
func (r *adminRepo) DeleteTickets(ctx context.Context, ids []uuid.UUID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = id.String()
	}
	res, err := r.supportDB.ExecContext(ctx,
		`DELETE FROM support_tickets WHERE id = ANY($1::uuid[])`,
		pq.Array(idStrs),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *adminRepo) GetTicketMessages(ctx context.Context, ticketID uuid.UUID) ([]TicketMessage, error) {
	query := `
		SELECT m.id, m.ticket_id, m.sender_id, m.message, m.is_internal, m.created_at,
		       COALESCE(NULLIF(TRIM(BOTH ' ' FROM (m.sender_first_name || ' ' || m.sender_last_name)), ''),
		                NULLIF(TRIM(BOTH ' ' FROM (u.first_name || ' ' || u.last_name)), ''),
		                '') as sender_name,
		       COALESCE(NULLIF(m.sender_email, ''), u.email, '') as sender_email
		FROM ticket_messages m
		LEFT JOIN users u ON m.sender_id = u.id
		WHERE m.ticket_id = $1
		ORDER BY m.created_at ASC
	`
	rows, err := r.supportDB.QueryContext(ctx, query, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []TicketMessage
	for rows.Next() {
		var m TicketMessage
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderID, &m.Message, &m.IsInternal,
			&m.CreatedAt, &m.SenderName, &m.SenderEmail); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *adminRepo) AddTicketMessage(ctx context.Context, ticketID, senderID uuid.UUID, message string, isInternal bool) error {
	id := uuid.New()
	email, firstName, lastName := r.lookupUserDenorm(ctx, senderID)
	query := `INSERT INTO ticket_messages (id, ticket_id, sender_id, message, is_internal, created_at, sender_email, sender_first_name, sender_last_name) VALUES ($1, $2, $3, $4, $5, NOW(), $6, $7, $8)`
	_, err := r.supportDB.ExecContext(ctx, query, id, ticketID, senderID, message, isInternal, email, firstName, lastName)
	if err != nil {
		return err
	}
	// Update ticket updated_at
	_, err = r.supportDB.ExecContext(ctx, "UPDATE support_tickets SET updated_at = NOW() WHERE id = $1", ticketID)
	return err
}

// SetTicketDuplicate sets exactly one of duplicate_of_ticket_id or
// duplicate_of_roadmap_id (the other is forced NULL). Pass nil for both to
// clear the duplicate marker.
func (r *adminRepo) SetTicketDuplicate(ctx context.Context, ticketID uuid.UUID, dupTicketID, dupRoadmapID *uuid.UUID) error {
	if dupTicketID != nil && dupRoadmapID != nil {
		return fmt.Errorf("ticket can be a duplicate of either a ticket or a roadmap item, not both")
	}
	_, err := r.supportDB.ExecContext(ctx, `
        UPDATE support_tickets
        SET duplicate_of_ticket_id  = $2,
            duplicate_of_roadmap_id = $3,
            updated_at              = NOW()
        WHERE id = $1
    `, ticketID, dupTicketID, dupRoadmapID)
	return err
}

// GetTicketDuplicates returns tickets that point at canonicalTicketID via
// duplicate_of_ticket_id.
func (r *adminRepo) GetTicketDuplicates(ctx context.Context, canonicalTicketID uuid.UUID) ([]SupportTicket, error) {
	return r.queryTicketsBy(ctx, "t.duplicate_of_ticket_id = $1", canonicalTicketID)
}

// GetTicketsDuplicatedToRoadmap returns tickets that were marked dup directly
// onto a roadmap item.
func (r *adminRepo) GetTicketsDuplicatedToRoadmap(ctx context.Context, roadmapID uuid.UUID) ([]SupportTicket, error) {
	return r.queryTicketsBy(ctx, "t.duplicate_of_roadmap_id = $1", roadmapID)
}

// SearchTicketsByText performs a case-insensitive substring match on subject.
// Used by the dup picker autocomplete; capped at the caller-provided limit.
func (r *adminRepo) SearchTicketsByText(ctx context.Context, query string, limit int) ([]SupportTicket, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := r.supportDB.QueryContext(ctx, `
        SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority, t.type,
               t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
               t.duplicate_of_ticket_id, t.duplicate_of_roadmap_id,
               COALESCE(NULLIF(t.user_email, ''), u.email, '') as user_email,
               COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name,
               (SELECT COUNT(*) FROM support_tickets d WHERE d.duplicate_of_ticket_id = t.id) AS duplicate_count
        FROM support_tickets t
        LEFT JOIN users u ON t.user_id = u.id
        LEFT JOIN users a ON t.assigned_to = a.id
        WHERE LOWER(t.subject) LIKE $1
        ORDER BY t.created_at DESC
        LIMIT $2
    `, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTickets(rows)
}

// queryTicketsBy is a small helper to avoid repeating the column list +
// joins for one-arg WHERE-clause lookups.
func (r *adminRepo) queryTicketsBy(ctx context.Context, whereClause string, arg interface{}) ([]SupportTicket, error) {
	q := `
        SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority, t.type,
               t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
               t.duplicate_of_ticket_id, t.duplicate_of_roadmap_id,
               COALESCE(NULLIF(t.user_email, ''), u.email, '') as user_email,
               COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name,
               (SELECT COUNT(*) FROM support_tickets d WHERE d.duplicate_of_ticket_id = t.id) AS duplicate_count
        FROM support_tickets t
        LEFT JOIN users u ON t.user_id = u.id
        LEFT JOIN users a ON t.assigned_to = a.id
        WHERE ` + whereClause + `
        ORDER BY t.created_at DESC
    `
	rows, err := r.supportDB.QueryContext(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTickets(rows)
}

func scanTickets(rows *sql.Rows) ([]SupportTicket, error) {
	var out []SupportTicket
	for rows.Next() {
		var t SupportTicket
		if err := rows.Scan(&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority, &t.Type,
			&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
			&t.DuplicateOfTicketID, &t.DuplicateOfRoadmapID,
			&t.UserEmail, &t.AssigneeName, &t.DuplicateCount); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ============================================================================
// METRICS (AGGREGATES ONLY - NO PHI)
// ============================================================================

func (r *adminRepo) GetCachedMetrics(ctx context.Context) (*SystemMetrics, error) {
	metrics := &SystemMetrics{}

	// Get user counts
	var userCountsJSON []byte
	err := r.db.QueryRowContext(ctx, "SELECT metric_value, calculated_at FROM system_metrics_cache WHERE metric_name = 'user_counts'").Scan(&userCountsJSON, &metrics.CachedAt)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if userCountsJSON != nil {
		var uc map[string]interface{}
		json.Unmarshal(userCountsJSON, &uc)
		if v, ok := uc["total"].(float64); ok {
			metrics.TotalUsers = int(v)
		}
		if v, ok := uc["active_24h"].(float64); ok {
			metrics.ActiveUsers24h = int(v)
		}
		if v, ok := uc["active_7d"].(float64); ok {
			metrics.ActiveUsers7d = int(v)
		}
		if v, ok := uc["new_this_week"].(float64); ok {
			metrics.NewUsersThisWeek = int(v)
		}
	}

	// Get family counts
	var familyCountsJSON []byte
	r.db.QueryRowContext(ctx, "SELECT metric_value FROM system_metrics_cache WHERE metric_name = 'family_counts'").Scan(&familyCountsJSON)
	if familyCountsJSON != nil {
		var fc map[string]interface{}
		json.Unmarshal(familyCountsJSON, &fc)
		if v, ok := fc["total"].(float64); ok {
			metrics.TotalFamilies = int(v)
		}
	}

	// Get entry counts
	var entryCountsJSON []byte
	r.db.QueryRowContext(ctx, "SELECT metric_value FROM system_metrics_cache WHERE metric_name = 'entry_counts'").Scan(&entryCountsJSON)
	if entryCountsJSON != nil {
		var ec map[string]interface{}
		json.Unmarshal(entryCountsJSON, &ec)
		if v, ok := ec["total"].(float64); ok {
			metrics.TotalEntries = int(v)
		}
		if v, ok := ec["this_week"].(float64); ok {
			metrics.EntriesThisWeek = int(v)
		}
		if v, ok := ec["avg_per_day"].(float64); ok {
			metrics.AvgEntriesPerDay = v
		}
	}

	// Get growth metrics
	var growthJSON []byte
	r.db.QueryRowContext(ctx, "SELECT metric_value FROM system_metrics_cache WHERE metric_name = 'growth_metrics'").Scan(&growthJSON)
	if growthJSON != nil {
		var gm map[string]interface{}
		json.Unmarshal(growthJSON, &gm)
		if v, ok := gm["user_growth_percent"].(float64); ok {
			metrics.UserGrowthPct = v
		}
		if v, ok := gm["new_users_last_week"].(float64); ok {
			metrics.NewUsersLastWeek = int(v)
		}
	}

	// Get system health metrics from system_health cache
	var healthJSON []byte
	r.db.QueryRowContext(ctx, "SELECT metric_value FROM system_metrics_cache WHERE metric_name = 'system_health'").Scan(&healthJSON)
	if healthJSON != nil {
		var sh map[string]interface{}
		json.Unmarshal(healthJSON, &sh)
		if v, ok := sh["cpu_utilization"].(float64); ok {
			metrics.CPUUtilization = v
		}
		if v, ok := sh["db_storage_utilization"].(float64); ok {
			metrics.DBStorageUtilization = v
		}
	}

	// Get avg response time from response_time_logs (last 24 hours)
	r.db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(response_time_ms), 0) FROM response_time_logs WHERE created_at > NOW() - INTERVAL '24 hours'",
	).Scan(&metrics.AvgResponseTimeMs)

	// Get error count from error_logs (last 24 hours)
	r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM error_logs WHERE created_at > NOW() - INTERVAL '24 hours'",
	).Scan(&metrics.ErrorCount24h)

	return metrics, nil
}

func (r *adminRepo) RefreshMetrics(ctx context.Context) error {
	now := time.Now()

	// Refresh user counts
	var totalUsers, active24h, active7d, newThisWeek int
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&totalUsers)
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE last_login_at > NOW() - INTERVAL '24 hours'").Scan(&active24h)
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE last_login_at > NOW() - INTERVAL '7 days'").Scan(&active7d)
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE created_at > NOW() - INTERVAL '7 days'").Scan(&newThisWeek)

	userCounts, _ := json.Marshal(map[string]int{
		"total": totalUsers, "active_24h": active24h, "active_7d": active7d, "new_this_week": newThisWeek,
	})
	r.db.ExecContext(ctx, "UPDATE system_metrics_cache SET metric_value = $1, calculated_at = $2 WHERE metric_name = 'user_counts'", userCounts, now)

	// Refresh family counts
	var totalFamilies int
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM families").Scan(&totalFamilies)
	familyCounts, _ := json.Marshal(map[string]int{"total": totalFamilies})
	r.db.ExecContext(ctx, "UPDATE system_metrics_cache SET metric_value = $1, calculated_at = $2 WHERE metric_name = 'family_counts'", familyCounts, now)

	// Refresh entry counts (aggregate across all log tables - NO individual data)
	var totalEntries, entriesThisWeek int
	entryTables := []string{
		"behavior_logs", "diet_logs", "sleep_logs", "bowel_logs", "medication_logs",
	}
	for _, table := range entryTables {
		var count int
		r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
		totalEntries += count
		r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE created_at > NOW() - INTERVAL '7 days'").Scan(&count)
		entriesThisWeek += count
	}
	avgPerDay := float64(entriesThisWeek) / 7.0
	entryCounts, _ := json.Marshal(map[string]interface{}{
		"total": totalEntries, "this_week": entriesThisWeek, "avg_per_day": avgPerDay,
	})
	r.db.ExecContext(ctx, "UPDATE system_metrics_cache SET metric_value = $1, calculated_at = $2 WHERE metric_name = 'entry_counts'", entryCounts, now)

	// Refresh growth metrics
	var newUsersLastWeek int
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE created_at > NOW() - INTERVAL '14 days' AND created_at <= NOW() - INTERVAL '7 days'").Scan(&newUsersLastWeek)
	var growthPct float64
	if newUsersLastWeek > 0 {
		growthPct = float64(newThisWeek-newUsersLastWeek) / float64(newUsersLastWeek) * 100
	}
	growthMetrics, _ := json.Marshal(map[string]interface{}{
		"user_growth_percent": growthPct, "new_users_this_week": newThisWeek, "new_users_last_week": newUsersLastWeek,
	})
	r.db.ExecContext(ctx, "UPDATE system_metrics_cache SET metric_value = $1, calculated_at = $2 WHERE metric_name = 'growth_metrics'", growthMetrics, now)

	return nil
}

// UpdateSystemHealthMetrics updates system health metrics from CloudWatch
func (r *adminRepo) UpdateSystemHealthMetrics(ctx context.Context, cpuUtil, dbStorageUtil float64) error {
	now := time.Now()
	healthMetrics, _ := json.Marshal(map[string]interface{}{
		"cpu_utilization":        cpuUtil,
		"db_storage_utilization": dbStorageUtil,
		"uptime_percent":         100.0,
	})
	_, err := r.db.ExecContext(ctx,
		"UPDATE system_metrics_cache SET metric_value = $1, calculated_at = $2 WHERE metric_name = 'system_health'",
		healthMetrics, now)
	return err
}

// ============================================================================
// SYSTEM SETTINGS
// ============================================================================

func (r *adminRepo) GetSetting(ctx context.Context, key string) (interface{}, error) {
	var valueJSON []byte
	err := r.db.QueryRowContext(ctx, "SELECT value FROM system_settings WHERE key = $1", key).Scan(&valueJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value interface{}
	if err := json.Unmarshal(valueJSON, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (r *adminRepo) GetAllSettings(ctx context.Context) (map[string]interface{}, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT key, value FROM system_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]interface{})
	for rows.Next() {
		var key string
		var valueJSON []byte
		if err := rows.Scan(&key, &valueJSON); err != nil {
			return nil, err
		}
		var value interface{}
		json.Unmarshal(valueJSON, &value)
		settings[key] = value
	}
	return settings, rows.Err()
}

func (r *adminRepo) UpdateSetting(ctx context.Context, key string, value interface{}, updatedBy uuid.UUID) error {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}
	query := `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_by = $3, updated_at = NOW()
	`
	_, err = r.db.ExecContext(ctx, query, key, valueJSON, updatedBy)
	return err
}

// ============================================================================
// AUDIT LOG
// ============================================================================

func (r *adminRepo) LogAction(ctx context.Context, adminID uuid.UUID, action, targetType string, targetID uuid.UUID, details map[string]interface{}, ip, userAgent string) error {
	id := uuid.New()
	detailsJSON, _ := json.Marshal(details)
	var targetIDPtr *uuid.UUID
	if targetID != uuid.Nil {
		targetIDPtr = &targetID
	}
	query := `
		INSERT INTO admin_audit_log (id, admin_id, action, target_type, target_id, details, ip_address, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	_, err := r.db.ExecContext(ctx, query, id, adminID, action, targetType, targetIDPtr, detailsJSON, ip, userAgent)
	return err
}

func (r *adminRepo) GetAuditLog(ctx context.Context, adminID uuid.UUID, action string, page, limit int) ([]AuditEntry, int, error) {
	offset := (page - 1) * limit

	// Build where clause
	where := "WHERE 1=1"
	args := []interface{}{}
	argNum := 1
	if adminID != uuid.Nil {
		where += " AND a.admin_id = $" + string(rune('0'+argNum))
		args = append(args, adminID)
		argNum++
	}
	if action != "" {
		where += " AND a.action = $" + string(rune('0'+argNum))
		args = append(args, action)
		argNum++
	}

	// Count
	countSQL := "SELECT COUNT(*) FROM admin_audit_log a " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get entries
	query := `
		SELECT a.id, a.admin_id, a.action, a.target_type, a.target_id, a.details,
		       COALESCE(a.ip_address::text, ''), COALESCE(a.user_agent, ''), a.created_at,
		       COALESCE(u.email, '') as admin_email
		FROM admin_audit_log a
		LEFT JOIN users u ON a.admin_id = u.id
		` + where + `
		ORDER BY a.created_at DESC
		LIMIT $` + string(rune('0'+argNum)) + ` OFFSET $` + string(rune('0'+argNum+1))
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.AdminID, &e.Action, &e.TargetType, &e.TargetID,
			&detailsJSON, &e.IPAddress, &e.UserAgent, &e.CreatedAt, &e.AdminEmail); err != nil {
			return nil, 0, err
		}
		if detailsJSON != nil {
			json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// GetOpenTicketCount returns the count of open tickets needing attention
func (r *adminRepo) GetOpenTicketCount(ctx context.Context) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM support_tickets WHERE status = 'open'`
	err := r.supportDB.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// ============================================================================
// ERROR LOG MANAGEMENT
// ============================================================================

// GetErrorLogs returns filtered error logs with pagination
// By default (when sources is empty), only returns 'user' and 'infrastructure' errors
func (r *adminRepo) GetErrorLogs(ctx context.Context, page, limit int, errorType string, acknowledged *bool, sources []models.ErrorSource, includeNoise bool) ([]models.ErrorLogView, int, error) {
	offset := (page - 1) * limit

	// Build WHERE clause
	where := "WHERE e.is_deleted = FALSE"
	args := []interface{}{}
	argNum := 1

	// Source filtering - default to user + infrastructure if not specified
	if len(sources) == 0 && !includeNoise {
		// Default view: only user and infrastructure errors
		where += " AND e.error_source IN ('user', 'infrastructure')"
	} else if len(sources) > 0 {
		// Custom source filter
		placeholders := ""
		for i, src := range sources {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "$" + itoa(argNum)
			args = append(args, string(src))
			argNum++
		}
		where += " AND e.error_source IN (" + placeholders + ")"
	}
	// If includeNoise is true and sources is empty, show all sources

	if errorType != "" {
		where += " AND e.error_type = $" + itoa(argNum)
		args = append(args, errorType)
		argNum++
	}

	if acknowledged != nil {
		if *acknowledged {
			where += " AND e.acknowledged_at IS NOT NULL"
		} else {
			where += " AND e.acknowledged_at IS NULL"
		}
	}

	// Count total
	countSQL := "SELECT COUNT(*) FROM error_logs e " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get logs with new columns
	query := `
		SELECT e.id, e.error_type, COALESCE(e.status_code, 0), COALESCE(e.method, ''),
		       COALESCE(e.path, ''), COALESCE(e.error_message, ''), e.stack_trace, e.user_id, e.request_id,
		       e.user_agent, e.ip_address, e.created_at,
		       COALESCE(e.error_source, 'unknown'), COALESCE(e.is_noise, false), e.auto_delete_at,
		       e.acknowledged_at, e.acknowledged_by, e.acknowledged_notes,
		       COALESCE(e.is_deleted, false), e.deleted_at, e.deleted_by,
		       COALESCE(u.email, '') as acknowledged_by_email,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as acknowledged_by_name,
		       COALESCE(eu.email, '') as user_email
		FROM error_logs e
		LEFT JOIN users u ON e.acknowledged_by = u.id
		LEFT JOIN users eu ON e.user_id = eu.id
		` + where + `
		ORDER BY e.created_at DESC
		LIMIT $` + itoa(argNum) + ` OFFSET $` + itoa(argNum+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []models.ErrorLogView
	for rows.Next() {
		var log models.ErrorLogView
		if err := rows.Scan(
			&log.ID, &log.ErrorType, &log.StatusCode, &log.Method, &log.Path,
			&log.Message, &log.StackTrace, &log.UserID, &log.RequestID,
			&log.UserAgent, &log.IPAddress, &log.CreatedAt,
			&log.ErrorSource, &log.IsNoise, &log.AutoDeleteAt,
			&log.AcknowledgedAt, &log.AcknowledgedBy, &log.AcknowledgedNotes,
			&log.IsDeleted, &log.DeletedAt, &log.DeletedBy,
			&log.AcknowledgedByEmail, &log.AcknowledgedByName, &log.UserEmail,
		); err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}
	return logs, total, rows.Err()
}

// GetErrorLogSourceCounts returns counts of errors by source for the filter UI
func (r *adminRepo) GetErrorLogSourceCounts(ctx context.Context) (map[models.ErrorSource]int, error) {
	query := `
		SELECT COALESCE(error_source, 'unknown'), COUNT(*)
		FROM error_logs
		WHERE is_deleted = FALSE
		GROUP BY error_source
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[models.ErrorSource]int)
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, err
		}
		counts[models.ErrorSource(source)] = count
	}
	return counts, rows.Err()
}

// CleanupExpiredErrorLogs soft-deletes error logs past their auto_delete_at date
func (r *adminRepo) CleanupExpiredErrorLogs(ctx context.Context) (int, error) {
	query := `
		UPDATE error_logs
		SET is_deleted = TRUE, deleted_at = NOW()
		WHERE auto_delete_at < NOW()
		  AND is_deleted = FALSE
		  AND acknowledged_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

func (r *adminRepo) GetErrorLogByID(ctx context.Context, id uuid.UUID) (*models.ErrorLogView, error) {
	query := `
		SELECT e.id, e.error_type, COALESCE(e.status_code, 0), COALESCE(e.method, ''),
		       COALESCE(e.path, ''), COALESCE(e.error_message, ''), e.stack_trace, e.user_id, e.request_id,
		       e.user_agent, e.ip_address, e.created_at,
		       COALESCE(e.error_source, 'unknown'), COALESCE(e.is_noise, false), e.auto_delete_at,
		       e.acknowledged_at, e.acknowledged_by, e.acknowledged_notes,
		       COALESCE(e.is_deleted, false), e.deleted_at, e.deleted_by,
		       COALESCE(u.email, '') as acknowledged_by_email,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as acknowledged_by_name,
		       COALESCE(eu.email, '') as user_email
		FROM error_logs e
		LEFT JOIN users u ON e.acknowledged_by = u.id
		LEFT JOIN users eu ON e.user_id = eu.id
		WHERE e.id = $1
	`
	log := &models.ErrorLogView{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ErrorType, &log.StatusCode, &log.Method, &log.Path,
		&log.Message, &log.StackTrace, &log.UserID, &log.RequestID,
		&log.UserAgent, &log.IPAddress, &log.CreatedAt,
		&log.ErrorSource, &log.IsNoise, &log.AutoDeleteAt,
		&log.AcknowledgedAt, &log.AcknowledgedBy, &log.AcknowledgedNotes,
		&log.IsDeleted, &log.DeletedAt, &log.DeletedBy,
		&log.AcknowledgedByEmail, &log.AcknowledgedByName, &log.UserEmail,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return log, nil
}

func (r *adminRepo) AcknowledgeErrorLog(ctx context.Context, id, acknowledgedBy uuid.UUID, notes string) error {
	query := `
		UPDATE error_logs
		SET acknowledged_at = NOW(), acknowledged_by = $2, acknowledged_notes = $3
		WHERE id = $1 AND acknowledged_at IS NULL
	`
	_, err := r.db.ExecContext(ctx, query, id, acknowledgedBy, notes)
	return err
}

func (r *adminRepo) AcknowledgeErrorLogsBulk(ctx context.Context, ids []uuid.UUID, acknowledgedBy uuid.UUID, notes string) error {
	if len(ids) == 0 {
		return nil
	}

	// Build placeholders
	placeholders := ""
	args := []interface{}{}
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "$" + itoa(i+1)
		args = append(args, id)
	}

	query := `
		UPDATE error_logs
		SET acknowledged_at = NOW(), acknowledged_by = $` + itoa(len(ids)+1) + `, acknowledged_notes = $` + itoa(len(ids)+2) + `
		WHERE id IN (` + placeholders + `) AND acknowledged_at IS NULL
	`
	args = append(args, acknowledgedBy, notes)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *adminRepo) DeleteErrorLog(ctx context.Context, id, deletedBy uuid.UUID) error {
	query := `
		UPDATE error_logs
		SET is_deleted = TRUE, deleted_at = NOW(), deleted_by = $2
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, id, deletedBy)
	return err
}

func (r *adminRepo) DeleteErrorLogsBulk(ctx context.Context, ids []uuid.UUID, deletedBy uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	// Build placeholders
	placeholders := ""
	args := []interface{}{}
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "$" + itoa(i+1)
		args = append(args, id)
	}

	query := `
		UPDATE error_logs
		SET is_deleted = TRUE, deleted_at = NOW(), deleted_by = $` + itoa(len(ids)+1) + `
		WHERE id IN (` + placeholders + `)
	`
	args = append(args, deletedBy)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *adminRepo) CreateTicketFromError(ctx context.Context, errorID, adminID uuid.UUID, priority, notes string) (*SupportTicket, error) {
	// Get the error log
	errorLog, err := r.GetErrorLogByID(ctx, errorID)
	if err != nil {
		return nil, err
	}
	if errorLog == nil {
		return nil, sql.ErrNoRows
	}

	// Create subject and description from error
	subject := "Error: " + errorLog.ErrorType + " - " + errorLog.Path
	if len(subject) > 200 {
		subject = subject[:197] + "..."
	}

	description := "Auto-generated from error log:\n\n"
	description += "Error Type: " + errorLog.ErrorType + "\n"
	description += "Status Code: " + itoa(errorLog.StatusCode) + "\n"
	description += "Method: " + errorLog.Method + "\n"
	description += "Path: " + errorLog.Path + "\n"
	description += "Message: " + errorLog.Message + "\n"
	description += "Time: " + errorLog.CreatedAt.Format(time.RFC3339) + "\n"
	if notes != "" {
		description += "\nAdmin Notes: " + notes
	}

	if priority == "" {
		priority = "medium"
	}

	// Create the ticket (assigned to the admin who created it)
	ticket, err := r.CreateTicket(ctx, uuid.Nil, subject, description, priority, "bug_report")
	if err != nil {
		return nil, err
	}

	// Assign to the admin
	if err := r.AssignTicket(ctx, ticket.ID, adminID); err != nil {
		return nil, err
	}

	// Mark error as acknowledged
	_ = r.AcknowledgeErrorLog(ctx, errorID, adminID, "Ticket created: "+ticket.ID.String())

	return r.GetTicketByID(ctx, ticket.ID)
}

func (r *adminRepo) GetUnacknowledgedErrorCount(ctx context.Context) (int, error) {
	var count int
	// Only count user and infrastructure errors (not scanner noise, anonymous, or unknown)
	query := `SELECT COUNT(*) FROM error_logs
		WHERE acknowledged_at IS NULL
		AND is_deleted = FALSE
		AND error_source IN ('user', 'infrastructure')`
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// ============================================================================
// PROMO CODE MANAGEMENT
// ============================================================================

func (r *adminRepo) ListPromoCodes(ctx context.Context, page, limit int, activeOnly bool, search string) ([]models.PromoCode, int, error) {
	offset := (page - 1) * limit

	where := "WHERE 1=1"
	args := []interface{}{}
	argNum := 1

	if activeOnly {
		where += " AND is_active = TRUE AND (expires_at IS NULL OR expires_at > NOW())"
	}

	if search != "" {
		where += " AND (UPPER(code) LIKE UPPER($" + itoa(argNum) + ") OR UPPER(name) LIKE UPPER($" + itoa(argNum) + "))"
		args = append(args, "%"+search+"%")
		argNum++
	}

	// Count
	countSQL := "SELECT COUNT(*) FROM promo_codes " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get promo codes
	query := `
		SELECT p.id, p.code, p.name, p.description,
		       p.discount_type, p.discount_value, p.max_discount_cents, p.applies_to,
		       p.applies_to_plans, p.applies_to_billing_intervals, p.minimum_purchase_cents,
		       p.new_users_only, p.existing_users_only, p.specific_user_ids, p.specific_email_domains,
		       p.max_total_uses, p.max_uses_per_user, p.current_total_uses,
		       p.starts_at, p.expires_at, p.duration_months,
		       p.is_stackable, p.stackable_with_codes,
		       p.campaign_name, p.campaign_source, p.affiliate_id,
		       p.total_discount_given_cents, p.total_revenue_attributed_cents,
		       p.is_active, p.deactivated_at, p.deactivated_by, p.deactivation_reason,
		       p.created_by, p.created_at, p.updated_at,
		       COALESCE(u.email, '') as created_by_email
		FROM promo_codes p
		LEFT JOIN users u ON p.created_by = u.id
		` + where + `
		ORDER BY p.created_at DESC
		LIMIT $` + itoa(argNum) + ` OFFSET $` + itoa(argNum+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var promos []models.PromoCode
	for rows.Next() {
		var p models.PromoCode
		if err := rows.Scan(
			&p.ID, &p.Code, &p.Name, &p.Description,
			&p.DiscountType, &p.DiscountValue, &p.MaxDiscountCents, &p.AppliesTo,
			&p.AppliesToPlans, &p.AppliesToBillingIntervals, &p.MinimumPurchaseCents,
			&p.NewUsersOnly, &p.ExistingUsersOnly, &p.SpecificUserIDs, &p.SpecificEmailDomains,
			&p.MaxTotalUses, &p.MaxUsesPerUser, &p.CurrentTotalUses,
			&p.StartsAt, &p.ExpiresAt, &p.DurationMonths,
			&p.IsStackable, &p.StackableWithCodes,
			&p.CampaignName, &p.CampaignSource, &p.AffiliateID,
			&p.TotalDiscountGivenCents, &p.TotalRevenueAttributedCents,
			&p.IsActive, &p.DeactivatedAt, &p.DeactivatedBy, &p.DeactivationReason,
			&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
			&p.CreatedByEmail,
		); err != nil {
			return nil, 0, err
		}
		promos = append(promos, p)
	}
	return promos, total, rows.Err()
}

func (r *adminRepo) GetPromoCodeByID(ctx context.Context, id uuid.UUID) (*models.PromoCode, error) {
	query := `
		SELECT p.id, p.code, p.name, p.description,
		       p.discount_type, p.discount_value, p.max_discount_cents, p.applies_to,
		       p.applies_to_plans, p.applies_to_billing_intervals, p.minimum_purchase_cents,
		       p.new_users_only, p.existing_users_only, p.specific_user_ids, p.specific_email_domains,
		       p.max_total_uses, p.max_uses_per_user, p.current_total_uses,
		       p.starts_at, p.expires_at, p.duration_months,
		       p.is_stackable, p.stackable_with_codes,
		       p.campaign_name, p.campaign_source, p.affiliate_id,
		       p.total_discount_given_cents, p.total_revenue_attributed_cents,
		       p.is_active, p.deactivated_at, p.deactivated_by, p.deactivation_reason,
		       p.created_by, p.created_at, p.updated_at,
		       COALESCE(u.email, '') as created_by_email
		FROM promo_codes p
		LEFT JOIN users u ON p.created_by = u.id
		WHERE p.id = $1
	`
	p := &models.PromoCode{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.MaxDiscountCents, &p.AppliesTo,
		&p.AppliesToPlans, &p.AppliesToBillingIntervals, &p.MinimumPurchaseCents,
		&p.NewUsersOnly, &p.ExistingUsersOnly, &p.SpecificUserIDs, &p.SpecificEmailDomains,
		&p.MaxTotalUses, &p.MaxUsesPerUser, &p.CurrentTotalUses,
		&p.StartsAt, &p.ExpiresAt, &p.DurationMonths,
		&p.IsStackable, &p.StackableWithCodes,
		&p.CampaignName, &p.CampaignSource, &p.AffiliateID,
		&p.TotalDiscountGivenCents, &p.TotalRevenueAttributedCents,
		&p.IsActive, &p.DeactivatedAt, &p.DeactivatedBy, &p.DeactivationReason,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		&p.CreatedByEmail,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *adminRepo) GetPromoCodeByCode(ctx context.Context, code string) (*models.PromoCode, error) {
	query := `
		SELECT p.id, p.code, p.name, p.description,
		       p.discount_type, p.discount_value, p.max_discount_cents, p.applies_to,
		       p.applies_to_plans, p.applies_to_billing_intervals, p.minimum_purchase_cents,
		       p.new_users_only, p.existing_users_only, p.specific_user_ids, p.specific_email_domains,
		       p.max_total_uses, p.max_uses_per_user, p.current_total_uses,
		       p.starts_at, p.expires_at, p.duration_months,
		       p.is_stackable, p.stackable_with_codes,
		       p.campaign_name, p.campaign_source, p.affiliate_id,
		       p.total_discount_given_cents, p.total_revenue_attributed_cents,
		       p.is_active, p.deactivated_at, p.deactivated_by, p.deactivation_reason,
		       p.created_by, p.created_at, p.updated_at,
		       COALESCE(u.email, '') as created_by_email
		FROM promo_codes p
		LEFT JOIN users u ON p.created_by = u.id
		WHERE UPPER(p.code) = UPPER($1)
	`
	p := &models.PromoCode{}
	err := r.db.QueryRowContext(ctx, query, code).Scan(
		&p.ID, &p.Code, &p.Name, &p.Description,
		&p.DiscountType, &p.DiscountValue, &p.MaxDiscountCents, &p.AppliesTo,
		&p.AppliesToPlans, &p.AppliesToBillingIntervals, &p.MinimumPurchaseCents,
		&p.NewUsersOnly, &p.ExistingUsersOnly, &p.SpecificUserIDs, &p.SpecificEmailDomains,
		&p.MaxTotalUses, &p.MaxUsesPerUser, &p.CurrentTotalUses,
		&p.StartsAt, &p.ExpiresAt, &p.DurationMonths,
		&p.IsStackable, &p.StackableWithCodes,
		&p.CampaignName, &p.CampaignSource, &p.AffiliateID,
		&p.TotalDiscountGivenCents, &p.TotalRevenueAttributedCents,
		&p.IsActive, &p.DeactivatedAt, &p.DeactivatedBy, &p.DeactivationReason,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		&p.CreatedByEmail,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *adminRepo) CreatePromoCode(ctx context.Context, promo *models.PromoCode) (*models.PromoCode, error) {
	promo.ID = uuid.New()
	promo.CreatedAt = time.Now()
	promo.UpdatedAt = promo.CreatedAt
	promo.CurrentTotalUses = 0
	promo.TotalDiscountGivenCents = 0
	promo.TotalRevenueAttributedCents = 0
	promo.IsActive = true

	query := `
		INSERT INTO promo_codes (
			id, code, name, description,
			discount_type, discount_value, max_discount_cents, applies_to,
			applies_to_plans, applies_to_billing_intervals, minimum_purchase_cents,
			new_users_only, existing_users_only, specific_user_ids, specific_email_domains,
			max_total_uses, max_uses_per_user, current_total_uses,
			starts_at, expires_at, duration_months,
			is_stackable, stackable_with_codes,
			campaign_name, campaign_source, affiliate_id,
			total_discount_given_cents, total_revenue_attributed_cents,
			is_active, created_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11,
			$12, $13, $14, $15,
			$16, $17, $18,
			$19, $20, $21,
			$22, $23,
			$24, $25, $26,
			$27, $28,
			$29, $30, $31, $32
		) RETURNING id
	`

	err := r.db.QueryRowContext(ctx, query,
		promo.ID, promo.Code, promo.Name, promo.Description,
		promo.DiscountType, promo.DiscountValue, promo.MaxDiscountCents, promo.AppliesTo,
		promo.AppliesToPlans, promo.AppliesToBillingIntervals, promo.MinimumPurchaseCents,
		promo.NewUsersOnly, promo.ExistingUsersOnly, promo.SpecificUserIDs, promo.SpecificEmailDomains,
		promo.MaxTotalUses, promo.MaxUsesPerUser, promo.CurrentTotalUses,
		promo.StartsAt, promo.ExpiresAt, promo.DurationMonths,
		promo.IsStackable, promo.StackableWithCodes,
		promo.CampaignName, promo.CampaignSource, promo.AffiliateID,
		promo.TotalDiscountGivenCents, promo.TotalRevenueAttributedCents,
		promo.IsActive, promo.CreatedBy, promo.CreatedAt, promo.UpdatedAt,
	).Scan(&promo.ID)

	if err != nil {
		return nil, err
	}
	return r.GetPromoCodeByID(ctx, promo.ID)
}

func (r *adminRepo) UpdatePromoCode(ctx context.Context, promo *models.PromoCode) error {
	promo.UpdatedAt = time.Now()

	query := `
		UPDATE promo_codes SET
			code = $2, name = $3, description = $4,
			discount_type = $5, discount_value = $6, max_discount_cents = $7, applies_to = $8,
			applies_to_plans = $9, applies_to_billing_intervals = $10, minimum_purchase_cents = $11,
			new_users_only = $12, existing_users_only = $13, specific_user_ids = $14, specific_email_domains = $15,
			max_total_uses = $16, max_uses_per_user = $17,
			starts_at = $18, expires_at = $19, duration_months = $20,
			is_stackable = $21, stackable_with_codes = $22,
			campaign_name = $23, campaign_source = $24,
			updated_at = $25
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query,
		promo.ID, promo.Code, promo.Name, promo.Description,
		promo.DiscountType, promo.DiscountValue, promo.MaxDiscountCents, promo.AppliesTo,
		promo.AppliesToPlans, promo.AppliesToBillingIntervals, promo.MinimumPurchaseCents,
		promo.NewUsersOnly, promo.ExistingUsersOnly, promo.SpecificUserIDs, promo.SpecificEmailDomains,
		promo.MaxTotalUses, promo.MaxUsesPerUser,
		promo.StartsAt, promo.ExpiresAt, promo.DurationMonths,
		promo.IsStackable, promo.StackableWithCodes,
		promo.CampaignName, promo.CampaignSource,
		promo.UpdatedAt,
	)
	return err
}

func (r *adminRepo) DeactivatePromoCode(ctx context.Context, id, deactivatedBy uuid.UUID, reason string) error {
	query := `
		UPDATE promo_codes
		SET is_active = FALSE, deactivated_at = NOW(), deactivated_by = $2, deactivation_reason = $3, updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, id, deactivatedBy, reason)
	return err
}

func (r *adminRepo) GetPromoCodeUsages(ctx context.Context, promoCodeID uuid.UUID, page, limit int) ([]models.PromoCodeUsage, int, error) {
	offset := (page - 1) * limit

	// Count
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM promo_code_usages WHERE promo_code_id = $1", promoCodeID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT pu.id, pu.promo_code_id, pu.user_id, pu.subscription_id, pu.payment_id,
		       pu.discount_applied_cents, pu.used_at,
		       COALESCE(pc.code, '') as promo_code,
		       COALESCE(u.email, '') as user_email,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as user_name
		FROM promo_code_usages pu
		LEFT JOIN promo_codes pc ON pu.promo_code_id = pc.id
		LEFT JOIN users u ON pu.user_id = u.id
		WHERE pu.promo_code_id = $1
		ORDER BY pu.used_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, promoCodeID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var usages []models.PromoCodeUsage
	for rows.Next() {
		var u models.PromoCodeUsage
		if err := rows.Scan(
			&u.ID, &u.PromoCodeID, &u.UserID, &u.SubscriptionID, &u.PaymentID,
			&u.DiscountAppliedCents, &u.UsedAt,
			&u.PromoCode, &u.UserEmail, &u.UserName,
		); err != nil {
			return nil, 0, err
		}
		usages = append(usages, u)
	}
	return usages, total, rows.Err()
}

// ============================================================================
// SUBSCRIPTION PLAN MANAGEMENT
// ============================================================================

func (r *adminRepo) ListSubscriptionPlans(ctx context.Context, activeOnly bool) ([]models.SubscriptionPlan, error) {
	query := `
		SELECT id, name, description, price_cents, billing_interval, features,
		       max_children, max_family_members, is_active,
		       stripe_price_id, stripe_product_id, created_at, updated_at
		FROM subscription_plans
	`
	if activeOnly {
		query += " WHERE is_active = TRUE"
	}
	query += " ORDER BY price_cents ASC"

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []models.SubscriptionPlan
	for rows.Next() {
		var p models.SubscriptionPlan
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.BillingInterval, &p.Features,
			&p.MaxChildren, &p.MaxFamilyMembers, &p.IsActive,
			&p.StripePriceID, &p.StripeProductID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (r *adminRepo) GetSubscriptionPlanByID(ctx context.Context, id uuid.UUID) (*models.SubscriptionPlan, error) {
	query := `
		SELECT id, name, description, price_cents, billing_interval, features,
		       max_children, max_family_members, is_active,
		       stripe_price_id, stripe_product_id, created_at, updated_at
		FROM subscription_plans
		WHERE id = $1
	`
	p := &models.SubscriptionPlan{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.BillingInterval, &p.Features,
		&p.MaxChildren, &p.MaxFamilyMembers, &p.IsActive,
		&p.StripePriceID, &p.StripeProductID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ============================================================================
// FINANCIAL MANAGEMENT
// ============================================================================

func (r *adminRepo) GetFinancialOverview(ctx context.Context) (*models.FinancialOverview, error) {
	overview := &models.FinancialOverview{}

	// Last 24 hours
	r.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(amount_cents), 0)
		FROM payments
		WHERE status = 'succeeded' AND created_at > NOW() - INTERVAL '24 hours'
	`).Scan(&overview.LicensesBought24h, &overview.Revenue24hCents)

	// Month to date
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0)
		FROM payments
		WHERE status = 'succeeded' AND created_at >= DATE_TRUNC('month', NOW())
	`).Scan(&overview.RevenueMTDCents)

	r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM family_subscriptions
		WHERE created_at >= DATE_TRUNC('month', NOW()) AND status = 'active'
	`).Scan(&overview.NewSubscriptionsMTD)

	r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM family_subscriptions
		WHERE cancelled_at >= DATE_TRUNC('month', NOW())
	`).Scan(&overview.ChurnedSubscriptionsMTD)

	// Year to date
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0)
		FROM payments
		WHERE status = 'succeeded' AND created_at >= DATE_TRUNC('year', NOW())
	`).Scan(&overview.RevenueYTDCents)

	r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM family_subscriptions
		WHERE status = 'active'
	`).Scan(&overview.TotalActiveSubscriptions)

	// Subscriptions by plan (using family_subscriptions)
	rows, err := r.db.QueryContext(ctx, `
		SELECT sp.id, sp.name, COUNT(fs.id) as count,
		       CASE
		           WHEN sp.billing_interval = 'monthly' THEN COALESCE(SUM(sp.price_cents), 0)
		           WHEN sp.billing_interval = 'yearly' THEN COALESCE(SUM(sp.price_cents), 0) / 12
		           ELSE 0
		       END as mrr_cents
		FROM subscription_plans sp
		LEFT JOIN family_subscriptions fs ON sp.id = fs.plan_id AND fs.status = 'active'
		WHERE sp.is_active = TRUE
		GROUP BY sp.id, sp.name, sp.billing_interval
		ORDER BY sp.price_cents ASC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var psc models.PlanSubscriptionCount
			if err := rows.Scan(&psc.PlanID, &psc.PlanName, &psc.Count, &psc.MRRCents); err == nil {
				overview.SubscriptionsByPlan = append(overview.SubscriptionsByPlan, psc)
			}
		}
	}

	// Total discounts YTD
	r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(discount_amount_cents), 0)
		FROM payments
		WHERE created_at >= DATE_TRUNC('year', NOW())
	`).Scan(&overview.TotalDiscountsYTDCents)

	return overview, nil
}

func (r *adminRepo) GetExpectedRevenueCalendar(ctx context.Context, startDate, endDate time.Time) ([]models.ExpectedRevenueDay, error) {
	query := `
		SELECT expected_date, SUM(expected_amount_cents) as amount_cents, COUNT(*) as renewal_count
		FROM expected_revenue_calendar
		WHERE expected_date >= $1 AND expected_date <= $2
		GROUP BY expected_date
		ORDER BY expected_date ASC
	`

	rows, err := r.db.QueryContext(ctx, query, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []models.ExpectedRevenueDay
	for rows.Next() {
		var d models.ExpectedRevenueDay
		if err := rows.Scan(&d.Date, &d.AmountCents, &d.RenewalCount); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

func (r *adminRepo) GetRecentPayments(ctx context.Context, page, limit int) ([]models.Payment, int, error) {
	offset := (page - 1) * limit

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM payments").Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT p.id, p.subscription_id, p.user_id, p.payment_type, p.amount_cents, p.currency,
		       p.status, p.payment_method, p.stripe_payment_intent_id, p.stripe_invoice_id,
		       p.description, p.promo_code_id, p.discount_amount_cents, p.refund_amount_cents,
		       p.refunded_at, p.failure_reason, p.metadata, p.created_at, p.updated_at,
		       COALESCE(u.email, '') as user_email,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as user_name,
		       COALESCE(pc.code, '') as promo_code,
		       COALESCE(sp.name, '') as plan_name
		FROM payments p
		LEFT JOIN users u ON p.user_id = u.id
		LEFT JOIN promo_codes pc ON p.promo_code_id = pc.id
		LEFT JOIN user_subscriptions us ON p.subscription_id = us.id
		LEFT JOIN subscription_plans sp ON us.plan_id = sp.id
		ORDER BY p.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var payments []models.Payment
	for rows.Next() {
		var p models.Payment
		if err := rows.Scan(
			&p.ID, &p.SubscriptionID, &p.UserID, &p.PaymentType, &p.AmountCents, &p.Currency,
			&p.Status, &p.PaymentMethod, &p.StripePaymentIntentID, &p.StripeInvoiceID,
			&p.Description, &p.PromoCodeID, &p.DiscountAmountCents, &p.RefundAmountCents,
			&p.RefundedAt, &p.FailureReason, &p.Metadata, &p.CreatedAt, &p.UpdatedAt,
			&p.UserEmail, &p.UserName, &p.PromoCode, &p.PlanName,
		); err != nil {
			return nil, 0, err
		}
		payments = append(payments, p)
	}
	return payments, total, rows.Err()
}

func (r *adminRepo) GetRecentSubscriptions(ctx context.Context, page, limit int) ([]models.UserSubscription, int, error) {
	offset := (page - 1) * limit

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_subscriptions").Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT s.id, s.user_id, s.plan_id, s.status, s.current_period_start, s.current_period_end,
		       s.trial_end, s.cancelled_at, s.cancel_at_period_end,
		       s.stripe_subscription_id, s.stripe_customer_id, s.promo_code_id,
		       s.created_at, s.updated_at,
		       COALESCE(sp.name, '') as plan_name,
		       COALESCE(u.email, '') as user_email,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as user_name
		FROM user_subscriptions s
		LEFT JOIN subscription_plans sp ON s.plan_id = sp.id
		LEFT JOIN users u ON s.user_id = u.id
		ORDER BY s.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var subs []models.UserSubscription
	for rows.Next() {
		var s models.UserSubscription
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.PlanID, &s.Status, &s.CurrentPeriodStart, &s.CurrentPeriodEnd,
			&s.TrialEnd, &s.CancelledAt, &s.CancelAtPeriodEnd,
			&s.StripeSubscriptionID, &s.StripeCustomerID, &s.PromoCodeID,
			&s.CreatedAt, &s.UpdatedAt,
			&s.PlanName, &s.UserEmail, &s.UserName,
		); err != nil {
			return nil, 0, err
		}
		subs = append(subs, s)
	}
	return subs, total, rows.Err()
}

func (r *adminRepo) GetDailyRevenueSnapshots(ctx context.Context, startDate, endDate time.Time) ([]models.DailyRevenueSnapshot, error) {
	query := `
		SELECT id, snapshot_date, total_revenue_cents, new_subscriptions, cancelled_subscriptions,
		       upgrades, downgrades, refunds_cents, promo_discounts_cents, calculated_at
		FROM daily_revenue_snapshots
		WHERE snapshot_date >= $1 AND snapshot_date <= $2
		ORDER BY snapshot_date ASC
	`

	rows, err := r.db.QueryContext(ctx, query, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []models.DailyRevenueSnapshot
	for rows.Next() {
		var s models.DailyRevenueSnapshot
		if err := rows.Scan(
			&s.ID, &s.SnapshotDate, &s.TotalRevenueCents, &s.NewSubscriptions, &s.CancelledSubscriptions,
			&s.Upgrades, &s.Downgrades, &s.RefundsCents, &s.PromoDiscountsCents, &s.CalculatedAt,
		); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// itoa is a helper function for building dynamic queries
func itoa(n int) string {
	return strconv.Itoa(n)
}

// ============================================================================
// Family-subscription admin tooling
// ============================================================================
//
// These methods power the /admin/super/subscriptions page. They join in the
// family name + plan name + an aggregated parent-email string so the listing
// view doesn't need a second round-trip per row. UpdateFamilySubscription is
// a generic editor used by the modal; CompFamilySubscription is a convenience
// wrapper that sets the right combination of status/comp_reason/comped_by/
// comp_until in one call so audit trails stay consistent.

const familySubscriptionListColumns = `
    fs.id, fs.family_id, fs.plan_id, fs.status,
    fs.current_period_start, fs.current_period_end,
    fs.trial_end, fs.cancelled_at, fs.cancel_at_period_end,
    fs.stripe_subscription_id, fs.stripe_customer_id, fs.promo_code_id,
    fs.comp_reason, fs.comped_by, fs.comp_until,
    fs.created_at, fs.updated_at,
    sp.name AS plan_name,
    f.name  AS family_name,
    (SELECT count(*) FROM children c WHERE c.family_id = f.id AND c.is_active = true) AS child_count,
    COALESCE(
        (SELECT string_agg(u.email, ', ' ORDER BY u.email)
         FROM family_memberships fm
         JOIN users u ON u.id = fm.user_id
         WHERE fm.family_id = f.id AND fm.role = 'parent' AND fm.is_active = true),
        ''
    ) AS parent_emails`

func scanFamilySubscriptionRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.FamilySubscription, error) {
	s := &models.FamilySubscription{}
	err := scanner.Scan(
		&s.ID, &s.FamilyID, &s.PlanID, &s.Status,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd,
		&s.TrialEnd, &s.CancelledAt, &s.CancelAtPeriodEnd,
		&s.StripeSubscriptionID, &s.StripeCustomerID, &s.PromoCodeID,
		&s.CompReason, &s.CompedBy, &s.CompUntil,
		&s.CreatedAt, &s.UpdatedAt,
		&s.PlanName, &s.FamilyName, &s.ChildCount, &s.ParentEmails,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ListFamilySubscriptions returns paginated rows + total. statusFilter and
// planFilter are matched exactly when non-empty; search matches family name
// or any parent email (case-insensitive substring).
func (r *adminRepo) ListFamilySubscriptions(ctx context.Context, statusFilter, planFilter, search string, page, limit int) ([]models.FamilySubscription, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	whereParts := []string{"1=1"}
	args := []interface{}{}
	argN := 0
	addArg := func(v interface{}) string {
		argN++
		args = append(args, v)
		return "$" + itoa(argN)
	}

	if statusFilter != "" {
		whereParts = append(whereParts, "fs.status = "+addArg(statusFilter))
	}
	if planFilter != "" {
		whereParts = append(whereParts, "fs.plan_id = "+addArg(planFilter))
	}
	if search != "" {
		// Match family name or any parent email substring.
		ph := addArg("%" + strings.ToLower(search) + "%")
		whereParts = append(whereParts,
			"(LOWER(f.name) LIKE "+ph+
				" OR EXISTS (SELECT 1 FROM family_memberships fm2 JOIN users u2 ON u2.id = fm2.user_id "+
				"WHERE fm2.family_id = f.id AND fm2.role = 'parent' AND LOWER(u2.email) LIKE "+ph+"))")
	}
	where := strings.Join(whereParts, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `
        SELECT count(*)
        FROM family_subscriptions fs
        JOIN families f ON f.id = fs.family_id
        WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
        SELECT `+familySubscriptionListColumns+`
        FROM family_subscriptions fs
        JOIN subscription_plans sp ON sp.id = fs.plan_id
        JOIN families f            ON f.id  = fs.family_id
        WHERE `+where+`
        ORDER BY
            CASE fs.status
                WHEN 'past_due'   THEN 1
                WHEN 'trialing'   THEN 2
                WHEN 'active'     THEN 3
                WHEN 'comped'     THEN 4
                WHEN 'paused'     THEN 5
                WHEN 'cancelled'  THEN 6
                WHEN 'expired'    THEN 7
                WHEN 'terminated' THEN 8
                ELSE 9
            END,
            fs.current_period_end ASC
        LIMIT $`+itoa(argN+1)+` OFFSET $`+itoa(argN+2), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []models.FamilySubscription{}
	for rows.Next() {
		s, err := scanFamilySubscriptionRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *s)
	}
	return out, total, rows.Err()
}

func (r *adminRepo) GetFamilySubscriptionByID(ctx context.Context, id uuid.UUID) (*models.FamilySubscription, error) {
	row := r.db.QueryRowContext(ctx, `
        SELECT `+familySubscriptionListColumns+`
        FROM family_subscriptions fs
        JOIN subscription_plans sp ON sp.id = fs.plan_id
        JOIN families f            ON f.id  = fs.family_id
        WHERE fs.id = $1`, id)
	return scanFamilySubscriptionRow(row)
}

func (r *adminRepo) GetFamilySubscriptionByFamilyID(ctx context.Context, familyID uuid.UUID) (*models.FamilySubscription, error) {
	row := r.db.QueryRowContext(ctx, `
        SELECT `+familySubscriptionListColumns+`
        FROM family_subscriptions fs
        JOIN subscription_plans sp ON sp.id = fs.plan_id
        JOIN families f            ON f.id  = fs.family_id
        WHERE fs.family_id = $1`, familyID)
	return scanFamilySubscriptionRow(row)
}

// UpdateFamilySubscription writes the editable fields. Stripe IDs are not
// touched here — those only change via the webhook receiver in Phase 3.
func (r *adminRepo) UpdateFamilySubscription(ctx context.Context, sub *models.FamilySubscription) error {
	_, err := r.db.ExecContext(ctx, `
        UPDATE family_subscriptions SET
            plan_id              = $2,
            status               = $3,
            current_period_start = $4,
            current_period_end   = $5,
            trial_end            = $6,
            cancelled_at         = $7,
            cancel_at_period_end = $8,
            comp_reason          = $9,
            comped_by            = $10,
            comp_until           = $11,
            updated_at           = NOW()
        WHERE id = $1`,
		sub.ID, sub.PlanID, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.TrialEnd, sub.CancelledAt, sub.CancelAtPeriodEnd,
		sub.CompReason, sub.CompedBy, sub.CompUntil,
	)
	return err
}

// CompFamilySubscription UPSERTs a family onto the chosen plan as comped
// through `until`. Used by the "Comp this family" admin button. If the
// family already has a subscription row, it's updated in place. Comping
// also clears past_due_since since the comp restores access — otherwise
// the termination clock could fire after the comp lapses, even though the
// family was healthy throughout the comp window.
func (r *adminRepo) CompFamilySubscription(ctx context.Context, familyID, planID, compedBy uuid.UUID, reason string, until time.Time) (*models.FamilySubscription, error) {
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO family_subscriptions (
            family_id, plan_id, status, current_period_start, current_period_end,
            comp_reason, comped_by, comp_until, cancel_at_period_end
        )
        VALUES ($1, $2, 'comped', NOW(), $3, $4, $5, $3, false)
        ON CONFLICT (family_id) DO UPDATE SET
            plan_id              = EXCLUDED.plan_id,
            status               = 'comped',
            current_period_end   = EXCLUDED.current_period_end,
            comp_reason          = EXCLUDED.comp_reason,
            comped_by            = EXCLUDED.comped_by,
            comp_until           = EXCLUDED.comp_until,
            cancel_at_period_end = false,
            cancelled_at         = NULL,
            past_due_since       = NULL,
            updated_at           = NOW()`,
		familyID, planID, until, reason, compedBy,
	)
	if err != nil {
		return nil, err
	}
	return r.GetFamilySubscriptionByFamilyID(ctx, familyID)
}

// CancelFamilySubscription marks a subscription cancelled. If immediate is
// true, the period_end is also moved to NOW() (forces enforcement to kick
// in immediately on the next request).
func (r *adminRepo) CancelFamilySubscription(ctx context.Context, familyID, cancelledBy uuid.UUID, immediate bool) error {
	if immediate {
		_, err := r.db.ExecContext(ctx, `
            UPDATE family_subscriptions SET
                status               = 'cancelled',
                cancelled_at         = NOW(),
                current_period_end   = NOW(),
                cancel_at_period_end = false,
                updated_at           = NOW()
            WHERE family_id = $1`, familyID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
        UPDATE family_subscriptions SET
            cancel_at_period_end = true,
            cancelled_at         = NOW(),
            updated_at           = NOW()
        WHERE family_id = $1`, familyID)
	return err
}
