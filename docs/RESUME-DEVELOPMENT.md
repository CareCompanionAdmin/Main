# Quick Start: Resuming CareCompanion Development

**Purpose**: This document enables quick onboarding for new Claude Code sessions or developers.

---

## TL;DR

CareCompanion is a Go web application for healthcare monitoring. It's feature-complete for alpha testing and ready for AWS deployment.

**Key Files to Read First**:
1. `docs/PROJECT-STATUS.md` - What's implemented
2. `docs/DEVELOPMENT-HISTORY.md` - Problems solved and lessons learned
3. `docs/AWS-DEPLOYMENT-GUIDE.md` - Deployment instructions
4. `internal/handler/web/handlers.go` - Web page handlers
5. `internal/handler/api/routes.go` - API route definitions

---

## Project Structure Quick Reference

```
/home/carecomp/carecompanion/
├── cmd/server/main.go       # Entry point - starts HTTP server
├── internal/
│   ├── handler/api/         # REST API handlers
│   ├── handler/web/         # Web page handlers
│   ├── service/             # Business logic
│   ├── repository/          # Database queries
│   └── models/              # Data structures
├── templates/               # HTML templates
├── docs/                    # Documentation
└── infrastructure/          # AWS deployment files
```

---

## How to Run Locally

### Prerequisites
- Go 1.21+
- PostgreSQL running on localhost:5432
- Redis running on localhost:6379
- Database `carecompanion` created

### Start Server
```bash
cd /home/carecomp/carecompanion
go build -o carecompanion ./cmd/server
./carecompanion
```

Server starts on `http://localhost:8090`

### Rebuild After Changes
```bash
pkill -f carecompanion && go build -o carecompanion ./cmd/server && ./carecompanion
```

### View Server Logs
```bash
tail -f /tmp/server.log
```

---

## Test Credentials

- **URL**: http://localhost:8090
- **Email**: testparent@example.com
- **Password**: password123
- **Test Child ID**: dddd0002-0002-0002-0002-000000000002

---

## Common Tasks

### Add a New API Endpoint

1. Define handler in `internal/handler/api/{resource}_handler.go`
2. Add route in `internal/handler/api/routes.go`
3. Add service method in `internal/service/{resource}_service.go`
4. Add repository method in `internal/repository/{resource}_repo.go`

### Add a New Web Page

1. Create template in `templates/{page}.html`
2. Add handler in `internal/handler/web/handlers.go`
3. Add route in `internal/handler/web/routes.go`

### Add a New Log Type

1. Add model struct in `internal/models/logs.go`
2. Add table and queries in `internal/repository/log_repo.go`
3. Add service methods in `internal/service/log_service.go`
4. Update API handler in `internal/handler/api/log_handler.go`
5. Update template in `templates/daily_logs.html`

---

## Key Gotchas

### 1. Go Doesn't Hot Reload
Always rebuild and restart after Go code changes:
```bash
pkill -f carecompanion && go build -o carecompanion ./cmd/server && ./carecompanion
```

### 2. PostgreSQL DATE vs Go time.Time
When querying DATE columns, format dates as strings:
```go
// WRONG - timezone issues
rows, err := db.Query(query, childID, startDate, endDate)

// RIGHT - works correctly
startStr := startDate.Format("2006-01-02")
endStr := endDate.Format("2006-01-02")
rows, err := db.Query(query, childID, startStr, endStr)
```

### 3. Template Variable Scope
Inside `{{range}}` loops, use `$` for root context:
```html
{{range .Items}}
    {{if eq $.ViewMode "weekly"}}  <!-- $ accesses root -->
        ...
    {{end}}
{{end}}
```

### 4. LogDate vs CreatedAt
- `LogDate` = The date the entry is FOR (user-selected)
- `CreatedAt` = When the entry was SAVED
- Display `LogDate` for dates, `CreatedAt` for times

---

## Current Branch Status

- **Branch**: master
- **Status**: Ready for initial commit with all alpha features

### Recent Work (January 2026)
1. Weekly view for daily logs (Monday-Sunday)
2. Family member CRUD operations
3. Sleep entry option adjustments
4. Date query fixes for timezone handling

---

## Pending Work / Next Steps

### Immediate (AWS Deployment)
- [ ] Create GitHub repository
- [ ] Push code to GitHub
- [ ] Deploy to AWS (see `AWS-DEPLOYMENT-GUIDE.md`)
- [ ] Set up domain and SSL
- [ ] Configure CI/CD pipeline

### Future Enhancements
- [ ] WebSocket for real-time chat
- [ ] Background job processing for alerts
- [ ] S3 integration for file uploads
- [ ] Rate limiting on API endpoints
- [ ] Automated testing suite

---

## Important Files by Purpose

### Configuration
- `internal/config/config.go` - Environment variable loading

### Authentication
- `internal/middleware/auth.go` - JWT validation middleware
- `internal/service/auth_service.go` - Login/register logic
- `internal/handler/api/auth_handler.go` - Auth API endpoints

### Core Data
- `internal/models/` - All data structures
- `internal/repository/` - All database queries
- `internal/service/` - All business logic

### Web UI
- `templates/` - HTML templates
- `internal/handler/web/handlers.go` - Page handlers
- `internal/handler/web/routes.go` - Web routes

### API
- `internal/handler/api/routes.go` - API route definitions
- `internal/handler/api/*.go` - API handlers

---

## Environment Variables

Create `.env` file (not committed):
```bash
DB_HOST=localhost
DB_PORT=5432
DB_USER=carecompanion
DB_PASSWORD=carecompanion
DB_NAME=carecompanion
REDIS_HOST=localhost
REDIS_PORT=6379
JWT_SECRET=your-secret-key-here
PORT=8090
ENVIRONMENT=development
```

---

## Database Access

### Connect to Database
```bash
psql -h localhost -U carecompanion -d carecompanion
```

### Useful Queries
```sql
-- See all tables
\dt

-- Check recent logs
SELECT * FROM behavior_logs ORDER BY created_at DESC LIMIT 10;

-- Check specific child's logs
SELECT * FROM behavior_logs
WHERE child_id = 'dddd0002-0002-0002-0002-000000000002';

-- Check alerts
SELECT * FROM alerts WHERE status = 'active';
```

---

## Getting Help

1. Read `docs/DEVELOPMENT-HISTORY.md` for past problems and solutions
2. Read `docs/PROJECT-STATUS.md` for current implementation status
3. Check server logs at `/tmp/server.log`
4. Use Claude Code to explore the codebase

---

## Quick Debugging

### Server Won't Start
1. Check PostgreSQL is running: `pg_isready`
2. Check Redis is running: `redis-cli ping`
3. Check environment variables are set
4. Check for port conflicts: `lsof -i :8090`

### Page Not Loading
1. Check server logs: `tail -f /tmp/server.log`
2. Verify route exists in `routes.go`
3. Check template exists and has no syntax errors

### API Returns Error
1. Check request method and path
2. Check authentication (JWT token in cookie)
3. Check server logs for stack trace
4. Verify request body format (JSON)

---

## Commit Convention

```
feat: Add new feature
fix: Bug fix
docs: Documentation changes
refactor: Code refactoring
test: Add or update tests
chore: Maintenance tasks
```

Example:
```bash
git commit -m "feat: Add weekly view for daily logs"
```
