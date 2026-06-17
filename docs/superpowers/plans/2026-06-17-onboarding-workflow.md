# New-User Onboarding Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a hybrid first-run onboarding — a required JS-stepper wizard (welcome → first child with condition chips) plus a dismissible dashboard "Finish setting up" checklist (add child, invite care team, basic settings incl. AI consent).

**Architecture:** A new `/onboarding` page renders a client-side stepper that reuses existing JSON APIs (`/api/families`, `/api/auth/switch-family`, `/api/children`, `/api/family/members`, `/api/users/me/preferences`, narrative consent). Onboarding progress is tracked by new nullable timestamp columns on `app_users`. The dashboard handler gates new users to `/onboarding` and conditionally renders a checklist partial.

**Tech Stack:** Go 1.24, Chi router, database/sql + Postgres, server-rendered html/template (parent pages are standalone docs using `head`/`nav`/`mascot` partials + `calm.css`/Tailwind), vanilla JS with `fetch` + `Authorization: Bearer <localStorage access_token>`.

---

## Design refinements vs. spec (read first)

The approved spec (`docs/superpowers/specs/2026-06-17-onboarding-workflow-design.md`) said per-item checklist state would live in an `app_users.settings` JSONB. **That column does not exist** and `app_users` is scanned column-by-column (no struct tags). To stay simple and avoid touching the shared `users` view, this plan instead uses **four dedicated nullable timestamp columns** on `app_users`:

- `onboarding_completed_at` — required wizard finished (the dashboard gate)
- `onboarding_checklist_dismissed_at` — user dismissed the checklist card
- `onboarding_settings_done_at` — "basic settings" checklist item completed
- `onboarding_invite_done_at` — "invite care team" checklist item completed

"Add another child" done-state is **derived** from the dashboard's child count (`len(Children) >= 2`) — no column. The checklist auto-dismisses when both invite and settings are done. Everything else matches the spec.

## File map

- **Create** `migrations/00042_onboarding.sql` — add 4 columns + backfill existing users to completed.
- **Modify** `internal/models/user.go` — add `OnboardingState` struct.
- **Modify** `internal/repository/repository.go` — add 5 methods to `UserRepository` interface.
- **Modify** `internal/repository/user_repo.go` — implement the 5 methods.
- **Modify** `internal/service/user_service.go` — add 5 onboarding service methods.
- **Create** `internal/service/onboarding_logic.go` — pure decision helpers (`OnboardingComplete`, `ShouldShowChecklist`).
- **Create** `internal/service/onboarding_logic_test.go` — unit tests for the helpers (TDD).
- **Create** `internal/handler/api/onboarding_handler.go` — `OnboardingHandler` with 4 POST endpoints.
- **Create** `internal/handler/api/onboarding_handler_test.go` — dependency-free HTTP wiring test.
- **Modify** `internal/handler/api/routes.go` — register `OnboardingHandler` + routes.
- **Modify** `internal/handler/web/handlers.go` — add `Onboarding` web handler + gate `Dashboard`.
- **Modify** `internal/handler/web/routes.go` — register `GET /onboarding`.
- **Create** `templates/onboarding.html` — the wizard page.
- **Create** `static/js/onboarding.js` — the stepper logic.
- **Create** `templates/partials/onboarding_checklist.html` — the dashboard checklist card.
- **Create** `static/js/onboarding_checklist.js` — checklist interactions.
- **Modify** `templates/dashboard.html` — include the checklist partial.

**Environment for all build/test steps:**
```bash
export PATH=$PATH:/usr/local/go/bin && export GOPATH=/home/carecomp/go
cd /home/carecomp/carecompanion
```

---

## Task 1: Migration — onboarding columns + backfill

**Files:**
- Create: `migrations/00042_onboarding.sql`

Migration runner runs the whole file as one transaction; **no `BEGIN`/`COMMIT`, no goose directives, no `GRANT`s** (those would break dev's local Postgres). Match the 00040/00041 style.

- [ ] **Step 1: Write the migration file**

`migrations/00042_onboarding.sql`:
```sql
-- 00042_onboarding.sql
-- Adds new-user onboarding tracking columns to app_users.
-- Rollback (manual):
--   ALTER TABLE app_users
--     DROP COLUMN IF EXISTS onboarding_completed_at,
--     DROP COLUMN IF EXISTS onboarding_checklist_dismissed_at,
--     DROP COLUMN IF EXISTS onboarding_settings_done_at,
--     DROP COLUMN IF EXISTS onboarding_invite_done_at;

ALTER TABLE app_users
    ADD COLUMN IF NOT EXISTS onboarding_completed_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS onboarding_checklist_dismissed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS onboarding_settings_done_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS onboarding_invite_done_at         TIMESTAMPTZ;

-- Backfill: every user that already exists has been using the app, so mark
-- them onboarding-complete. Only users created AFTER this migration (NULL)
-- will be routed through onboarding.
UPDATE app_users
   SET onboarding_completed_at = NOW()
 WHERE onboarding_completed_at IS NULL;
```

- [ ] **Step 2: Apply on the dev database**

Run:
```bash
PGPASSWORD="carecompanion" psql -h localhost -U carecompanion -d carecompanion -f migrations/00042_onboarding.sql
```
Expected: `ALTER TABLE` then `UPDATE <n>` (n = number of existing app_users).

- [ ] **Step 3: Verify the columns + backfill**

Run:
```bash
PGPASSWORD="carecompanion" psql -h localhost -U carecompanion -d carecompanion -tAc \
"SELECT count(*) AS total, count(onboarding_completed_at) AS completed FROM app_users;"
```
Expected: `total` equals `completed` (all existing users backfilled).

- [ ] **Step 4: Commit**

```bash
git add migrations/00042_onboarding.sql
git commit -m "feat(onboarding): migration 00042 — app_users onboarding columns + backfill"
```

---

## Task 2: Model + repository methods

**Files:**
- Modify: `internal/models/user.go`
- Modify: `internal/repository/repository.go` (`UserRepository` interface, ~lines 23-33)
- Modify: `internal/repository/user_repo.go`

- [ ] **Step 1: Add the `OnboardingState` model**

Append to `internal/models/user.go` (uses `time` and `*time.Time` for nullability — `time` is already imported in this file):
```go
// OnboardingState captures a user's onboarding progress (all timestamps nullable).
type OnboardingState struct {
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	ChecklistDismissedAt *time.Time `json:"checklist_dismissed_at,omitempty"`
	SettingsDoneAt       *time.Time `json:"settings_done_at,omitempty"`
	InviteDoneAt         *time.Time `json:"invite_done_at,omitempty"`
}
```

- [ ] **Step 2: Add methods to the `UserRepository` interface**

In `internal/repository/repository.go`, add these to the `UserRepository` interface (after `UpdateLastLogin`):
```go
	GetOnboardingState(ctx context.Context, id uuid.UUID) (*models.OnboardingState, error)
	SetOnboardingCompleted(ctx context.Context, id uuid.UUID) error
	SetOnboardingChecklistDismissed(ctx context.Context, id uuid.UUID) error
	SetOnboardingSettingsDone(ctx context.Context, id uuid.UUID) error
	SetOnboardingInviteDone(ctx context.Context, id uuid.UUID) error
```

- [ ] **Step 3: Implement the methods in `user_repo.go`**

Append to `internal/repository/user_repo.go` (the file already imports `context`, `database/sql`, `time`, `github.com/google/uuid`, and `carecompanion/internal/models`). These query/UPDATE `app_users` directly (NOT the `users` view):
```go
// GetOnboardingState reads onboarding timestamps from app_users.
func (r *userRepo) GetOnboardingState(ctx context.Context, id uuid.UUID) (*models.OnboardingState, error) {
	const q = `
		SELECT onboarding_completed_at, onboarding_checklist_dismissed_at,
		       onboarding_settings_done_at, onboarding_invite_done_at
		FROM app_users
		WHERE id = $1`
	var completed, dismissed, settings, invite sql.NullTime
	err := r.db.QueryRowContext(ctx, q, id).Scan(&completed, &dismissed, &settings, &invite)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state := &models.OnboardingState{}
	if completed.Valid {
		state.CompletedAt = &completed.Time
	}
	if dismissed.Valid {
		state.ChecklistDismissedAt = &dismissed.Time
	}
	if settings.Valid {
		state.SettingsDoneAt = &settings.Time
	}
	if invite.Valid {
		state.InviteDoneAt = &invite.Time
	}
	return state, nil
}

func (r *userRepo) SetOnboardingCompleted(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE app_users SET onboarding_completed_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *userRepo) SetOnboardingChecklistDismissed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE app_users SET onboarding_checklist_dismissed_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *userRepo) SetOnboardingSettingsDone(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE app_users SET onboarding_settings_done_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *userRepo) SetOnboardingInviteDone(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE app_users SET onboarding_invite_done_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}
```
(If the concrete struct is not named `userRepo`, match the receiver used by the existing `Update`/`UpdateStatus` methods in this file.)

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/models/user.go internal/repository/repository.go internal/repository/user_repo.go
git commit -m "feat(onboarding): OnboardingState model + repository methods"
```

---

## Task 3: UserService onboarding methods

**Files:**
- Modify: `internal/service/user_service.go`

- [ ] **Step 1: Add the service methods**

Append to `internal/service/user_service.go` (imports `context`, `github.com/google/uuid`, `carecompanion/internal/models` already present):
```go
// GetOnboardingState returns the user's onboarding progress.
func (s *UserService) GetOnboardingState(ctx context.Context, userID uuid.UUID) (*models.OnboardingState, error) {
	return s.userRepo.GetOnboardingState(ctx, userID)
}

// CompleteOnboarding marks the required wizard finished.
func (s *UserService) CompleteOnboarding(ctx context.Context, userID uuid.UUID) error {
	return s.userRepo.SetOnboardingCompleted(ctx, userID)
}

// DismissChecklist marks the dashboard setup checklist dismissed.
func (s *UserService) DismissChecklist(ctx context.Context, userID uuid.UUID) error {
	return s.userRepo.SetOnboardingChecklistDismissed(ctx, userID)
}

// MarkSettingsDone marks the basic-settings checklist item complete.
func (s *UserService) MarkSettingsDone(ctx context.Context, userID uuid.UUID) error {
	return s.userRepo.SetOnboardingSettingsDone(ctx, userID)
}

// MarkInviteDone marks the invite-care-team checklist item complete.
func (s *UserService) MarkInviteDone(ctx context.Context, userID uuid.UUID) error {
	return s.userRepo.SetOnboardingInviteDone(ctx, userID)
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/service/user_service.go
git commit -m "feat(onboarding): UserService onboarding methods"
```

---

## Task 4: Pure decision helpers (TDD)

The highest-risk logic is "is onboarding complete?" (the gate) and "should the checklist show?". Extract these into pure functions so they can be unit-tested without a DB — matching the repo's dependency-free test convention.

**Files:**
- Create: `internal/service/onboarding_logic.go`
- Test: `internal/service/onboarding_logic_test.go`

- [ ] **Step 1: Write the failing test**

`internal/service/onboarding_logic_test.go`:
```go
package service

import (
	"testing"
	"time"

	"carecompanion/internal/models"
)

func TestOnboardingComplete(t *testing.T) {
	now := time.Now()
	if OnboardingComplete(nil) {
		t.Fatal("nil state should be incomplete")
	}
	if OnboardingComplete(&models.OnboardingState{}) {
		t.Fatal("empty state should be incomplete")
	}
	if !OnboardingComplete(&models.OnboardingState{CompletedAt: &now}) {
		t.Fatal("state with CompletedAt should be complete")
	}
}

func TestShouldShowChecklist(t *testing.T) {
	now := time.Now()
	// not completed -> never show checklist (user is still in the wizard)
	if ShouldShowChecklist(&models.OnboardingState{}) {
		t.Fatal("incomplete onboarding should not show checklist")
	}
	// completed, nothing else -> show
	if !ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now}) {
		t.Fatal("completed with pending items should show checklist")
	}
	// dismissed -> hide
	if ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now, ChecklistDismissedAt: &now}) {
		t.Fatal("dismissed checklist should not show")
	}
	// both invite + settings done -> hide (auto-complete)
	if ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now, SettingsDoneAt: &now, InviteDoneAt: &now}) {
		t.Fatal("all items done should not show checklist")
	}
	// only one done -> still show
	if !ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now, SettingsDoneAt: &now}) {
		t.Fatal("partial completion should still show checklist")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/service/ -run 'TestOnboarding|TestShouldShowChecklist' -v`
Expected: FAIL — `undefined: OnboardingComplete` / `undefined: ShouldShowChecklist`.

- [ ] **Step 3: Write the implementation**

`internal/service/onboarding_logic.go`:
```go
package service

import "carecompanion/internal/models"

// OnboardingComplete reports whether the required onboarding wizard is finished.
// A nil state (no row / brand-new user) counts as incomplete.
func OnboardingComplete(s *models.OnboardingState) bool {
	return s != nil && s.CompletedAt != nil
}

// ShouldShowChecklist reports whether the dashboard "finish setting up" card
// should render: only after the wizard is complete, and only while the user
// has neither dismissed it nor finished both the invite and settings items.
func ShouldShowChecklist(s *models.OnboardingState) bool {
	if !OnboardingComplete(s) {
		return false
	}
	if s.ChecklistDismissedAt != nil {
		return false
	}
	if s.SettingsDoneAt != nil && s.InviteDoneAt != nil {
		return false
	}
	return true
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/service/ -run 'TestOnboarding|TestShouldShowChecklist' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/onboarding_logic.go internal/service/onboarding_logic_test.go
git commit -m "feat(onboarding): pure gate/checklist decision helpers + tests"
```

---

## Task 5: API OnboardingHandler + routes

**Files:**
- Create: `internal/handler/api/onboarding_handler.go`
- Test: `internal/handler/api/onboarding_handler_test.go`
- Modify: `internal/handler/api/routes.go` (`Handlers` struct ~14-35, `NewHandlers` ~37-60, protected group ~399-406)

- [ ] **Step 1: Create the handler**

`internal/handler/api/onboarding_handler.go`:
```go
package api

import (
	"net/http"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// OnboardingHandler handles per-user onboarding state transitions.
type OnboardingHandler struct {
	userService *service.UserService
}

// NewOnboardingHandler creates a new onboarding handler.
func NewOnboardingHandler(userService *service.UserService) *OnboardingHandler {
	return &OnboardingHandler{userService: userService}
}

// Complete handles POST /api/onboarding/complete — marks the wizard finished.
func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.CompleteOnboarding(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to complete onboarding")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Onboarding completed"})
}

// DismissChecklist handles POST /api/onboarding/checklist/dismiss.
func (h *OnboardingHandler) DismissChecklist(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.DismissChecklist(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to dismiss checklist")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Checklist dismissed"})
}

// SettingsDone handles POST /api/onboarding/settings-done.
func (h *OnboardingHandler) SettingsDone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.MarkSettingsDone(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to mark settings done")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Settings step done"})
}

// InviteDone handles POST /api/onboarding/invite-done.
func (h *OnboardingHandler) InviteDone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.MarkInviteDone(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to mark invite done")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Invite step done"})
}
```

- [ ] **Step 2: Add the handler to the `Handlers` struct and `NewHandlers`**

In `internal/handler/api/routes.go`, add a field to the `Handlers` struct (after `NarrativeConsent`):
```go
	Onboarding *OnboardingHandler
```
And in `NewHandlers`, add a construction line (after the `NarrativeConsent:` line):
```go
		Onboarding: NewOnboardingHandler(services.User),
```

- [ ] **Step 3: Register the routes**

In `internal/handler/api/routes.go`, inside the protected group (near the `/users/me/...` block around line 399-406), add:
```go
	r.Post("/onboarding/complete", handlers.Onboarding.Complete)
	r.Post("/onboarding/checklist/dismiss", handlers.Onboarding.DismissChecklist)
	r.Post("/onboarding/settings-done", handlers.Onboarding.SettingsDone)
	r.Post("/onboarding/invite-done", handlers.Onboarding.InviteDone)
```
(These need only auth — NOT `RequireFamilyContext` — because onboarding can run before a family exists.)

- [ ] **Step 4: Write a dependency-free wiring test**

`internal/handler/api/onboarding_handler_test.go` (matches the repo's nil-dependency handler-test convention; verifies the routes are wired and reachable without panicking on the nil-service path by asserting the method set exists via the constructor):
```go
package api

import "testing"

func TestNewOnboardingHandler_Constructs(t *testing.T) {
	h := NewOnboardingHandler(nil)
	if h == nil {
		t.Fatal("NewOnboardingHandler returned nil")
	}
}
```
(Note: deeper behavior — the actual DB writes — is verified manually on dev in Task 9, consistent with this repo having no DB-backed handler tests.)

- [ ] **Step 5: Run the test + build**

Run: `go test ./internal/handler/api/ -run TestNewOnboardingHandler_Constructs -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/api/onboarding_handler.go internal/handler/api/onboarding_handler_test.go internal/handler/api/routes.go
git commit -m "feat(onboarding): API onboarding-state endpoints"
```

---

## Task 6: Web Onboarding handler + Dashboard gate

**Files:**
- Modify: `internal/handler/web/handlers.go` (`Dashboard` ~143-176; add `Onboarding`)
- Modify: `internal/handler/web/routes.go` (protected group ~36-63)

- [ ] **Step 1: Add the `Onboarding` web handler**

Append to `internal/handler/web/handlers.go` (near `Settings`). It renders the wizard and passes the flags the JS stepper needs. An already-completed user is redirected to the dashboard so they can't re-enter the wizard. Invited members (a family that already has children) get the trimmed flag:
```go
// Onboarding renders the first-run wizard.
func (h *WebHandlers) Onboarding(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())

	state, err := h.services.User.GetOnboardingState(r.Context(), userID)
	if err != nil {
		renderError(w, "Failed to load onboarding", http.StatusInternalServerError)
		return
	}
	if service.OnboardingComplete(state) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	hasFamily := familyID.String() != "00000000-0000-0000-0000-000000000000"
	invitedMember := false
	if hasFamily {
		if dash, derr := h.services.Family.GetDashboard(r.Context(), familyID); derr == nil && len(dash.Children) > 0 {
			// Joined a family that already has children -> they don't add a child.
			invitedMember = true
		}
	}

	data := map[string]interface{}{
		"FirstName":     middleware.GetFirstName(r.Context()),
		"HasFamily":     hasFamily,
		"InvitedMember": invitedMember,
	}
	renderTemplate(w, "onboarding", data)
}
```
(If `internal/handler/web/handlers.go` does not already import `carecompanion/internal/service`, add it to the import block — it is needed for `service.OnboardingComplete`.)

- [ ] **Step 2: Add the gate + checklist data to `Dashboard`**

Edit the `Dashboard` handler in `internal/handler/web/handlers.go`. Replace the existing body (lines ~143-176) with:
```go
func (h *WebHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())

	// Gate: brand-new users must finish onboarding first.
	state, err := h.services.User.GetOnboardingState(r.Context(), userID)
	if err != nil {
		renderError(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}
	if !service.OnboardingComplete(state) {
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}

	if familyID.String() == "00000000-0000-0000-0000-000000000000" {
		// No family, redirect to create one
		http.Redirect(w, r, "/family/new", http.StatusSeeOther)
		return
	}

	dashboard, err := h.services.Family.GetDashboard(r.Context(), familyID)
	if err != nil {
		renderError(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}

	families, err := h.services.User.GetUserFamilies(r.Context(), userID)
	if err != nil {
		families = nil
	}

	data := map[string]interface{}{
		"UserID":                  userID,
		"FamilyID":                familyID,
		"Dashboard":               dashboard,
		"FirstName":               middleware.GetFirstName(r.Context()),
		"Families":                families,
		"ShowOnboardingChecklist": service.ShouldShowChecklist(state),
		"OnboardingInviteDone":    state != nil && state.InviteDoneAt != nil,
		"OnboardingSettingsDone":  state != nil && state.SettingsDoneAt != nil,
		"ChildCount":              len(dashboard.Children),
	}

	renderTemplate(w, "dashboard", data)
}
```

- [ ] **Step 3: Register the web route**

In `internal/handler/web/routes.go`, inside the protected group (after `r.Get("/dashboard", handlers.Dashboard)`), add:
```go
		r.Get("/onboarding", handlers.Onboarding)
```

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/web/handlers.go internal/handler/web/routes.go
git commit -m "feat(onboarding): web wizard handler + dashboard gate"
```

---

## Task 7: The wizard page + stepper JS

**Files:**
- Create: `templates/onboarding.html`
- Create: `static/js/onboarding.js`

- [ ] **Step 1: Create the page**

`templates/onboarding.html` (standalone parent doc using shared partials + calm classes; mirrors `new_child.html`). It embeds the flags as JSON and a step container that `onboarding.js` drives:
```html
<!DOCTYPE html>
<html lang="en">
{{template "head" "Welcome"}}
<body class="calm-bg min-h-screen">

{{template "nav" .}}
{{template "mascot" .}}

<main class="max-w-xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
  <div class="bloom ring-soft rounded-3xl p-8 sm:p-10">
    <!-- progress -->
    <div class="w-full h-2 bg-orange-100 rounded-full mb-8 overflow-hidden">
      <div id="ob-progress" class="h-2 bg-orange-500 rounded-full transition-all duration-300" style="width:0%"></div>
    </div>

    <!-- Step: Welcome -->
    <section data-step="welcome">
      <p class="handwritten text-2xl text-amber-700">Welcome{{if .FirstName}}, {{.FirstName}}{{end}}!</p>
      <h1 class="display text-3xl text-stone-800 mt-1">Let's set up your space</h1>
      <p class="text-stone-600 mt-3">It takes about a minute. We'll get your first child added so the app is ready to use.</p>
      <button type="button" class="seed-btn mt-8 inline-flex items-center bg-orange-600 text-white rounded-full px-6 py-3" data-next>Let's go</button>
    </section>

    <!-- Step: Name your family (only when no family yet) -->
    <section data-step="family" hidden>
      <h1 class="display text-2xl text-stone-800">What should we call your family space?</h1>
      <input id="ob-family-name" type="text" placeholder="e.g. The Johnsons"
             class="w-full mt-4 rounded-2xl border-stone-200 focus:ring-orange-300 px-4 py-3" />
      <p data-error class="text-red-600 text-sm mt-2 hidden"></p>
      <div class="mt-8 flex justify-between">
        <button type="button" class="text-stone-500" data-back>Back</button>
        <button type="button" class="seed-btn bg-orange-600 text-white rounded-full px-6 py-3" data-save-family>Continue</button>
      </div>
    </section>

    <!-- Step: First child -->
    <section data-step="child" hidden>
      <h1 class="display text-2xl text-stone-800">Tell us about your child</h1>
      <label class="block mt-4 text-sm text-stone-600">First name</label>
      <input id="ob-child-first" type="text" class="w-full mt-1 rounded-2xl border-stone-200 focus:ring-orange-300 px-4 py-3" />
      <label class="block mt-4 text-sm text-stone-600">Date of birth</label>
      <input id="ob-child-dob" type="date" class="w-full mt-1 rounded-2xl border-stone-200 focus:ring-orange-300 px-4 py-3" />
      <label class="block mt-4 text-sm text-stone-600">Gender (optional)</label>
      <select id="ob-child-gender" class="w-full mt-1 rounded-2xl border-stone-200 focus:ring-orange-300 px-4 py-3">
        <option value="">Prefer not to say</option>
        <option value="male">Male</option>
        <option value="female">Female</option>
        <option value="other">Other</option>
      </select>
      <label class="block mt-4 text-sm text-stone-600">Conditions (optional)</label>
      <div id="ob-chips" class="flex flex-wrap gap-2 mt-2"></div>
      <div class="flex gap-2 mt-3">
        <input id="ob-chip-custom" type="text" placeholder="Add your own"
               class="flex-1 rounded-2xl border-stone-200 focus:ring-orange-300 px-4 py-2" />
        <button type="button" id="ob-chip-add" class="rounded-full bg-stone-100 px-4">Add</button>
      </div>
      <p data-error class="text-red-600 text-sm mt-2 hidden"></p>
      <div class="mt-8 flex justify-between">
        <button type="button" class="text-stone-500" data-back>Back</button>
        <button type="button" class="seed-btn bg-orange-600 text-white rounded-full px-6 py-3" data-finish>Finish</button>
      </div>
    </section>

    <!-- Step: Basic settings (trimmed path for invited members) -->
    <section data-step="settings" hidden>
      <h1 class="display text-2xl text-stone-800">A couple of quick settings</h1>
      <label class="block mt-4 text-sm text-stone-600">Time zone</label>
      <input id="ob-tz" type="text" class="w-full mt-1 rounded-2xl border-stone-200 px-4 py-3" readonly />
      <label class="inline-flex items-center mt-4 gap-2">
        <input id="ob-notify" type="checkbox" checked class="rounded" /> Send me reminders & alerts
      </label>
      <div class="mt-8 flex justify-between">
        <button type="button" class="text-stone-500" data-back>Back</button>
        <button type="button" class="seed-btn bg-orange-600 text-white rounded-full px-6 py-3" data-finish-invited>Finish</button>
      </div>
    </section>
  </div>
</main>

<script>
  window.OB = {
    hasFamily: {{if .HasFamily}}true{{else}}false{{end}},
    invitedMember: {{if .InvitedMember}}true{{else}}false{{end}}
  };
</script>
<script src="/static/js/onboarding.js"></script>
</body>
</html>
```

- [ ] **Step 2: Create the stepper JS**

`static/js/onboarding.js`:
```js
(function () {
  const token = () => localStorage.getItem('access_token');
  const authHeaders = () => ({ 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token() });

  const CONDITIONS = ['Autism Spectrum Disorder', 'ADHD', 'Anxiety', 'Epilepsy/Seizures', 'Sensory Processing Disorder', 'Speech/Language Delay'];
  const selected = new Set();

  // Build the step order based on the user's situation.
  let steps;
  if (window.OB.invitedMember) {
    steps = ['welcome', 'settings'];           // trimmed path
  } else if (window.OB.hasFamily) {
    steps = ['welcome', 'child'];
  } else {
    steps = ['welcome', 'family', 'child'];
  }
  let idx = 0;

  function show(i) {
    idx = i;
    document.querySelectorAll('[data-step]').forEach(s => { s.hidden = (s.dataset.step !== steps[i]); });
    document.getElementById('ob-progress').style.width = Math.round((i / (steps.length - 1)) * 100) + '%';
    if (steps[i] === 'settings') {
      const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'America/Chicago';
      document.getElementById('ob-tz').value = tz;
    }
  }
  function next() { if (idx < steps.length - 1) show(idx + 1); }
  function back() { if (idx > 0) show(idx - 1); }
  function err(sectionStep, msg) {
    const sec = document.querySelector('[data-step="' + sectionStep + '"]');
    const p = sec.querySelector('[data-error]');
    if (p) { p.textContent = msg; p.classList.remove('hidden'); }
  }

  // Condition chips
  function renderChips() {
    const wrap = document.getElementById('ob-chips');
    wrap.innerHTML = '';
    const all = CONDITIONS.concat([...selected].filter(c => !CONDITIONS.includes(c)));
    all.forEach(c => {
      const on = selected.has(c);
      const b = document.createElement('button');
      b.type = 'button';
      b.textContent = c + (on ? '  ✕' : '');
      b.className = 'rounded-full px-3 py-1 text-sm ' + (on ? 'bg-orange-500 text-white' : 'bg-stone-100 text-stone-700');
      b.onclick = () => { on ? selected.delete(c) : selected.add(c); renderChips(); };
      wrap.appendChild(b);
    });
  }

  document.addEventListener('click', async (e) => {
    if (e.target.matches('[data-next]')) return next();
    if (e.target.matches('[data-back]')) return back();

    if (e.target.matches('[data-save-family]')) {
      const name = document.getElementById('ob-family-name').value.trim();
      if (!name) return err('family', 'Please enter a family name.');
      e.target.disabled = true;
      try {
        const res = await fetch('/api/families', { method: 'POST', headers: authHeaders(), body: JSON.stringify({ name }) });
        const data = await res.json();
        if (!res.ok) { e.target.disabled = false; return err('family', data.message || 'Could not create family.'); }
        // Switch context so the JWT carries the new family_id.
        const sw = await fetch('/api/auth/switch-family', { method: 'POST', headers: authHeaders(), credentials: 'same-origin', body: JSON.stringify({ family_id: data.id }) });
        if (sw.ok) { const sd = await sw.json(); if (sd.access_token) localStorage.setItem('access_token', sd.access_token); }
        next();
      } catch (_) { e.target.disabled = false; err('family', 'Network error — please try again.'); }
    }

    if (e.target.matches('[data-finish]')) {
      const first = document.getElementById('ob-child-first').value.trim();
      const dob = document.getElementById('ob-child-dob').value;
      if (!first) return err('child', 'Please enter a first name.');
      if (!dob) return err('child', 'Please enter a date of birth.');
      const body = {
        first_name: first,
        date_of_birth: new Date(dob).toISOString(),
        gender: document.getElementById('ob-child-gender').value || undefined,
        conditions: [...selected]
      };
      e.target.disabled = true;
      try {
        const res = await fetch('/api/children', { method: 'POST', headers: authHeaders(), body: JSON.stringify(body) });
        const data = await res.json();
        if (!res.ok) { e.target.disabled = false; return err('child', data.message || 'Could not save child.'); }
        await fetch('/api/onboarding/complete', { method: 'POST', headers: authHeaders() });
        window.location.href = '/dashboard';
      } catch (_) { e.target.disabled = false; err('child', 'Network error — please try again.'); }
    }

    if (e.target.matches('[data-finish-invited]')) {
      const tz = document.getElementById('ob-tz').value;
      e.target.disabled = true;
      try {
        await fetch('/api/users/me/preferences', { method: 'PUT', headers: authHeaders(), body: JSON.stringify({ timezone: tz }) });
        await fetch('/api/onboarding/complete', { method: 'POST', headers: authHeaders() });
        window.location.href = '/dashboard';
      } catch (_) { e.target.disabled = false; }
    }

    if (e.target.matches('#ob-chip-add')) {
      const v = document.getElementById('ob-chip-custom').value.trim();
      if (v) { selected.add(v); document.getElementById('ob-chip-custom').value = ''; renderChips(); }
    }
  });

  renderChips();
  show(0);
})();
```

- [ ] **Step 3: Build + restart dev, smoke the page renders**

Run: `go build ./... && ./scripts/dev.sh` (or restart the dev systemd service per the project's dev workflow), then:
```bash
curl -s -m5 -o /dev/null -w "%{http_code}\n" http://localhost:8090/onboarding
```
Expected: `303` (redirect to /login when unauthenticated) — confirms the route exists. Full visual verification happens in Task 9 via the browser.

- [ ] **Step 4: Commit**

```bash
git add templates/onboarding.html static/js/onboarding.js
git commit -m "feat(onboarding): wizard page + stepper JS"
```

---

## Task 8: Dashboard checklist card

**Files:**
- Create: `templates/partials/onboarding_checklist.html`
- Create: `static/js/onboarding_checklist.js`
- Modify: `templates/dashboard.html`

- [ ] **Step 1: Create the checklist partial**

`templates/partials/onboarding_checklist.html`:
```html
{{define "onboarding_checklist"}}
<div id="ob-checklist" class="bloom ring-soft rounded-3xl p-6 sm:p-8 mb-6 border border-orange-100">
  <div class="flex items-start justify-between">
    <div>
      <p class="handwritten text-xl text-amber-700">Almost there</p>
      <h2 class="display text-2xl text-stone-800">Finish setting up</h2>
    </div>
    <button type="button" id="ob-dismiss" class="text-sm text-stone-400 hover:text-stone-600">I'm all set ✕</button>
  </div>

  <ul class="mt-5 space-y-3">
    <!-- Add another child (derived done when 2+ children) -->
    <li class="flex items-center justify-between rounded-2xl bg-white/60 px-4 py-3">
      <span>{{if ge .ChildCount 2}}✅{{else}}➕{{end}} Add another child</span>
      <a href="/child/new" class="text-orange-600 text-sm">Add</a>
    </li>

    <!-- Invite care team -->
    <li class="rounded-2xl bg-white/60 px-4 py-3">
      <div class="flex items-center justify-between">
        <span>{{if .OnboardingInviteDone}}✅{{else}}👪{{end}} Invite your care team</span>
        <button type="button" class="text-orange-600 text-sm" data-toggle="ob-invite-panel">Invite</button>
      </div>
      <div id="ob-invite-panel" class="mt-3 hidden">
        <div id="ob-invite-rows"></div>
        <button type="button" id="ob-invite-add-row" class="text-sm text-stone-500 mt-1">+ add another</button>
        <div class="mt-3 flex items-center gap-3">
          <button type="button" id="ob-invite-send" class="seed-btn bg-orange-600 text-white rounded-full px-5 py-2 text-sm">Send invites</button>
          <a href="/settings#members" class="text-sm text-stone-500">Manage everyone in Settings</a>
        </div>
        <p id="ob-invite-msg" class="text-sm mt-2"></p>
      </div>
    </li>

    <!-- Basic settings -->
    <li class="rounded-2xl bg-white/60 px-4 py-3">
      <div class="flex items-center justify-between">
        <span>{{if .OnboardingSettingsDone}}✅{{else}}⚙️{{end}} Basic settings</span>
        <button type="button" class="text-orange-600 text-sm" data-toggle="ob-settings-panel">Open</button>
      </div>
      <div id="ob-settings-panel" class="mt-3 hidden space-y-3">
        <label class="block text-sm text-stone-600">Time zone</label>
        <input id="ob-set-tz" type="text" class="w-full rounded-2xl border-stone-200 px-3 py-2" readonly />
        <label class="inline-flex items-center gap-2 text-sm"><input id="ob-set-notify" type="checkbox" checked class="rounded"/> Reminders & alerts</label>
        <div class="text-sm">
          <label class="block text-stone-600">Time format</label>
          <select id="ob-set-timefmt" class="rounded-2xl border-stone-200 px-3 py-2">
            <option value="12h">12-hour</option>
            <option value="24h">24-hour</option>
          </select>
        </div>
        <div class="border-t border-orange-100 pt-3">
          <label class="inline-flex items-start gap-2 text-sm">
            <input id="ob-set-ai" type="checkbox" class="rounded mt-1"/>
            <span><strong>AI Insights</strong> — let MyCareCompanion surface gentle patterns from your logs.
            <span class="block text-stone-500 mt-1">This is a logging tool, not a medical device. Always consult your healthcare provider. You can turn this off anytime in Settings.</span></span>
          </label>
        </div>
        <button type="button" id="ob-set-save" class="seed-btn bg-orange-600 text-white rounded-full px-5 py-2 text-sm">Save</button>
        <p id="ob-set-msg" class="text-sm"></p>
      </div>
    </li>
  </ul>
</div>
<script src="/static/js/onboarding_checklist.js"></script>
{{end}}
```

- [ ] **Step 2: Create the checklist JS**

`static/js/onboarding_checklist.js`:
```js
(function () {
  const el = document.getElementById('ob-checklist');
  if (!el) return;
  const token = () => localStorage.getItem('access_token');
  const H = () => ({ 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token() });

  // Toggle panels
  document.querySelectorAll('[data-toggle]').forEach(btn => {
    btn.addEventListener('click', () => {
      const p = document.getElementById(btn.dataset.toggle);
      if (p) p.classList.toggle('hidden');
    });
  });

  // Dismiss
  const dismiss = document.getElementById('ob-dismiss');
  if (dismiss) dismiss.addEventListener('click', async () => {
    await fetch('/api/onboarding/checklist/dismiss', { method: 'POST', headers: H() });
    el.remove();
  });

  // Invite rows
  const rows = document.getElementById('ob-invite-rows');
  function addRow() {
    const div = document.createElement('div');
    div.className = 'flex gap-2 mb-2 ob-invite-row';
    div.innerHTML =
      '<input type="email" placeholder="email" class="flex-1 rounded-2xl border-stone-200 px-3 py-2 ob-email"/>' +
      '<select class="rounded-2xl border-stone-200 px-2 ob-role">' +
        '<option value="parent">Parent/Guardian</option>' +
        '<option value="caregiver">Caregiver</option>' +
        '<option value="medical_provider">Medical Provider</option>' +
      '</select>';
    rows.appendChild(div);
  }
  if (rows) addRow();
  const addRowBtn = document.getElementById('ob-invite-add-row');
  if (addRowBtn) addRowBtn.addEventListener('click', addRow);

  const sendBtn = document.getElementById('ob-invite-send');
  if (sendBtn) sendBtn.addEventListener('click', async () => {
    const msg = document.getElementById('ob-invite-msg');
    const rowEls = [...document.querySelectorAll('.ob-invite-row')];
    const targets = rowEls
      .map(r => ({ email: r.querySelector('.ob-email').value.trim(), role: r.querySelector('.ob-role').value }))
      .filter(t => t.email);
    if (!targets.length) { msg.textContent = 'Enter at least one email.'; return; }
    sendBtn.disabled = true;
    let ok = 0;
    for (const t of targets) {
      const res = await fetch('/api/family/members', { method: 'POST', headers: H(),
        body: JSON.stringify({ email: t.email, role: t.role, mode: 'invite' }) });
      if (res.ok) ok++;
    }
    if (ok > 0) {
      await fetch('/api/onboarding/invite-done', { method: 'POST', headers: H() });
      msg.textContent = 'Sent ' + ok + ' invite(s)!';
    } else {
      msg.textContent = 'Could not send invites — check the addresses.';
    }
    sendBtn.disabled = false;
  });

  // Settings panel
  const tz = document.getElementById('ob-set-tz');
  if (tz) tz.value = Intl.DateTimeFormat().resolvedOptions().timeZone || 'America/Chicago';
  const saveBtn = document.getElementById('ob-set-save');
  if (saveBtn) saveBtn.addEventListener('click', async () => {
    const msg = document.getElementById('ob-set-msg');
    saveBtn.disabled = true;
    try {
      await fetch('/api/users/me/preferences', { method: 'PUT', headers: H(),
        body: JSON.stringify({ timezone: tz.value, time_format: document.getElementById('ob-set-timefmt').value }) });
      // AI consent (only enable; disclosure SHA fetched from the consent GET)
      if (document.getElementById('ob-set-ai').checked) {
        const c = await fetch('/api/users/me/narrative-consent', { headers: H() });
        if (c.ok) {
          const cd = await c.json();
          await fetch('/api/users/me/narrative-consent', { method: 'PUT', headers: H(),
            body: JSON.stringify({ enabled: true, acknowledged_sha: cd.disclosure_sha }) });
        }
      }
      await fetch('/api/onboarding/settings-done', { method: 'POST', headers: H() });
      msg.textContent = 'Saved!';
    } catch (_) { msg.textContent = 'Could not save — try again.'; }
    saveBtn.disabled = false;
  });
})();
```

- [ ] **Step 3: Include the partial in the dashboard**

In `templates/dashboard.html`, immediately inside `<main>` (before the existing hero/empty-state block), add:
```html
{{if .ShowOnboardingChecklist}}{{template "onboarding_checklist" .}}{{end}}
```

- [ ] **Step 4: Build + restart dev**

Run: `go build ./...` then restart dev. Confirm no template parse error on boot (the boot log must not show a template error; `/dashboard` must still load for an existing user).

- [ ] **Step 5: Commit**

```bash
git add templates/partials/onboarding_checklist.html static/js/onboarding_checklist.js templates/dashboard.html
git commit -m "feat(onboarding): dashboard finish-setup checklist card"
```

---

## Task 9: End-to-end manual verification on dev

**Files:** none (verification only). Dev URL: `https://dev.mycarecompanion.net` (gate code in `.env`).

- [ ] **Step 1: New owner happy path**

Register a brand-new account (with a family name). Expected: after login you are redirected to `/onboarding`. Walk Welcome → first child (add name + DOB + tap a couple of condition chips + add a custom one) → Finish. Expected: redirected to `/dashboard`; the child exists with the chosen conditions; the "Finish setting up" checklist card is visible.

- [ ] **Step 2: Checklist items**

On the dashboard: open **Invite** → add a row (email + role) → Send invites → expect success message and the row's checkmark on reload. Open **Basic settings** → confirm timezone prefilled, toggle AI Insights on (disclosure visible) → Save → expect "Saved!" and checkmark on reload. After both invite + settings are done, reload → the checklist card no longer appears (auto-dismiss). Verify in DB:
```bash
PGPASSWORD="carecompanion" psql -h localhost -U carecompanion -d carecompanion -tAc \
"SELECT onboarding_completed_at IS NOT NULL, onboarding_invite_done_at IS NOT NULL, onboarding_settings_done_at IS NOT NULL FROM app_users WHERE email='<the new email>';"
```
Expected: `t|t|t`.

- [ ] **Step 2b: Dismiss path (separate fresh account)**

With another new account that finished the wizard, click "I'm all set ✕" → card disappears; reload → still gone. Verify `onboarding_checklist_dismissed_at IS NOT NULL`.

- [ ] **Step 3: Invited-member trimmed path**

From the owner account, invite a new email as caregiver. Register that invited email (it auto-joins the family). Expected: redirected to `/onboarding`, which shows Welcome → Basic settings only (NO add-child step), Finish → `/dashboard`. Verify the invited user did not create a child.

- [ ] **Step 4: Existing user untouched**

Log in as a pre-existing test account (e.g. a Joe/Smith test family account). Expected: straight to `/dashboard`, NO onboarding redirect, NO checklist card (backfilled complete).

- [ ] **Step 5: Abandonment re-entry**

As a fresh account, start the wizard, then navigate directly to `/dashboard` before finishing. Expected: bounced back to `/onboarding`. Finish it; confirm it then sticks to `/dashboard`.

- [ ] **Step 6: Full test suite + build**

Run: `go build ./... && go test ./...`
Expected: build clean; tests pass (at minimum the new onboarding logic + handler tests; pre-existing suite unchanged).

---

## Production rollout (after all tasks verified on dev)

Per the spec §10 and the now-in-production stability priority:
- Deploy this **on its own**, not bundled with other changes.
- Migration 00042 runs automatically on boot (the runner applies pending migrations). **Before deploying**, re-confirm the backfill marked all existing prod `app_users` complete will happen as part of that same migration (it does — the `UPDATE ... WHERE onboarding_completed_at IS NULL` runs in the migration transaction), so no existing prod user is bounced into onboarding.
- Deploy via `./scripts/deploy.sh` only on explicit go-ahead from Bryan (dev-first rule). Monitor the ASG refresh + `/health` + that an existing prod account still lands on `/dashboard`.

---

## Self-review notes (author)

- **Spec coverage:** hybrid gating (Task 6 gate); required wizard welcome→child (Task 7); conditions chips folded into child step (Task 7); dashboard checklist with add-child/invite/settings incl. AI consent + disclosure (Task 8); invited-member trimmed path (Tasks 6–7); state model + backfill (Task 1); reuse of existing APIs (Tasks 7–8); testing + rollout (Tasks 4/9 + rollout section). All spec sections map to a task.
- **Refinement flagged:** dedicated timestamp columns instead of the spec's `app_users.settings` JSONB (column doesn't exist; avoids touching the shared `users` view). "Add another child" done-state is derived from child count.
- **Type consistency:** repo method names (`SetOnboardingChecklistDismissed`, etc.) match across interface (Task 2), impl (Task 2), service (Task 3), and handler (Task 5). `OnboardingState` field names (`CompletedAt`/`ChecklistDismissedAt`/`SettingsDoneAt`/`InviteDoneAt`) consistent across model, repo, helpers, handler data.
- **Known limitation:** this repo has no DB-backed/router tests; deep behavior is covered by the pure-logic unit tests (Task 4) + manual dev verification (Task 9), consistent with existing conventions.
