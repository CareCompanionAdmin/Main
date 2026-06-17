# Ticket Controls — Design Spec

**Date:** 2026-06-17
**Status:** Approved (design); pending implementation plan

## Goal

Give both sides more control over support tickets after creation:

- **App users** can adjust the **type** and **priority** of their own tickets.
- **Support/admin staff** can change **type**, **priority**, and **status** of any ticket from the support portal (assign + resolve already exist).

Motivation: before the team asks real users to help validate which existing tickets are legitimate, users and staff need the controls to re-classify and re-prioritize tickets during that pass.

## Current state (verified 2026-06-17)

Enums (Postgres `USER-DEFINED`):
- `ticket_type`: `bug_report`, `feature_request`, `billing`, `general`
- `ticket_priority`: `low`, `normal`, `high`, `urgent`
- `ticket_status`: `open`, `in_progress`, `waiting_on_user`, `resolved`, `closed`

User side (`internal/service/user_support_service.go`, `internal/handler/api/support_handler.go`, `internal/repository/user_support_repository.go`, `templates/support.html`):
- Capabilities: create (type validated against `validTicketTypes`; priority passed through unvalidated), reply, reopen, mark-read.
- **No** way to change type or priority after creation.
- `support.html` is a list + `#ticket-detail-container` detail panel; `selectTicket()` fetches `GET /api/support/tickets/{id}` and renders `#ticket-status` / `#ticket-type` badges + a reopen button. The new-ticket form has `type` and `priority` `<select>`s.

Admin side (`internal/handler/admin/handlers.go`, `internal/repository/admin_repository.go`, `templates/admin/ticket_detail.html`):
- `PUT /tickets/{id}` `UpdateTicket` — `UpdateTicketRequest{Status, Priority}` but the handler **only applies `Status`**; `Priority` is decoded then **silently dropped**. No `Type` field.
- `UpdateTicketStatus`, `AssignTicket`, `ResolveTicket` repo methods exist. **No** `UpdateTicketPriority` / `UpdateTicketType`.
- `ticket_detail.html` action bar has Assign-to-Me and Resolve; a `setPriority`-style prompt exists but is for **roadmap** priority (p0–p3), not ticket priority. No ticket type/priority/status dropdowns.

## Decisions

1. **User priority ceiling:** users may set `low` / `normal` / `high`. `urgent` is **staff-only** (keeps Urgent meaningful for triage).
2. **Change visibility:** every type/priority/status change posts a **system note into the ticket thread** (reusing `ticket_messages`). User-initiated changes are visible (`is_internal=false`); admin-initiated changes are internal (`is_internal=true`). Admin changes also keep the existing audit-log entry.
3. **Approach:** shared repo primitives + thin per-side endpoints (admin and app live in separate route trees / auth middleware; one unified endpoint was rejected as messy, a generic field-patch as YAGNI).

## Permission matrix

| Field | App user (own ticket) | Support/admin staff |
|-------|----------------------|---------------------|
| Type | ✅ any of bug_report / feature_request / billing / general | ✅ any |
| Priority | ✅ low / normal / high — **urgent rejected** | ✅ low / normal / high / urgent |
| Status | ❌ (existing Reopen flow only) | ✅ open / in_progress / waiting_on_user / resolved / closed |

Invalid values are **rejected with a clear error**, never silently clamped.

## Components

### Repository
- `UpdateTicketStatus(ctx, id, status)` — exists, unchanged.
- **New** `UpdateTicketPriority(ctx, id, priority)` — `UPDATE support_tickets SET priority=$2, updated_at=now() WHERE id=$1` (admin repo; admin authority).
- **New** `UpdateTicketType(ctx, id, type)` — analogous (admin repo).
- **New** `UpdateOwnTicketFields(ctx, ticketID, userID, type, priority)` — user repo; ownership enforced in SQL (`WHERE id=$1 AND user_id=$2`), returns rows-affected so a non-owner (0 rows) is rejected. Only updates the fields provided (build the SET clause from non-empty inputs).

### Service / validation
- **Pure** `validateTicketFields(actorIsStaff bool, type, priority, status string) error` (service layer, unit-tested):
  - `type` (if set) ∈ {bug_report, feature_request, billing, general}.
  - `priority` (if set) ∈ {low, normal, high}; `urgent` allowed only when `actorIsStaff`.
  - `status` (if set) allowed only when `actorIsStaff`, and ∈ the status enum.
- User service `UpdateOwnTicketFields(ctx, ticketID, userID, req{type?, priority?})`: load ticket (ownership), validate (`actorIsStaff=false`), apply changed fields, post visible change note(s).
- Admin path extends `UpdateTicket` to validate (`actorIsStaff=true`) and apply type + priority (+ status as today), post internal note(s), keep `logAction`.

### Change-note helper
- `postChangeNote(ctx, ticketID, actorID, body string, internal bool)` — inserts a `ticket_messages` row. Body format: `"<Field> changed <Old> → <New>"` using human labels (e.g. `Type changed General → Bug report`, `Priority changed Normal → High`). One note per changed field (or a single combined note if multiple fields change at once — implementer's choice, kept readable).

### Endpoints
- **User (new):** `PATCH /api/support/tickets/{id}` → body `{type?, priority?}`. Returns the updated ticket JSON. Under the existing `/api/support` app-auth group.
- **Admin (extend):** `UpdateTicket` (`PUT /tickets/{id}`): add `Type` to `UpdateTicketRequest`; apply `Priority` (currently dropped); status unchanged. On close/resolve, the existing attachment-purge behavior is preserved.

### UI
- **User `templates/support.html`** detail panel: replace the `#ticket-type` display badge with a **Type** `<select>` and add a **Priority** `<select>` (Low / Normal / High — no Urgent) near `#ticket-status`. On `change`, call `PATCH /api/support/tickets/{id}` then re-render the thread so the new system note appears. Update `selectTicket()` to set the dropdowns' current values. Also drop `Urgent` from the **new-ticket** priority `<select>` for consistency (users never self-assign Urgent).
- **Admin `templates/admin/ticket_detail.html`**: add **Status**, **Priority** (incl. Urgent), and **Type** dropdowns to the action bar near Assign/Resolve; on `change`, call the extended `UpdateTicket` PUT and refresh. Dense/functional styling (admin needs data density; this is functional, not a visual redesign).

## Out of scope / explicit non-goals
- **Roadmap auto-promotion on type change:** changing a ticket's type to/from `feature_request` only updates the column. Promotion to the roadmap stays the separate existing admin "Add to roadmap" action — no auto-trigger.
- Users still cannot change **status** (Reopen remains the only user-driven status transition).
- No bulk re-classification UI (single-ticket controls only).
- No new notification/email on type/priority changes (the thread note is the record).

## Testing
- **Pure unit tests** for `validateTicketFields` (table-driven): user `urgent` rejected; admin `urgent` allowed; invalid type rejected; user attempting `status` rejected; valid user/admin combos pass.
- **Ownership** test at the user service level: a user cannot edit a ticket they don't own (0 rows affected → error).
- **Handler wiring** smoke test (constructor / route registered), consistent with repo convention.
- **Manual dev verification** end-to-end on `dev.mycarecompanion.net`:
  1. As an app user, open own ticket → change Type and set Priority=High → confirm the field updates and a **visible** thread note appears; confirm Urgent is not offered.
  2. As admin in the portal → set Priority=Urgent, change Status and Type → confirm fields update and an **internal** thread note appears (not shown to the user) + audit-log entry.
  3. Confirm a user cannot PATCH another user's ticket (403/empty).
  4. `go build ./... && go test ./...` — build clean, new unit tests pass.

## Rollout
- Dev-first; deploy on explicit go-ahead per project rules. No migration required (uses existing columns + enums). Standard `deploy.sh` path.
