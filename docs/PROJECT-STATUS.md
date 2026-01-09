# CareCompanion Project Status

**Last Updated**: January 9, 2026
**Version**: Alpha (Pre-release)
**Status**: Development Complete, Ready for AWS Deployment

---

## Overview

CareCompanion is a healthcare monitoring application designed to help caregivers track children's health conditions, medications, behaviors, and receive intelligent alerts about potential issues. The application uses AI-driven correlation analysis to identify patterns and provide actionable insights.

---

## Technology Stack

| Component | Technology | Version |
|-----------|------------|---------|
| **Backend** | Go | 1.21+ |
| **Web Framework** | Chi Router | v5 |
| **Database** | PostgreSQL | 16 |
| **Cache** | Redis | 7.x |
| **Templates** | Go html/template | stdlib |
| **CSS** | Tailwind CSS | CDN |
| **External APIs** | OpenFDA Drug API | v2 |

---

## Application Architecture

```
carecompanion/
├── cmd/
│   └── server/main.go          # Application entry point
├── internal/
│   ├── config/                 # Configuration management
│   ├── database/               # PostgreSQL & Redis connections
│   ├── handler/
│   │   ├── api/               # REST API handlers
│   │   └── web/               # Web page handlers
│   ├── middleware/            # Auth, logging, CORS, error handling
│   ├── models/                # Data structures
│   ├── repository/            # Database access layer
│   └── service/               # Business logic layer
├── templates/                  # HTML templates
├── static/                     # Static assets (JS, CSS)
└── migrations/                 # SQL migrations
```

---

## Feature Implementation Status

### Core Features (Complete)

#### User Management
- [x] User registration with email verification
- [x] Login/logout with JWT authentication
- [x] Password hashing with bcrypt
- [x] User preferences (timezone, notifications)
- [x] Session management via Redis

#### Family Management
- [x] Create family groups
- [x] Invite family members
- [x] Family member role management (admin, caregiver)
- [x] Switch between families (multi-family support)
- [x] Family member CRUD operations in settings page

#### Child Profiles
- [x] Create child profiles with medical conditions
- [x] Manage child conditions (ADHD, ASD, etc.)
- [x] Child dashboard with summary stats
- [x] Switch between children in family

#### Daily Logs
- [x] Behavior logs (mood, focus, hyperactivity, etc.)
- [x] Sleep logs with quality tracking
- [x] Meal logs with nutrition tracking
- [x] Symptom logs
- [x] School logs (teacher notes)
- [x] Bowel movement logs
- [x] Appointment logs
- [x] Social logs
- [x] Event logs (general activities)
- [x] **Daily view** - single day entries
- [x] **Weekly view** - Monday 00:00 to Sunday 23:59 (NEW)
- [x] Date picker navigation
- [x] Timezone-aware date handling

#### Medications
- [x] Medication tracking with schedules
- [x] Medication logging (taken, skipped, delayed)
- [x] OpenFDA drug information integration
- [x] Medication reference search
- [x] Medication reminders
- [x] Active/inactive medication status

#### Alerts System
- [x] Intelligent alert generation
- [x] Alert severity levels (critical, warning, info)
- [x] Alert confidence scoring
- [x] Alert dismissal with reasons
- [x] Alert analysis pages
- [x] Active/resolved alert filtering

#### Insights & Correlations
- [x] Pattern detection in logged data
- [x] Medication-behavior correlations
- [x] Sleep-behavior correlations
- [x] Cohort comparison (anonymized)
- [x] Insights page with visualizations

#### Chat/Communication
- [x] Family chat threads
- [x] Image attachments in chat
- [x] Child-specific discussion threads
- [x] Message read receipts

#### Transparency Features
- [x] Alert methodology explanations
- [x] Confidence factor breakdowns
- [x] Data source transparency
- [x] Algorithm explainability

### UI/UX Features (Complete)
- [x] Responsive design (mobile-friendly)
- [x] Dark mode support
- [x] Dashboard overview
- [x] Quick summary widgets
- [x] Navigation between sections

---

## Database Schema

### Core Tables
| Table | Purpose | Status |
|-------|---------|--------|
| `users` | User accounts | Complete |
| `families` | Family groups | Complete |
| `family_members` | User-family relationships | Complete |
| `children` | Child profiles | Complete |
| `child_conditions` | Medical conditions | Complete |

### Logging Tables
| Table | Purpose | Status |
|-------|---------|--------|
| `behavior_logs` | Behavior observations | Complete |
| `sleep_logs` | Sleep tracking | Complete |
| `meal_logs` | Meal/nutrition tracking | Complete |
| `symptom_logs` | Symptom tracking | Complete |
| `school_logs` | School reports | Complete |
| `bowel_logs` | Bowel movement tracking | Complete |
| `appointment_logs` | Medical appointments | Complete |
| `social_logs` | Social interactions | Complete |
| `event_logs` | General activities | Complete |

### Medication Tables
| Table | Purpose | Status |
|-------|---------|--------|
| `medications` | Medication definitions | Complete |
| `medication_logs` | Dose tracking | Complete |
| `medication_references` | Drug database cache | Complete |

### Alert & Insight Tables
| Table | Purpose | Status |
|-------|---------|--------|
| `alerts` | Generated alerts | Complete |
| `alert_confidence_factors` | Confidence breakdown | Complete |
| `correlations` | Pattern correlations | Complete |
| `insights` | Generated insights | Complete |

### Communication Tables
| Table | Purpose | Status |
|-------|---------|--------|
| `chat_threads` | Chat conversations | Complete |
| `chat_messages` | Individual messages | Complete |
| `chat_files` | File attachments | Complete |

---

## API Endpoints

### Authentication
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/login` | User login |
| POST | `/api/auth/register` | User registration |
| POST | `/api/auth/logout` | User logout |
| POST | `/api/auth/refresh` | Refresh JWT token |

### Children
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/children` | List family children |
| POST | `/api/children` | Create child |
| GET | `/api/children/{id}` | Get child details |
| PUT | `/api/children/{id}` | Update child |
| DELETE | `/api/children/{id}` | Delete child |
| GET | `/api/children/{id}/logs/quick-summary` | Quick summary |

### Logs
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/children/{id}/logs` | Get daily logs |
| POST | `/api/children/{id}/logs/{type}` | Create log entry |
| PUT | `/api/children/{id}/logs/{type}/{logId}` | Update log |
| DELETE | `/api/children/{id}/logs/{type}/{logId}` | Delete log |
| GET | `/api/children/{id}/logs/dates` | Get dates with logs |

### Medications
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/children/{id}/medications` | List medications |
| POST | `/api/children/{id}/medications` | Add medication |
| PUT | `/api/children/{id}/medications/{medId}` | Update medication |
| DELETE | `/api/children/{id}/medications/{medId}` | Remove medication |
| GET | `/api/medication-references` | Search drug database |
| GET | `/api/drugs/info` | Get drug details |

### Alerts
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/children/{id}/alerts` | List alerts |
| GET | `/api/alerts/{id}` | Get alert details |
| PUT | `/api/alerts/{id}/dismiss` | Dismiss alert |
| GET | `/api/alerts/{id}/analysis` | Get full analysis |
| GET | `/api/alerts/{id}/confidence-factors` | Get confidence breakdown |

### Family
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/family` | Get family details |
| POST | `/api/family` | Create family |
| GET | `/api/family/members` | List members |
| POST | `/api/family/members` | Add member |
| PUT | `/api/family/members/{id}` | Update member |
| DELETE | `/api/family/members/{id}` | Remove member |

### Chat
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/chat/threads` | List threads |
| POST | `/api/chat/threads` | Create thread |
| GET | `/api/chat/threads/{id}` | Get thread messages |
| POST | `/api/chat/threads/{id}/messages` | Send message |
| GET | `/api/chat/files/{filename}` | Get file attachment |

---

## Web Pages

| Route | Template | Description |
|-------|----------|-------------|
| `/` | - | Redirect to dashboard |
| `/login` | login.html | Login page |
| `/register` | register.html | Registration page |
| `/dashboard` | dashboard.html | Family dashboard |
| `/child/{id}` | child_dashboard.html | Child dashboard |
| `/child/{id}/logs` | daily_logs.html | Daily/weekly logs |
| `/child/{id}/medications` | medications.html | Medication management |
| `/child/{id}/alerts` | alerts.html | Alert list |
| `/child/{id}/alert/{alertId}/analysis` | alert_analysis.html | Alert detail |
| `/child/{id}/insights` | insights.html | Insights page |
| `/child/{id}/settings` | child_settings.html | Child settings |
| `/child/new` | new_child.html | Add child form |
| `/family/new` | new_family.html | Create family form |
| `/settings` | settings.html | User & family settings |
| `/chat` | chat.html | Family chat |

---

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `DB_HOST` | PostgreSQL host | Yes |
| `DB_PORT` | PostgreSQL port | Yes |
| `DB_USER` | Database user | Yes |
| `DB_PASSWORD` | Database password | Yes |
| `DB_NAME` | Database name | Yes |
| `REDIS_HOST` | Redis host | Yes |
| `REDIS_PORT` | Redis port | Yes |
| `REDIS_PASSWORD` | Redis password | No |
| `JWT_SECRET` | JWT signing key | Yes |
| `PORT` | Server port (default: 8090) | No |
| `ENVIRONMENT` | development/production | No |

---

## Known Limitations

1. **Single timezone per user**: Timezone is set at user level, not per-child
2. **No offline support**: Requires constant internet connection
3. **File uploads**: Currently stored locally, needs S3 migration for production
4. **No real-time updates**: Chat requires page refresh (no WebSocket)
5. **Alert generation**: Runs on log creation, not scheduled background jobs

---

## Test Data

Test data is available for development:
- **Family ID**: `aaaa0001-0001-0001-0001-000000000001`
- **User**: testparent@example.com / password123
- **Children**: Multiple test children with varied log data

See `testdata.md` (not committed) for full test data details.

---

## Recent Changes (January 2026)

1. **Weekly View for Daily Logs**
   - Added `ViewMode` and `EndDate` to DailyLogPage model
   - Implemented `GetLogsForDateRange()` repository method
   - Added `GetWeekBounds()` service method (Monday-Sunday)
   - Updated template to show "This Week's Entries" header
   - Fixed timezone issues in date range queries

2. **Family Member CRUD**
   - Full CRUD operations for family members in settings page
   - Role management (admin, caregiver)
   - Member removal with confirmation

3. **Sleep Entry Options**
   - Adjusted selectable options for sleep quality and duration

---

## Next Steps

1. **AWS Deployment** - See `AWS-DEPLOYMENT-GUIDE.md`
2. **CI/CD Pipeline** - GitHub Actions workflow
3. **Monitoring** - CloudWatch integration
4. **Performance** - Add indexes for common queries
5. **Real-time** - Consider WebSocket for chat

---

## File Counts

```
Go Files:        78 files
Templates:       18 files
Static Assets:   5 files
Migrations:      1 file
```
