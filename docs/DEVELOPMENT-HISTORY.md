# CareCompanion Development History

**Document Purpose**: Chronicle of problems encountered, solutions implemented, and design decisions made during development. This document helps future developers (or AI assistants) understand why certain approaches were taken.

---

## Session: January 7-9, 2026

### Feature: Weekly View for Daily Logs

**Requirement**: Users should be able to toggle between "Daily" and "Weekly" views on the daily logs page. Weekly view shows Monday 00:00 through Sunday 23:59.

#### Problem 1: JavaScript Button Not Working

**Symptom**: Clicking "Weekly" button did nothing.

**Root Cause**: Go server was not restarted after code changes. Go is a compiled language and doesn't hot-reload.

**Solution**: Kill the running server process and rebuild:
```bash
pkill -f carecompanion
go build -o carecompanion ./cmd/server
./carecompanion
```

**Lesson**: Always restart the Go server after backend code changes.

---

#### Problem 2: Weekly View Only Showing One Day's Data

**Symptom**: When selecting weekly view for week of 1/5-1/11, only entries from 1/7 (today) were displayed despite having entries on 1/5, 1/6, and 1/7.

**Initial Investigation**:
- Verified the date range calculation was correct (Monday-Sunday)
- Query was passing `startDate` and `endDate` as `time.Time` objects
- PostgreSQL `DATE` column comparisons were failing silently

**Root Cause**: PostgreSQL timezone conversion issues when comparing Go `time.Time` with PostgreSQL `DATE` columns.

When passing `time.Time` to a query against a `DATE` column, PostgreSQL converts the timestamp to UTC, which can shift the date. For example:
- Go: `2026-01-05 00:00:00 CST` (Central Time)
- PostgreSQL receives: `2026-01-05 06:00:00 UTC`
- Date comparison: `2026-01-05` but timezone-aware comparison may shift

**Failed Attempt**: Adding `::date` casting to SQL queries:
```sql
WHERE log_date >= $2::date AND log_date <= $3::date
```
This did not resolve the issue.

**Working Solution**: Format dates as ISO strings before passing to queries:
```go
// BEFORE (broken)
rows, err := r.db.QueryContext(ctx, query, childID, startDate, endDate)

// AFTER (working)
startStr := startDate.Format("2006-01-02")
endStr := endDate.Format("2006-01-02")
rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
```

**Files Modified**: All 12 log query functions in `internal/repository/log_repo.go`:
- `GetBehaviorLogs()`
- `GetSleepLogs()`
- `GetMealLogs()`
- `GetSymptomLogs()`
- `GetSchoolLogs()`
- `GetBowelLogs()`
- `GetAppointmentLogs()`
- `GetSocialLogs()`
- `GetEventLogs()`
- `getMedicationLogsForDateRange()`
- Plus date range variants

**Lesson**: When querying PostgreSQL DATE columns from Go, always format dates as strings in `YYYY-MM-DD` format to avoid timezone conversion issues.

---

#### Problem 3: Entries Showing Wrong Date

**Symptom**: An entry logged for 1/5 displayed as "1/7" in the entries list.

**Root Cause**: Template was displaying `CreatedAt` (when the entry was saved to the database) instead of `LogDate` (the date the entry is for).

**Context**: Users often log entries for past dates. For example, logging yesterday's sleep at 8am today creates:
- `LogDate`: Yesterday
- `CreatedAt`: Today 8am

**Solution**: Changed template to use `LogDate` for the date portion:
```html
<!-- BEFORE -->
<p>{{.CreatedAt.Format "Mon 1/2 3:04 PM"}}</p>

<!-- AFTER -->
<p>{{.LogDate.Format "Mon 1/2"}} {{.CreatedAt.Format "3:04 PM"}}</p>
```

The time portion still uses `CreatedAt` because `LogDate` is DATE only (no time component).

**Files Modified**: `templates/daily_logs.html` - all entry display sections

---

#### Problem 4: Template Variable Scope in Range Loops

**Symptom**: `{{if eq .ViewMode "weekly"}}` inside a `{{range}}` loop didn't work.

**Root Cause**: Inside a `{{range}}` loop, `.` refers to the current item, not the root context.

**Solution**: Use `$` to access root context:
```html
{{range .Logs.BehaviorLogs}}
    {{if eq $.ViewMode "weekly"}}
        <!-- This works - $.ViewMode accesses root -->
    {{end}}
{{end}}
```

**Lesson**: Go templates use `$` for root context access within loops.

---

### Feature: Family Member CRUD

**Requirement**: Full create, read, update, delete operations for family members from the settings page.

**Implementation**:
- Added API endpoints in `internal/handler/api/family_handler.go`
- Added repository methods in `internal/repository/family_repo.go`
- Added service methods in `internal/service/family_service.go`
- Updated settings template with member management UI

**No significant problems encountered** - straightforward CRUD implementation.

---

### Feature: Sleep Entry Options Adjustment

**Requirement**: Modify selectable options for sleep duration and quality.

**Implementation**: Updated the sleep log form in `templates/daily_logs.html`

**No significant problems encountered**.

---

## Architecture Decisions

### Decision 1: Repository Pattern

**Choice**: Implemented repository pattern separating data access from business logic.

**Rationale**:
- Clean separation of concerns
- Easier testing (can mock repositories)
- Database-agnostic business logic
- Consistent data access patterns

**Trade-off**: More boilerplate code, but worth it for maintainability.

---

### Decision 2: Service Layer

**Choice**: Business logic lives in service layer, handlers only handle HTTP.

**Rationale**:
- Handlers stay thin and focused on request/response
- Business rules centralized and reusable
- Easier to add new interfaces (API, CLI, etc.)

---

### Decision 3: JWT + Redis Sessions

**Choice**: JWT tokens stored in Redis with configurable expiry.

**Rationale**:
- JWT provides stateless authentication
- Redis enables session invalidation (logout)
- Can check token validity without database hit
- Easy horizontal scaling

---

### Decision 4: Single Database Connection Pool

**Choice**: Using `pgx` pool with connection reuse.

**Rationale**:
- Efficient connection management
- Built-in connection health checks
- Native PostgreSQL support (no ORM overhead)

---

### Decision 5: Template-Based Rendering

**Choice**: Server-side HTML rendering with Go templates instead of SPA.

**Rationale**:
- Simpler architecture for MVP
- Better SEO potential
- No JavaScript framework complexity
- Faster initial page loads

**Trade-off**: Less interactive UI, requires page refreshes for updates.

---

## Technical Debt Identified

1. **No WebSocket for Chat**: Currently requires page refresh to see new messages. Should add WebSocket for real-time updates.

2. **Local File Storage**: Chat attachments stored locally. Needs migration to S3 for production.

3. **No Rate Limiting**: API endpoints have no rate limiting. Should add before production.

4. **No Request Validation Library**: Manual validation in handlers. Consider using `go-playground/validator`.

5. **Alert Generation Timing**: Alerts generated synchronously on log creation. Should move to background job for better UX.

6. **No Caching Layer**: All database queries hit PostgreSQL directly. Consider Redis caching for frequent reads.

7. **Missing Database Indexes**: Need to audit and add indexes for common query patterns.

8. **No Structured Logging**: Using basic `log.Printf`. Should migrate to structured logging (zerolog, zap).

---

## Performance Notes

### Database Queries

**Observed Response Times** (from server logs):
- Dashboard load: ~5-8ms
- Daily logs page: ~7-24ms
- Weekly logs (date range): ~8-27ms
- Medication search: ~200-800ms (OpenFDA API)
- Alert analysis: ~3-5ms

**Bottlenecks Identified**:
1. OpenFDA drug search is slow (external API)
2. Weekly view with many entries takes longer

---

## Environment Specifics

### Development Environment
- **OS**: Linux (Ubuntu-based)
- **Go Version**: 1.21+
- **PostgreSQL**: Local instance, port 5432
- **Redis**: Local instance, port 6379
- **Server Port**: 8090

### Timezone Handling
- User timezone stored in preferences (default: America/New_York)
- All dates displayed in user's timezone
- Database stores dates as DATE (no time component for LogDate)
- Database stores timestamps in UTC for CreatedAt/UpdatedAt

---

## Code Patterns Used

### Error Handling
```go
if err != nil {
    return nil, fmt.Errorf("failed to get logs: %w", err)
}
```
Always wrap errors with context using `fmt.Errorf` and `%w`.

### Context Propagation
```go
func (s *Service) DoSomething(ctx context.Context, ...) error {
    return s.repo.Query(ctx, ...)
}
```
Always pass context through the call chain.

### UUID Usage
```go
import "github.com/google/uuid"

id := uuid.New()
parsed, err := uuid.Parse(idString)
```
Using Google's UUID library for all entity IDs.

---

## Testing Notes

### Manual Testing Performed
- Login/logout flow
- Child creation and management
- Log entry creation for all types
- Weekly view with multiple days of data
- Timezone handling (Central Time)
- Family member management
- Medication search and tracking

### Areas Needing Automated Tests
- Repository layer (database queries)
- Service layer (business logic)
- Handler layer (HTTP endpoints)
- Date range calculations
- Timezone conversions

---

## External Dependencies

| Dependency | Purpose | Notes |
|------------|---------|-------|
| `chi/v5` | HTTP router | Lightweight, stdlib compatible |
| `pgx/v5` | PostgreSQL driver | Native, high-performance |
| `redis/v9` | Redis client | Official client |
| `google/uuid` | UUID generation | Industry standard |
| `golang-jwt/jwt` | JWT handling | Popular, maintained |
| `golang.org/x/crypto/bcrypt` | Password hashing | Stdlib extension |

---

## Useful Commands

### Rebuild and Restart Server
```bash
pkill -f carecompanion && go build -o carecompanion ./cmd/server && ./carecompanion
```

### Check Server Logs
```bash
tail -f /tmp/server.log
```

### Database Access
```bash
psql -h localhost -U carecompanion -d carecompanion
```

### Test Specific Query
```sql
SELECT * FROM behavior_logs
WHERE child_id = 'dddd0002-0002-0002-0002-000000000002'
AND log_date >= '2026-01-05' AND log_date <= '2026-01-11';
```
