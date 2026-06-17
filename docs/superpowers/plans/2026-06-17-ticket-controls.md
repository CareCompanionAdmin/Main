# Ticket Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let app users edit the type + priority of their own tickets (priority capped at High; Urgent staff-only), and let support staff edit type + priority + status from the admin portal — every change posting a thread note.

**Architecture:** A shared pure validator (`service.ValidateTicketFields`) enforces the permission matrix for both sides. New repo primitives update individual columns. The user side gets a new `PATCH /api/support/tickets/{ticketID}`; the admin side extends the existing `UpdateTicket` (`PUT /api/admin/support/tickets/{id}`), which today decodes priority but only applies status. User changes post a visible thread message (reusing `AddMessage`); admin changes post an internal one (`AddTicketMessage(..., isInternal=true)`).

**Tech Stack:** Go 1.24, Chi router, `database/sql` + Postgres, server-rendered `html/template`, vanilla JS with `fetch`.

## Global Constraints

- Enums are fixed: `type` ∈ {bug_report, feature_request, billing, general}; `priority` ∈ {low, normal, high, urgent}; `status` ∈ {open, in_progress, waiting_on_user, resolved, closed}. **No migration** — reuse existing columns/enums.
- **Permission matrix:** users may set type (any) + priority (low/normal/high only — Urgent rejected); users may NOT set status. Staff may set type/priority/status (full ranges incl. Urgent).
- Invalid values are **rejected** (400), never silently clamped.
- Changing type to/from `feature_request` only updates the column — **no** roadmap auto-promotion.
- Do this work on a branch: `git checkout -b feature/ticket-controls` before Task 1.
- Build/test env for every Go step:
  ```bash
  export PATH=$PATH:/usr/local/go/bin && export GOPATH=/home/carecomp/go
  cd /home/carecomp/carecompanion
  ```
- This repo has no DB-backed repo/handler tests by convention. Only the pure validator (Task 1) is unit-tested; the repo/service/handler/UI wiring is build-verified + covered by the Task 9 manual dev E2E.

## File map

- **Create** `internal/service/ticket_field_validation.go` — `ValidateTicketFields` + `ErrInvalidTicketField`.
- **Create** `internal/service/ticket_field_validation_test.go` — table tests.
- **Modify** `internal/repository/admin_repository.go` — add `UpdateTicketPriority` + `UpdateTicketType` to the `AdminRepository` interface (~line 174) and `adminRepo` impl (~after line 651). (`ReplicatingAdminRepo` embeds `AdminRepository`, so no wrapper edit needed.)
- **Modify** `internal/handler/admin/handlers.go` — extend `UpdateTicketRequest` + `UpdateTicket` (~lines 370-403) to validate, apply type/priority/status, post internal notes.
- **Modify** `internal/repository/user_support_repository.go` — add `UpdateOwnTicketFields` to the `UserSupportRepository` interface + `userSupportRepo` impl.
- **Modify** `internal/service/user_support_service.go` — add `UpdateTicketFieldsRequest` + `UpdateTicketFields`.
- **Modify** `internal/handler/api/support_handler.go` — add `UpdateTicketFields` HTTP handler.
- **Modify** `internal/handler/api/routes.go` — register `PATCH /tickets/{ticketID}/`.
- **Modify** `templates/support.html` — user type/priority dropdowns + change handler; drop Urgent from the new-ticket priority select.
- **Modify** `templates/admin/ticket_detail.html` — admin status/priority/type dropdowns wired to `UpdateTicket`.

---

## Task 1: Shared field validator (pure, TDD)

**Files:**
- Create: `internal/service/ticket_field_validation.go`
- Test: `internal/service/ticket_field_validation_test.go`

**Interfaces:**
- Produces: `service.ValidateTicketFields(actorIsStaff bool, ticketType, priority, status string) error` and `service.ErrInvalidTicketField` (wrapped by all validation failures).

- [ ] **Step 1: Write the failing test**

`internal/service/ticket_field_validation_test.go`:
```go
package service

import (
	"errors"
	"testing"
)

func TestValidateTicketFields(t *testing.T) {
	cases := []struct {
		name    string
		staff   bool
		typ     string
		prio    string
		status  string
		wantErr bool
	}{
		{"empty all ok", false, "", "", "", false},
		{"user valid type+prio", false, "bug_report", "high", "", false},
		{"user urgent rejected", false, "", "urgent", "", true},
		{"user bad type rejected", false, "nonsense", "", "", true},
		{"user bad priority rejected", false, "", "screaming", "", true},
		{"user status rejected", false, "", "", "open", true},
		{"staff urgent allowed", true, "", "urgent", "", false},
		{"staff status allowed", true, "", "", "waiting_on_user", false},
		{"staff bad status rejected", true, "", "", "nope", true},
		{"staff full combo", true, "feature_request", "urgent", "closed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateTicketFields(c.staff, c.typ, c.prio, c.status)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if c.wantErr && !errors.Is(err, ErrInvalidTicketField) {
				t.Fatalf("error should wrap ErrInvalidTicketField, got %v", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/service/ -run TestValidateTicketFields -v`
Expected: FAIL — `undefined: ValidateTicketFields` / `undefined: ErrInvalidTicketField`.

- [ ] **Step 3: Write the implementation**

`internal/service/ticket_field_validation.go` (reuses the existing `validTicketTypes` map already defined in `user_support_service.go`, same package):
```go
package service

import (
	"errors"
	"fmt"
)

// ErrInvalidTicketField is wrapped by every ticket field-validation failure so
// HTTP handlers can map it to 400 (vs. a 500 for genuine errors).
var ErrInvalidTicketField = errors.New("invalid ticket field")

// validTicketStatuses whitelists settable ticket status values.
var validTicketStatuses = map[string]bool{
	"open": true, "in_progress": true, "waiting_on_user": true,
	"resolved": true, "closed": true,
}

// ValidateTicketFields checks a requested type/priority/status change.
// An empty string for a field means "no change" and is always allowed.
// actorIsStaff gates the 'urgent' priority and any status change — app users
// may set priority only up to 'high' and may never set status directly.
func ValidateTicketFields(actorIsStaff bool, ticketType, priority, status string) error {
	if ticketType != "" && !validTicketTypes[ticketType] {
		return fmt.Errorf("%w: type %q", ErrInvalidTicketField, ticketType)
	}
	if priority != "" {
		switch priority {
		case "low", "normal", "high":
			// allowed for everyone
		case "urgent":
			if !actorIsStaff {
				return fmt.Errorf("%w: priority %q is reserved for support staff", ErrInvalidTicketField, priority)
			}
		default:
			return fmt.Errorf("%w: priority %q", ErrInvalidTicketField, priority)
		}
	}
	if status != "" {
		if !actorIsStaff {
			return fmt.Errorf("%w: users cannot change ticket status", ErrInvalidTicketField)
		}
		if !validTicketStatuses[status] {
			return fmt.Errorf("%w: status %q", ErrInvalidTicketField, status)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/service/ -run TestValidateTicketFields -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/service/ticket_field_validation.go internal/service/ticket_field_validation_test.go
git commit -m "feat(tickets): shared ValidateTicketFields helper + tests"
```

---

## Task 2: Admin repo — UpdateTicketPriority + UpdateTicketType

**Files:**
- Modify: `internal/repository/admin_repository.go` (interface ~line 174; impl ~after line 651)

**Interfaces:**
- Produces: `AdminRepository.UpdateTicketPriority(ctx, id uuid.UUID, priority string) error` and `AdminRepository.UpdateTicketType(ctx, id uuid.UUID, ticketType string) error`.

- [ ] **Step 1: Add to the `AdminRepository` interface**

In `internal/repository/admin_repository.go`, immediately after the `UpdateTicketStatus(...)` line in the interface (~line 174):
```go
	UpdateTicketPriority(ctx context.Context, id uuid.UUID, priority string) error
	UpdateTicketType(ctx context.Context, id uuid.UUID, ticketType string) error
```

- [ ] **Step 2: Add the impl**

After `ResolveTicket` (~line 651), mirroring `UpdateTicketStatus`:
```go
func (r *adminRepo) UpdateTicketPriority(ctx context.Context, id uuid.UUID, priority string) error {
	_, err := r.supportDB.ExecContext(ctx,
		`UPDATE support_tickets SET priority = $2, updated_at = NOW() WHERE id = $1`, id, priority)
	return err
}

func (r *adminRepo) UpdateTicketType(ctx context.Context, id uuid.UUID, ticketType string) error {
	_, err := r.supportDB.ExecContext(ctx,
		`UPDATE support_tickets SET type = $2, updated_at = NOW() WHERE id = $1`, id, ticketType)
	return err
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors (the interface + impl are now consistent; `ReplicatingAdminRepo` satisfies the new methods via its embedded `AdminRepository`).

- [ ] **Step 4: Commit**

```bash
git add internal/repository/admin_repository.go
git commit -m "feat(tickets): admin repo UpdateTicketPriority + UpdateTicketType"
```

---

## Task 3: Admin handler — extend UpdateTicket

**Files:**
- Modify: `internal/handler/admin/handlers.go` (`UpdateTicketRequest` + `UpdateTicket`, ~lines 370-403)

**Interfaces:**
- Consumes: `service.ValidateTicketFields`, `AdminRepository.{GetTicketByID, UpdateTicketStatus, UpdateTicketPriority, UpdateTicketType, AddTicketMessage}`.

- [ ] **Step 1: Ensure imports**

At the top of `internal/handler/admin/handlers.go`, confirm `"fmt"` and `"carecompanion/internal/service"` are imported. Add any that are missing to the import block.

- [ ] **Step 2: Replace `UpdateTicketRequest` + `UpdateTicket`**

Replace lines ~370-403 with:
```go
type UpdateTicketRequest struct {
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Type     string `json:"type"`
}

func (h *Handler) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	var req UpdateTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := service.ValidateTicketFields(true, req.Type, req.Priority, req.Status); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Load current values so change notes read "old → new".
	before, err := h.adminRepo.GetTicketByID(ctx, id)
	if err != nil {
		http.Error(w, "Failed to load ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if before == nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}

	claims := middleware.GetAuthClaims(ctx)
	var notes []string

	if req.Status != "" && req.Status != before.Status {
		if err := h.adminRepo.UpdateTicketStatus(ctx, id, req.Status); err != nil {
			http.Error(w, "Failed to update ticket: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// On close/resolve, purge attachments per our PHI promise to users.
		if (req.Status == "closed" || req.Status == "resolved") && h.attachService != nil {
			h.attachService.DeleteAllForTicket(ctx, id)
		}
		notes = append(notes, fmt.Sprintf("Status changed %s → %s", before.Status, req.Status))
	}
	if req.Priority != "" && req.Priority != before.Priority {
		if err := h.adminRepo.UpdateTicketPriority(ctx, id, req.Priority); err != nil {
			http.Error(w, "Failed to update ticket: "+err.Error(), http.StatusInternalServerError)
			return
		}
		notes = append(notes, fmt.Sprintf("Priority changed %s → %s", before.Priority, req.Priority))
	}
	if req.Type != "" && req.Type != before.Type {
		if err := h.adminRepo.UpdateTicketType(ctx, id, req.Type); err != nil {
			http.Error(w, "Failed to update ticket: "+err.Error(), http.StatusInternalServerError)
			return
		}
		notes = append(notes, fmt.Sprintf("Type changed %s → %s", before.Type, req.Type))
	}

	// Internal change notes (visible to staff only).
	for _, n := range notes {
		_ = h.adminRepo.AddTicketMessage(ctx, id, claims.UserID, n, true)
	}

	h.logAction(r, "update_ticket", "ticket", id, map[string]interface{}{
		"status": req.Status, "priority": req.Priority, "type": req.Type,
	})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/handler/admin/handlers.go
git commit -m "feat(tickets): admin UpdateTicket applies type/priority/status + internal notes"
```

---

## Task 4: User repo — UpdateOwnTicketFields

**Files:**
- Modify: `internal/repository/user_support_repository.go` (interface + `userSupportRepo` impl)

**Interfaces:**
- Produces: `UserSupportRepository.UpdateOwnTicketFields(ctx, ticketID, userID uuid.UUID, ticketType, priority string) (bool, error)` — ownership enforced in SQL; returns whether a row was updated.

- [ ] **Step 1: Add imports**

In `internal/repository/user_support_repository.go`, add `"fmt"` and `"strings"` to the import block (currently `context`, `database/sql`, `time`, `uuid`, `models`).

- [ ] **Step 2: Add to the `UserSupportRepository` interface**

After `ReopenTicket(...)` in the interface:
```go
	// UpdateOwnTicketFields updates the type and/or priority of a ticket the
	// user owns. Empty strings skip that field. Ownership is enforced in SQL
	// (WHERE id=$1 AND user_id=$2); returns true when a row was updated.
	UpdateOwnTicketFields(ctx context.Context, ticketID, userID uuid.UUID, ticketType, priority string) (bool, error)
```

- [ ] **Step 3: Add the impl**

At the end of the file:
```go
// UpdateOwnTicketFields updates type and/or priority for a ticket the user
// owns. Field validation (incl. the user priority cap) happens in the service
// layer; this method only builds the scoped UPDATE.
func (r *userSupportRepo) UpdateOwnTicketFields(ctx context.Context, ticketID, userID uuid.UUID, ticketType, priority string) (bool, error) {
	sets := []string{}
	args := []interface{}{ticketID, userID}
	n := 3
	if ticketType != "" {
		sets = append(sets, fmt.Sprintf("type = $%d", n))
		args = append(args, ticketType)
		n++
	}
	if priority != "" {
		sets = append(sets, fmt.Sprintf("priority = $%d", n))
		args = append(args, priority)
		n++
	}
	if len(sets) == 0 {
		return false, nil
	}
	sets = append(sets, "updated_at = NOW()")
	query := fmt.Sprintf("UPDATE support_tickets SET %s WHERE id = $1 AND user_id = $2", strings.Join(sets, ", "))
	res, err := r.supportDB.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
```

- [ ] **Step 4: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/user_support_repository.go
git commit -m "feat(tickets): user repo UpdateOwnTicketFields (ownership-scoped)"
```

---

## Task 5: User service — UpdateTicketFields

**Files:**
- Modify: `internal/service/user_support_service.go`

**Interfaces:**
- Consumes: `ValidateTicketFields`, `repo.{GetTicketByID, UpdateOwnTicketFields, AddMessage}`.
- Produces: `UserSupportService.UpdateTicketFields(ctx, ticketID, userID uuid.UUID, req *UpdateTicketFieldsRequest) (*repository.SupportTicket, error)` and `UpdateTicketFieldsRequest{Type, Priority string}`.

- [ ] **Step 1: Add `"fmt"` import**

In `internal/service/user_support_service.go`, add `"fmt"` to the import block.

- [ ] **Step 2: Add the request type + method**

Append:
```go
// UpdateTicketFieldsRequest is a user's request to change their own ticket's
// type and/or priority. Empty fields are left unchanged.
type UpdateTicketFieldsRequest struct {
	Type     string `json:"type"`
	Priority string `json:"priority"`
}

// UpdateTicketFields lets a user change the type and/or priority of a ticket
// they own. Priority is capped at "high" (Urgent is staff-only) — see
// ValidateTicketFields. Each changed field posts a visible thread note.
func (s *UserSupportService) UpdateTicketFields(ctx context.Context, ticketID, userID uuid.UUID, req *UpdateTicketFieldsRequest) (*repository.SupportTicket, error) {
	before, err := s.repo.GetTicketByID(ctx, ticketID, userID)
	if err != nil {
		return nil, err
	}
	if before == nil {
		return nil, ErrTicketNotFound
	}

	if err := ValidateTicketFields(false, req.Type, req.Priority, ""); err != nil {
		return nil, err
	}

	var notes []string
	if req.Type != "" && req.Type != before.Type {
		notes = append(notes, fmt.Sprintf("Type changed %s → %s", before.Type, req.Type))
	}
	if req.Priority != "" && req.Priority != before.Priority {
		notes = append(notes, fmt.Sprintf("Priority changed %s → %s", before.Priority, req.Priority))
	}

	if _, err := s.repo.UpdateOwnTicketFields(ctx, ticketID, userID, req.Type, req.Priority); err != nil {
		return nil, err
	}

	// Visible thread notes from the user (AddMessage writes is_internal=false).
	for _, n := range notes {
		_ = s.repo.AddMessage(ctx, ticketID, userID, n)
	}

	return s.repo.GetTicketByID(ctx, ticketID, userID)
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/service/user_support_service.go
git commit -m "feat(tickets): user service UpdateTicketFields with visible change notes"
```

---

## Task 6: User API handler + route

**Files:**
- Modify: `internal/handler/api/support_handler.go`
- Modify: `internal/handler/api/routes.go` (~line 424, inside the `/tickets/{ticketID}` subgroup)

**Interfaces:**
- Consumes: `service.UpdateTicketFieldsRequest`, `service.UpdateTicketFields`, `service.ErrInvalidTicketField`, `service.ErrTicketNotFound`.

- [ ] **Step 1: Add the handler**

Append to `internal/handler/api/support_handler.go`:
```go
// UpdateTicketFields lets the owning user change their ticket's type and/or
// priority (priority capped at High — Urgent is staff-only). Returns the
// updated ticket. 404 if not owned, 400 on invalid field values.
func (h *SupportHandler) UpdateTicketFields(w http.ResponseWriter, r *http.Request) {
	ticketID, err := parseUUID(chi.URLParam(r, "ticketID"))
	if err != nil {
		respondBadRequest(w, "Invalid ticket ID")
		return
	}
	userID := middleware.GetUserID(r.Context())

	var req service.UpdateTicketFieldsRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}
	if req.Type == "" && req.Priority == "" {
		respondBadRequest(w, "Nothing to update")
		return
	}

	ticket, err := h.supportService.UpdateTicketFields(r.Context(), ticketID, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTicketNotFound):
			respondNotFound(w, "Ticket not found")
		case errors.Is(err, service.ErrInvalidTicketField):
			respondBadRequest(w, err.Error())
		default:
			respondInternalError(w, "Failed to update ticket")
		}
		return
	}
	respondOK(w, ticket)
}
```

- [ ] **Step 2: Register the route**

In `internal/handler/api/routes.go`, inside the `r.Route("/tickets/{ticketID}", ...)` block (right after `r.Get("/", handlers.Support.GetTicket)`):
```go
				r.Patch("/", handlers.Support.UpdateTicketFields)
```

- [ ] **Step 3: Build + route smoke test**

Run: `go build ./...`
Then rebuild + restart dev (Task 9 covers the full restart procedure) and:
```bash
curl -s -m5 --noproxy '*' -b 'dev_gate_ok=1' -o /dev/null -w "%{http_code}\n" \
  -X PATCH http://localhost:8090/api/support/tickets/00000000-0000-0000-0000-000000000000/ \
  -H 'Content-Type: application/json' -d '{"priority":"high"}'
```
Expected: `401` (unauthenticated — confirms the route exists and isn't 404).

- [ ] **Step 4: Commit**

```bash
git add internal/handler/api/support_handler.go internal/handler/api/routes.go
git commit -m "feat(tickets): PATCH /api/support/tickets/{id} for user type/priority edits"
```

---

## Task 7: User UI — support.html controls

**Files:**
- Modify: `templates/support.html` (detail header ~line 88; `selectTicket` ~line 279; new-ticket priority `<select>` ~line 151)

- [ ] **Step 1: Add the editable controls to the detail header**

In `templates/support.html`, immediately after the `#ticket-created` span (line 87) and before the closing `</div>` at line 88, add a controls row:
```html
                            <label class="text-sm text-stone-500 ml-2">Type
                                <select id="ticket-type-select" onchange="updateTicketField('type', this.value)"
                                    class="ml-1 text-xs border border-stone-200 rounded-full px-2 py-0.5 bg-white">
                                    <option value="bug_report">Bug</option>
                                    <option value="feature_request">Feature</option>
                                    <option value="billing">Billing</option>
                                    <option value="general">General</option>
                                </select>
                            </label>
                            <label class="text-sm text-stone-500">Priority
                                <select id="ticket-priority-select" onchange="updateTicketField('priority', this.value)"
                                    class="ml-1 text-xs border border-stone-200 rounded-full px-2 py-0.5 bg-white">
                                    <option value="low">Low</option>
                                    <option value="normal">Normal</option>
                                    <option value="high">High</option>
                                </select>
                            </label>
```
(Note: the Priority select intentionally has no "Urgent" option — Urgent is staff-only.)

- [ ] **Step 2: Set the dropdowns' current values in `selectTicket`**

In `selectTicket`, right after the `typeEl.className = ...` line (~line 288), add:
```javascript
        document.getElementById('ticket-type-select').value = ticket.type || 'general';
        document.getElementById('ticket-priority-select').value =
            (ticket.priority === 'urgent') ? 'high' : (ticket.priority || 'normal');
```
(The Urgent→High coercion keeps a staff-set Urgent from being an invalid `<select>` value; the user can't lower a staff Urgent through this control without an explicit change, which is acceptable.)

- [ ] **Step 3: Add the `updateTicketField` function**

In the `<script>` block (e.g. just before `selectTicket`), add:
```javascript
async function updateTicketField(field, value) {
    if (!currentTicketID) return;
    const body = {};
    body[field] = value;
    try {
        const res = await fetch(`/api/support/tickets/${currentTicketID}/`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            alert('Could not update — please try again.');
            return;
        }
        // Re-render so the new thread note + updated fields show.
        selectTicket(currentTicketID);
    } catch (_) {
        alert('Network error — please try again.');
    }
}
```

- [ ] **Step 4: Drop Urgent from the new-ticket priority select**

In the new-ticket form's priority `<select>` (~line 151), remove any `<option value="urgent">` line so the create form offers only Low / Normal / High (consistency — users never self-assign Urgent).

- [ ] **Step 5: Rebuild + restart dev, smoke**

Run (per Task 9 restart procedure), then confirm `GET /support` still returns 200 for an authenticated session and no template parse error appears in the boot log. (Full click-through is Task 9.)

- [ ] **Step 6: Commit**

```bash
git add templates/support.html
git commit -m "feat(tickets): user type/priority controls on support ticket detail"
```

---

## Task 8: Admin UI — ticket_detail.html controls

**Files:**
- Modify: `templates/admin/ticket_detail.html` (action bar near the Assign/Resolve buttons, ~lines 78-83; uses the existing `apiCall(method, path, body)` helper)

- [ ] **Step 1: Add the controls row**

In `templates/admin/ticket_detail.html`, just after the Assign/Resolve button row (~line 83), add a dense controls row. `.Status`, `.Priority`, `.Type` are fields on the ticket template data (`SupportTicket`):
```html
        <div class="flex items-center gap-3 mt-3 text-sm">
            <label>Status
                <select id="adm-status" onchange="updateTicketAdmin('status', this.value)" class="ml-1 border rounded px-2 py-1">
                    <option value="open" {{if eq .Status "open"}}selected{{end}}>Open</option>
                    <option value="in_progress" {{if eq .Status "in_progress"}}selected{{end}}>In progress</option>
                    <option value="waiting_on_user" {{if eq .Status "waiting_on_user"}}selected{{end}}>Waiting on user</option>
                    <option value="resolved" {{if eq .Status "resolved"}}selected{{end}}>Resolved</option>
                    <option value="closed" {{if eq .Status "closed"}}selected{{end}}>Closed</option>
                </select>
            </label>
            <label>Priority
                <select id="adm-priority" onchange="updateTicketAdmin('priority', this.value)" class="ml-1 border rounded px-2 py-1">
                    <option value="low" {{if eq .Priority "low"}}selected{{end}}>Low</option>
                    <option value="normal" {{if eq .Priority "normal"}}selected{{end}}>Normal</option>
                    <option value="high" {{if eq .Priority "high"}}selected{{end}}>High</option>
                    <option value="urgent" {{if eq .Priority "urgent"}}selected{{end}}>Urgent</option>
                </select>
            </label>
            <label>Type
                <select id="adm-type" onchange="updateTicketAdmin('type', this.value)" class="ml-1 border rounded px-2 py-1">
                    <option value="bug_report" {{if eq .Type "bug_report"}}selected{{end}}>Bug report</option>
                    <option value="feature_request" {{if eq .Type "feature_request"}}selected{{end}}>Feature request</option>
                    <option value="billing" {{if eq .Type "billing"}}selected{{end}}>Billing</option>
                    <option value="general" {{if eq .Type "general"}}selected{{end}}>General</option>
                </select>
            </label>
        </div>
```

- [ ] **Step 2: Add the `updateTicketAdmin` function**

In the page's `<script>`, near the existing `assignToMe`/`resolveTicket` helpers, add (the ticket ID is available as `{{.ID}}` in those existing handlers):
```javascript
async function updateTicketAdmin(field, value) {
    const body = {};
    body[field] = value;
    const res = await fetch('/api/admin/support/tickets/{{.ID}}', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(body),
    });
    if (res.ok) {
        location.reload();
    } else {
        alert('Update failed: ' + (await res.text()));
    }
}
```
(If `apiCall` is the established pattern in this file, use `await apiCall('PUT', '/api/admin/support/tickets/{{.ID}}', body); location.reload();` instead — match the file's convention.)

- [ ] **Step 3: Rebuild + restart dev, smoke**

Restart dev; confirm the admin ticket detail page renders the three dropdowns with the ticket's current values selected and no template parse error on boot.

- [ ] **Step 4: Commit**

```bash
git add templates/admin/ticket_detail.html
git commit -m "feat(tickets): admin status/priority/type controls on ticket detail"
```

---

## Task 9: End-to-end verification on dev

**Files:** none (verification only). Dev: `https://dev.mycarecompanion.net` / `http://localhost:8090` with the `dev_gate_ok=1` cookie. Rebuild + restart procedure:
```bash
export PATH=$PATH:/usr/local/go/bin && export GOPATH=/home/carecomp/go
cd /home/carecomp/carecompanion
go build -buildvcs=false -o bin/carecompanion ./cmd/server
sudo systemctl restart carecompanion && sleep 4
curl -s -m5 -o /dev/null -w "health=%{http_code}\n" http://localhost:8090/health
```
(Template-only changes still require the restart — templates are parsed once at boot.)

- [ ] **Step 1: Full build + tests**

Run: `go build ./... && go test ./internal/service/ -run TestValidateTicketFields -v`
Expected: build clean; validator tests PASS.

- [ ] **Step 2: User happy path (Playwright or manual)**

As a logged-in app user (registration is normally closed on dev — reuse an existing dev account, or temporarily enable `registration_enabled` then restore it), open one of your own tickets:
- Change **Type** via the dropdown → page re-renders; a **visible** thread note "Type changed X → Y" appears.
- Set **Priority = High** → visible note appears; confirm the Priority dropdown offers only Low/Normal/High (no Urgent).
- Attempt `urgent` via direct API (`PATCH .../tickets/{id}/` body `{"priority":"urgent"}`) → expect **400** with the "reserved for support staff" message.

- [ ] **Step 3: User ownership guard**

`PATCH /api/support/tickets/{someoneElsesTicketID}/` as this user → expect **404** (not found / not owned), and verify in dev DB that the other ticket's priority/type are unchanged.

- [ ] **Step 4: Admin path**

In the admin portal ticket detail for the same ticket:
- Set **Priority = Urgent**, change **Status** (e.g. → waiting_on_user), change **Type** → page reloads showing new values.
- Confirm **internal** thread notes were posted (visible in the admin thread, `is_internal=true`) and do NOT appear in the user-facing thread.
- Confirm the admin audit log got an `update_ticket` entry.

- [ ] **Step 5: DB spot-check (read-only)**

Verify the changed ticket's `type`/`priority`/`status` columns match, and that `ticket_messages` has the expected notes with correct `is_internal` flags (user note false, admin notes true).

- [ ] **Step 6: Full suite**

Run: `go build ./... && go test ./...`
Expected: build clean; pre-existing suite unchanged (the `cmd/createadmin` vet failure is pre-existing and unrelated); new validator tests pass.

---

## Production rollout (after dev verification)

- Deploy on its own via `./scripts/deploy.sh`, on explicit go-ahead (dev-first rule). No migration. Monitor the ASG refresh + `/health` (note: `HealthCheckGracePeriod` is 300s after the 2026-06-17 fix).

## Self-review notes (author)

- **Spec coverage:** user type+priority edits (Tasks 4-7); user priority cap at High (Task 1 validator, Task 7 UI omits Urgent); admin type/priority/status (Tasks 2-3, 8); thread notes visible-vs-internal (Task 3 internal via `AddTicketMessage(...,true)`, Task 5 visible via `AddMessage`); ownership enforcement (Task 4 SQL scope + Task 5/9); no roadmap auto-promotion (type change is a plain column update — Tasks 3/4); testing (Task 1 unit + Task 9 manual). All spec sections map to a task.
- **Type consistency:** `ValidateTicketFields(actorIsStaff, type, priority, status)` and `ErrInvalidTicketField` used identically in Tasks 1/3/5/6; `UpdateOwnTicketFields(ctx, ticketID, userID, type, priority) (bool, error)` consistent across interface (Task 4), impl (Task 4), and caller (Task 5); admin `UpdateTicketPriority`/`UpdateTicketType` consistent across interface + impl (Task 2) and handler (Task 3).
- **No migration / no new enum values** — reuses existing columns and enums.
- **Known limitation:** wiring (repo/service/handler/UI) is build-verified + manually verified on dev, consistent with this repo's no-DB-test convention; only the pure validator is unit-tested.
