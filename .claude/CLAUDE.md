# CareCompanion Project

## Overview
CareCompanion is a Go web application for tracking and managing care for children with autism. It includes a public-facing website and an admin portal.

## Tech Stack
- **Backend:** Go 1.24 with Chi router
- **Database:** PostgreSQL (AWS RDS for prod, Docker container for dev)
- **Cache:** Redis (AWS ElastiCache for prod, Docker container for dev)
- **Frontend:** Server-side rendered HTML with Tailwind CSS
- **Infrastructure:** AWS (EC2 Auto Scaling, ALB, RDS, ElastiCache, ECR)
- **Deployment:** Docker containers via ECR and ASG instance refresh

## Key Directories
- `cmd/server/main.go` - Application entry point
- `internal/handler/admin/` - Admin portal handlers
- `internal/handler/api/` - Public API handlers
- `internal/service/` - Business logic layer
- `internal/repository/` - Database access layer
- `internal/models/` - Data models
- `templates/admin/` - Admin portal HTML templates
- `migrations/` - SQL migration files (00001-00014)
- `scripts/` - Dev and deploy scripts

## Environment Setup
```bash
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
```

---

## ⚠ CRITICAL: Dev vs Production Environment Rules

**This server runs BOTH the dev environment and triggers production deployments.**
**You MUST follow these rules to avoid impacting production.**

### DEFAULT ENVIRONMENT: Development
- All code changes, testing, and iteration happen in the **dev environment**
- Dev runs on `localhost:8090` with local Postgres and Redis (Docker containers)
- Start dev: `./scripts/dev.sh`
- Stop dev: `./scripts/dev-stop.sh`

### NEVER do these without explicit user confirmation:
1. **NEVER** run `sudo docker build` — this builds a production image
2. **NEVER** run `sudo docker push` — this pushes to the production ECR registry
3. **NEVER** run `aws autoscaling start-instance-refresh` — this replaces production servers
4. **NEVER** run SQL directly against the production RDS database
5. **NEVER** run `scripts/deploy.sh` without the user explicitly asking to deploy

### When the user asks to deploy to production:
- Always use `./scripts/deploy.sh` which has built-in confirmation prompts
- NEVER run docker build/push/ASG commands directly — always go through deploy.sh

### Database rules:
- **Dev database**: `localhost:5432` (Docker container, password: `carecompanion`)
- **Prod database**: `carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com`
- When running migrations during development, run them against **localhost** only
- Only run migrations against production RDS when the user explicitly asks to deploy or migrate production

### How to tell which environment you're affecting:
- If the host is `localhost` or `127.0.0.1` → dev (safe)
- If the host contains `amazonaws.com` or `mycarecompanion.net` → **production (requires user confirmation)**
- If the command is `docker build`, `docker push`, or `aws autoscaling` → **production (requires user confirmation)**

---

## Dev Environment

### Starting dev
```bash
./scripts/dev.sh    # Starts Postgres, Redis, and app with hot-reload
```
Access at: http://98.88.131.147:8090

### Stopping dev
```bash
./scripts/dev-stop.sh    # Stops Postgres and Redis containers
```

### Dev database access
```bash
psql -h localhost -U carecompanion -d carecompanion    # password: carecompanion
```

### Running migrations in dev
```bash
PGPASSWORD="carecompanion" psql -h localhost -U carecompanion -d carecompanion -f migrations/00011_family_billing.sql
```

## Production Deployment

### Deploy to production (requires confirmation at each step)
```bash
./scripts/deploy.sh
```

### Monitor deployment
```bash
aws elbv2 describe-target-health --region us-east-1 --target-group-arn arn:aws:elasticloadbalancing:us-east-1:943431294725:targetgroup/carecompanion-tg/bade3e56ae036ce7 --output table
```

### Production database (use only when explicitly requested)
Host: carecompanion-db.cns7qg5iujxu.us-east-1.rds.amazonaws.com
Port: 5432
Database: carecompanion
User: carecompanion

---

## Important Patterns

### Adding Admin Pages
When adding new admin pages, the template data struct MUST include:
- `Title string`
- `CurrentUser AdminUser`
- `Flash string` (can be empty but must exist)

### Authentication
Use `middleware.GetAuthClaims(r.Context())` to get the authenticated user.

### Template Location
Admin templates are in `templates/admin/`. The layout is `layout.html` and content templates define `{{define "content"}}`.

## URLs
- **Production:** https://www.mycarecompanion.net
- **Dev:** http://98.88.131.147:8090
- **Admin Portal:** https://www.mycarecompanion.net/admin
- **Health Check:** https://www.mycarecompanion.net/health

## Documentation
Additional documentation files are in `/home/carecomp/carecompanion/docs/`
