package middleware

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// ErrorTracker handles error logging and automatic ticket creation
type ErrorTracker struct {
	db *sql.DB
	mu sync.Mutex
}

// NewErrorTracker creates a new error tracker
func NewErrorTracker(db *sql.DB) *ErrorTracker {
	return &ErrorTracker{db: db}
}

// errorResponseWriter captures response body for error responses
type errorResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (w *errorResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *errorResponseWriter) Write(b []byte) (int, error) {
	// Capture body for error responses
	if w.statusCode >= 400 {
		w.body = append(w.body, b...)
	}
	return w.ResponseWriter.Write(b)
}

// Middleware returns the error tracking middleware
func (et *ErrorTracker) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer
		wrapped := &errorResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next.ServeHTTP(wrapped, r)

		// Calculate response time
		responseTime := float64(time.Since(start).Milliseconds())

		// Log response time (async)
		go et.logResponseTime(r.URL.Path, r.Method, responseTime, wrapped.statusCode)

		// Check for errors
		if wrapped.statusCode >= 400 {
			go et.handleError(r, wrapped)
		}
	})
}

func (et *ErrorTracker) logResponseTime(path, method string, responseTimeMs float64, statusCode int) {
	if et.db == nil {
		return
	}

	// Skip static assets and health checks
	if strings.HasPrefix(path, "/static") || path == "/health" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := et.db.ExecContext(ctx,
		`INSERT INTO response_time_logs (path, method, response_time_ms, status_code) VALUES ($1, $2, $3, $4)`,
		path, method, responseTimeMs, statusCode)
	if err != nil {
		log.Printf("Failed to log response time: %v", err)
	}

	// Cleanup old logs (keep only last 24 hours)
	et.db.ExecContext(ctx, `DELETE FROM response_time_logs WHERE created_at < NOW() - INTERVAL '24 hours'`)
}

func (et *ErrorTracker) handleError(r *http.Request, wrapped *errorResponseWriter) {
	if et.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Determine error type
	var errorType string
	switch {
	case wrapped.statusCode >= 500:
		errorType = "server_error"
	case wrapped.statusCode == 401 || wrapped.statusCode == 403:
		errorType = "auth_error"
	case wrapped.statusCode == 404:
		errorType = "not_found"
	case wrapped.statusCode >= 400:
		errorType = "bad_request"
	default:
		errorType = "unknown"
	}

	// Get user ID from context if available
	var userID *uuid.UUID
	if claims := GetAuthClaims(r.Context()); claims != nil {
		userID = &claims.UserID
	}

	// Get request ID
	requestID := chimiddleware.GetReqID(r.Context())

	// Get error message from response body
	errorMessage := string(wrapped.body)
	if len(errorMessage) > 1000 {
		errorMessage = errorMessage[:1000]
	}

	// Get IP address
	ipAddress := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ipAddress = strings.Split(forwarded, ",")[0]
	}
	// Remove port from IP address
	if idx := strings.LastIndex(ipAddress, ":"); idx != -1 {
		ipAddress = ipAddress[:idx]
	}

	// Insert error log
	var errorLogID uuid.UUID
	err := et.db.QueryRowContext(ctx,
		`INSERT INTO error_logs (user_id, error_type, status_code, path, method, error_message, user_agent, ip_address, request_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, $9)
		 RETURNING id`,
		userID, errorType, wrapped.statusCode, r.URL.Path, r.Method, errorMessage, r.UserAgent(), ipAddress, requestID,
	).Scan(&errorLogID)
	if err != nil {
		log.Printf("Failed to log error: %v", err)
		return
	}

	// Auto-create support ticket for server errors (5xx)
	if wrapped.statusCode >= 500 {
		et.createErrorTicket(ctx, errorLogID, r, errorType, wrapped.statusCode, errorMessage)
	}
}

func (et *ErrorTracker) createErrorTicket(ctx context.Context, errorLogID uuid.UUID, r *http.Request, errorType string, statusCode int, errorMessage string) {
	et.mu.Lock()
	defer et.mu.Unlock()

	// Check if we recently created a ticket for a similar error (within last 5 minutes)
	var recentCount int
	et.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM support_tickets 
		 WHERE subject LIKE 'Auto: Server Error%' 
		 AND created_at > NOW() - INTERVAL '5 minutes'`).Scan(&recentCount)

	if recentCount >= 5 {
		// Don't flood with tickets - just update the log
		log.Printf("Suppressing auto-ticket creation (too many recent tickets)")
		return
	}

	// Create ticket
	subject := "Auto: Server Error " + r.URL.Path
	if len(subject) > 255 {
		subject = subject[:255]
	}

	description := "Automatic ticket created due to server error.\n\n"
	description += "Path: " + r.Method + " " + r.URL.Path + "\n"
	description += "Status Code: " + http.StatusText(statusCode) + " (" + string(rune(statusCode)) + ")\n"
	description += "Error Type: " + errorType + "\n"
	description += "Time: " + time.Now().Format(time.RFC3339) + "\n"
	if errorMessage != "" {
		description += "\nError Details:\n" + errorMessage
	}

	var ticketID uuid.UUID
	err := et.db.QueryRowContext(ctx,
		`INSERT INTO support_tickets (subject, description, status, priority)
		 VALUES ($1, $2, 'open', 'high')
		 RETURNING id`,
		subject, description,
	).Scan(&ticketID)

	if err != nil {
		log.Printf("Failed to create auto-ticket: %v", err)
		return
	}

	// Link ticket to error log
	et.db.ExecContext(ctx, `UPDATE error_logs SET ticket_id = $1 WHERE id = $2`, ticketID, errorLogID)

	log.Printf("Auto-created support ticket %s for error on %s", ticketID, r.URL.Path)
}
