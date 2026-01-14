package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

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
	ID          uuid.UUID         `json:"id"`
	UserID      models.NullUUID   `json:"user_id,omitempty"`
	Subject     string            `json:"subject"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Priority    string            `json:"priority"`
	AssignedTo  models.NullUUID   `json:"assigned_to,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ResolvedAt  models.NullTime   `json:"resolved_at,omitempty"`
	ResolvedBy  models.NullUUID   `json:"resolved_by,omitempty"`
	// Populated when needed
	UserEmail    string `json:"user_email,omitempty"`
	AssigneeName string `json:"assignee_name,omitempty"`
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
	CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority string) (*SupportTicket, error)
	GetTickets(ctx context.Context, status string, page, limit int) ([]SupportTicket, int, error)
	GetTicketByID(ctx context.Context, id uuid.UUID) (*SupportTicket, error)
	UpdateTicketStatus(ctx context.Context, id uuid.UUID, status string) error
	AssignTicket(ctx context.Context, ticketID, assigneeID uuid.UUID) error
	ResolveTicket(ctx context.Context, ticketID, resolverID uuid.UUID) error
	GetTicketMessages(ctx context.Context, ticketID uuid.UUID) ([]TicketMessage, error)
	AddTicketMessage(ctx context.Context, ticketID, senderID uuid.UUID, message string, isInternal bool) error

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
}

// adminRepo implements AdminRepository
type adminRepo struct {
	db *sql.DB
}

// NewAdminRepo creates a new admin repository
func NewAdminRepo(db *sql.DB) AdminRepository {
	return &adminRepo{db: db}
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
	query := `UPDATE users SET status = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, status)
	return err
}

func (r *adminRepo) ResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error {
	query := `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, newHash)
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
	query := `
		SELECT u.id, u.email, u.first_name, u.last_name, u.phone, u.status, u.system_role,
		       u.created_at, u.last_login_at,
		       (SELECT COUNT(*) FROM family_memberships WHERE user_id = u.id AND is_active = true) as family_count
		FROM users u
		WHERE u.system_role IS NOT NULL
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
		INSERT INTO users (id, email, password_hash, first_name, last_name, system_role, status, created_at, updated_at)
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
	query := `UPDATE users SET system_role = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, role)
	return err
}

func (r *adminRepo) RemoveAdminRole(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE users SET system_role = NULL, updated_at = NOW() WHERE id = $1`
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

func (r *adminRepo) CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority string) (*SupportTicket, error) {
	id := uuid.New()
	now := time.Now()
	if priority == "" {
		priority = "normal"
	}
	query := `
		INSERT INTO support_tickets (id, user_id, subject, description, priority, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING id
	`
	var userIDPtr *uuid.UUID
	if userID != uuid.Nil {
		userIDPtr = &userID
	}
	err := r.db.QueryRowContext(ctx, query, id, userIDPtr, subject, description, priority, now).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.GetTicketByID(ctx, id)
}

func (r *adminRepo) GetTickets(ctx context.Context, status string, page, limit int) ([]SupportTicket, int, error) {
	offset := (page - 1) * limit

	// Count query
	countArgs := []interface{}{}
	countSQL := "SELECT COUNT(*) FROM support_tickets"
	if status != "" {
		countSQL += " WHERE status = $1"
		countArgs = append(countArgs, status)
	}
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get tickets
	args := []interface{}{}
	query := `
		SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       COALESCE(u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id
	`
	if status != "" {
		query += " WHERE t.status = $1"
		args = append(args, status)
		query += " ORDER BY t.created_at DESC LIMIT $2 OFFSET $3"
		args = append(args, limit, offset)
	} else {
		query += " ORDER BY t.created_at DESC LIMIT $1 OFFSET $2"
		args = append(args, limit, offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tickets []SupportTicket
	for rows.Next() {
		var t SupportTicket
		if err := rows.Scan(&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority,
			&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
			&t.UserEmail, &t.AssigneeName); err != nil {
			return nil, 0, err
		}
		tickets = append(tickets, t)
	}
	return tickets, total, rows.Err()
}

func (r *adminRepo) GetTicketByID(ctx context.Context, id uuid.UUID) (*SupportTicket, error) {
	query := `
		SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       COALESCE(u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id
		WHERE t.id = $1
	`
	t := &SupportTicket{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority,
		&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
		&t.UserEmail, &t.AssigneeName,
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
	_, err := r.db.ExecContext(ctx, query, id, status)
	return err
}

func (r *adminRepo) AssignTicket(ctx context.Context, ticketID, assigneeID uuid.UUID) error {
	query := `UPDATE support_tickets SET assigned_to = $2, status = 'in_progress', updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, ticketID, assigneeID)
	return err
}

func (r *adminRepo) ResolveTicket(ctx context.Context, ticketID, resolverID uuid.UUID) error {
	query := `UPDATE support_tickets SET status = 'resolved', resolved_at = NOW(), resolved_by = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, ticketID, resolverID)
	return err
}

func (r *adminRepo) GetTicketMessages(ctx context.Context, ticketID uuid.UUID) ([]TicketMessage, error) {
	query := `
		SELECT m.id, m.ticket_id, m.sender_id, m.message, m.is_internal, m.created_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as sender_name,
		       COALESCE(u.email, '') as sender_email
		FROM ticket_messages m
		LEFT JOIN users u ON m.sender_id = u.id
		WHERE m.ticket_id = $1
		ORDER BY m.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, ticketID)
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
	query := `INSERT INTO ticket_messages (id, ticket_id, sender_id, message, is_internal, created_at) VALUES ($1, $2, $3, $4, $5, NOW())`
	_, err := r.db.ExecContext(ctx, query, id, ticketID, senderID, message, isInternal)
	if err != nil {
		return err
	}
	// Update ticket updated_at
	_, err = r.db.ExecContext(ctx, "UPDATE support_tickets SET updated_at = NOW() WHERE id = $1", ticketID)
	return err
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
