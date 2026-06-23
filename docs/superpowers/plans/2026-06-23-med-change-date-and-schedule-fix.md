# Med-Change Effective Date + Schedule-Change Dedup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve three production tickets in the medication date/time area: #112402 (med change shows on the wrong day), #112369 (no way to edit/backdate a med change), and #112397 (editing a med's schedule time produces a "double meds" duplicate in the daily checklist).

**Architecture:** Add a user-controlled `effective_date` to `treatment_changes` (defaulting to the user's *local* today, not the UTC server date), drive all med-change day bucketing off that column, and expose a clickable calendar-pill → day-detail modal that lets the user view and edit the change date. Separately, fix the medication-update schedule sync so it reconciles schedules in place (preserving schedule IDs and today's already-logged dose linkage) instead of deactivate-all-then-recreate.

**Tech Stack:** Go 1.24 + Chi, PostgreSQL (migration runner runs whole-file as one tx; no goose directives), server-rendered HTML + Tailwind, vanilla JS fetch.

## Global Constraints

- Dev DB only for migrations during development: `PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -f migrations/<file>.sql`. **No prod migration without Bryan's explicit go-ahead.**
- Migration files: no goose directives, whole file runs as one transaction, rollback SQL in a bottom comment block (see [[reference_migration_runner_quirks]]).
- This is parent-facing medical-adjacent code: production stability is a priority. Test on dev before declaring done; narrow blast radius.
- Per-user IANA timezone already exists: `app_users.timezone` (e.g. `America/New_York`); web handlers resolve it at `internal/handler/web/handlers.go:207` defaulting to `America/New_York`.
- Build: `export PATH=$PATH:/usr/local/go/bin && go build ./...`; restart dev to pick up template/Go changes: `sudo systemctl restart carecompanion` (templates that are pure CSS/JS are live; Go + template-struct changes need restart per [[reference_carecompanion_dev_ui_verification]]).

---

## File Structure

- `migrations/00043_treatment_change_effective_date.sql` — **create**: add `effective_date DATE`, backfill, NOT NULL + default.
- `internal/models/transparency.go:270` — **modify**: add `EffectiveDate` field to `TreatmentChange`.
- `internal/repository/transparency_repo.go` — **modify**: `CreateTreatmentChange` INSERT + all SELECT scans include `effective_date`; `GetMedChangeDates` buckets on `effective_date`; add `GetTreatmentChangesByDate`, `UpdateTreatmentChangeEffectiveDate`.
- `internal/service/medication_service.go` — **modify**: `*WithTracking` methods accept a `*time.Location` and stamp `tc.EffectiveDate`; fix schedule sync in `UpdateWithTracking`.
- `internal/repository/medication_repo.go` — **modify**: add `ReconcileSchedules` (or reuse Update/Create by time_of_day) to preserve schedule IDs.
- `internal/service/transparency_service.go` — **modify**: pass-throughs for the two new repo methods.
- `internal/handler/api/transparency_handler.go` — **modify**: add `GetTreatmentChangesByDate` + `UpdateTreatmentChangeEffectiveDate` handlers; resolve user tz.
- `internal/handler/api/medication_handler.go:201` — **modify**: pass resolved `*time.Location` into `UpdateWithTracking`.
- `internal/handler/api/routes.go` — **modify**: register the two new routes.
- `templates/child_dashboard.html:1161-1217` — **modify**: make the 💊 pill clickable → open day-detail modal; add the modal + JS.

---

## Task 1: Migration — add `effective_date` to `treatment_changes`

**Files:**
- Create: `migrations/00043_treatment_change_effective_date.sql`

**Interfaces:**
- Produces: `treatment_changes.effective_date DATE NOT NULL DEFAULT CURRENT_DATE`.

- [ ] **Step 1: Write the migration**

```sql
-- 00043_treatment_change_effective_date.sql
-- Adds a user-controlled effective date to treatment changes so the calendar
-- pill and med-change history reflect the day the change actually took effect
-- in the user's local timezone, not the UTC server timestamp.
-- Fixes #112402 (wrong day) and underpins #112369 (editable date).

ALTER TABLE treatment_changes ADD COLUMN IF NOT EXISTS effective_date DATE;

-- Backfill historical rows from created_at in a sensible default zone.
-- (Per-row owner tz is not reliably joinable for all legacy rows; America/Chicago
-- is the app's default-ish US zone and is close enough for historical pills.)
UPDATE treatment_changes
SET effective_date = (created_at AT TIME ZONE 'America/Chicago')::date
WHERE effective_date IS NULL;

ALTER TABLE treatment_changes ALTER COLUMN effective_date SET DEFAULT CURRENT_DATE;
ALTER TABLE treatment_changes ALTER COLUMN effective_date SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_treatment_changes_effective_date
  ON treatment_changes(child_id, effective_date);

-- ROLLBACK:
-- DROP INDEX IF EXISTS idx_treatment_changes_effective_date;
-- ALTER TABLE treatment_changes DROP COLUMN IF EXISTS effective_date;
```

- [ ] **Step 2: Apply on dev and verify**

Run: `PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -f migrations/00043_treatment_change_effective_date.sql`
Then: `PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -c "\d treatment_changes" | grep effective_date`
Expected: column `effective_date | date | not null | CURRENT_DATE`.

- [ ] **Step 3: Verify backfill non-null**

Run: `PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion -c "SELECT count(*) FILTER (WHERE effective_date IS NULL) AS nulls, count(*) AS total FROM treatment_changes;"`
Expected: `nulls = 0`.

- [ ] **Step 4: Commit**

```bash
git add migrations/00043_treatment_change_effective_date.sql
git commit -m "feat(med-change): add effective_date to treatment_changes (migration)"
```

---

## Task 2: Model field

**Files:**
- Modify: `internal/models/transparency.go:286` (after `CreatedAt`)

**Interfaces:**
- Produces: `TreatmentChange.EffectiveDate string` (formatted `2006-01-02`).

- [ ] **Step 1: Add field after `CreatedAt time.Time`**

```go
	CreatedAt                       time.Time           `json:"created_at" db:"created_at"`
	EffectiveDate                   string              `json:"effective_date" db:"effective_date"`
```

- [ ] **Step 2: Build**

Run: `export PATH=$PATH:/usr/local/go/bin && go build ./internal/models/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/models/transparency.go
git commit -m "feat(med-change): add EffectiveDate to TreatmentChange model"
```

---

## Task 3: Repo — write/read/edit `effective_date`

**Files:**
- Modify: `internal/repository/transparency_repo.go` (`CreateTreatmentChange` ~248, the SELECT scan ~230, `GetMedChangeDates` ~521)
- Test: `internal/repository/transparency_repo_effdate_test.go` (create)

**Interfaces:**
- Consumes: `TreatmentChange.EffectiveDate` (Task 2).
- Produces:
  - `CreateTreatmentChange` persists `effective_date` (defaults to `CURRENT_DATE` if `tc.EffectiveDate == ""`).
  - `GetMedChangeDates(ctx, childID, start, end)` buckets by `effective_date`.
  - `GetTreatmentChangesByDate(ctx, childID, date string) ([]models.TreatmentChange, error)`.
  - `UpdateTreatmentChangeEffectiveDate(ctx, id, childID, date string) error`.

- [ ] **Step 1: Update `CreateTreatmentChange` INSERT** — add `effective_date` as a conditional column. Replace the query/exec at `transparency_repo.go:250-262` with:

```go
	query := `
		INSERT INTO treatment_changes (
			child_id, change_type, source_table, source_id, previous_value, new_value,
			change_summary, changed_by_user_id, potentially_related_alert_id,
			potentially_related_share_thread_id, days_since_analysis_shared, effective_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, COALESCE(NULLIF($12,'')::date, CURRENT_DATE))
		RETURNING id, interrogative_status, created_at, effective_date::text`

	return r.db.QueryRowContext(ctx, query,
		tc.ChildID, tc.ChangeType, tc.SourceTable, tc.SourceID, tc.PreviousValue,
		tc.NewValue, tc.ChangeSummary, tc.ChangedByUserID, tc.PotentiallyRelatedAlertID,
		tc.PotentiallyRelatedShareThreadID, tc.DaysSinceAnalysisShared, tc.EffectiveDate,
	).Scan(&tc.ID, &tc.InterrogativeStatus, &tc.CreatedAt, &tc.EffectiveDate)
```

- [ ] **Step 2: Update `GetMedChangeDates`** at `transparency_repo.go:522-529` to bucket on `effective_date`:

```go
	query := `
		SELECT DISTINCT effective_date::text
		FROM treatment_changes
		WHERE child_id = $1
		  AND effective_date >= $2::date
		  AND effective_date < $3::date
		  AND change_type IN ('medication_added', 'medication_discontinued', 'medication_dose_changed', 'medication_schedule_changed', 'medication_switched')
	`
```

- [ ] **Step 3: Add the two new methods** (append near the other treatment-change methods):

```go
// GetTreatmentChangesByDate returns medication-related treatment changes whose
// effective_date matches the given YYYY-MM-DD for a child.
func (r *TransparencyRepository) GetTreatmentChangesByDate(ctx context.Context, childID, date string) ([]models.TreatmentChange, error) {
	query := `
		SELECT id, child_id, change_type, source_table, source_id, previous_value,
		       new_value, change_summary, changed_by_user_id, created_at, effective_date::text
		FROM treatment_changes
		WHERE child_id = $1 AND effective_date = $2::date
		  AND change_type IN ('medication_added','medication_discontinued','medication_dose_changed','medication_schedule_changed','medication_switched')
		ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query, childID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.TreatmentChange
	for rows.Next() {
		var tc models.TreatmentChange
		if err := rows.Scan(&tc.ID, &tc.ChildID, &tc.ChangeType, &tc.SourceTable,
			&tc.SourceID, &tc.PreviousValue, &tc.NewValue, &tc.ChangeSummary,
			&tc.ChangedByUserID, &tc.CreatedAt, &tc.EffectiveDate); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// UpdateTreatmentChangeEffectiveDate sets a new effective_date (YYYY-MM-DD),
// scoped by child_id to prevent cross-child edits.
func (r *TransparencyRepository) UpdateTreatmentChangeEffectiveDate(ctx context.Context, id, childID, date string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE treatment_changes SET effective_date = $1::date WHERE id = $2 AND child_id = $3`,
		date, id, childID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
```

(Ensure `database/sql` is imported in this file; it is already used elsewhere in the repo package — add the import if the file lacks it.)

- [ ] **Step 4: Update the existing list scan** at `transparency_repo.go:233-238` to also read `effective_date`. Add `effective_date::text` as the final selected column in that function's query and append `&tc.EffectiveDate` to its `rows.Scan(...)`. (Find the SELECT that feeds the scan at line 233 and add the column at the end of the column list.)

- [ ] **Step 5: Write a repo test** (`internal/repository/transparency_repo_effdate_test.go`) that, against the dev DB, inserts a change with `EffectiveDate: "2026-06-20"`, reads it via `GetTreatmentChangesByDate(child, "2026-06-20")`, asserts one row; calls `UpdateTreatmentChangeEffectiveDate(id, child, "2026-06-18")`; asserts `GetTreatmentChangesByDate(child, "2026-06-18")` returns it and the `2026-06-20` query is empty. Use the existing test DB harness pattern from `internal/repository/session_repo_test.go`.

- [ ] **Step 6: Run build + test**

Run: `export PATH=$PATH:/usr/local/go/bin && go build ./... && go test ./internal/repository/ -run EffDate -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/transparency_repo.go internal/repository/transparency_repo_effdate_test.go
git commit -m "feat(med-change): persist/read/edit effective_date; bucket calendar by it (#112402)"
```

---

## Task 4: Service — stamp effective_date in user-local time + service pass-throughs

**Files:**
- Modify: `internal/service/medication_service.go` (`UpdateWithTracking` ~99, `DiscontinueWithTracking` ~183, `DiscontinueWithReason` ~226, `UpdateLogWithTracking` ~382)
- Modify: `internal/service/transparency_service.go` (add pass-throughs ~297)

**Interfaces:**
- Consumes: repo methods (Task 3).
- Produces:
  - `*WithTracking` methods accept `loc *time.Location` and set `tc.EffectiveDate = time.Now().In(loc).Format("2006-01-02")` before `CreateTreatmentChange`.
  - `TransparencyService.GetTreatmentChangesByDate`, `TransparencyService.UpdateTreatmentChangeEffectiveDate` pass-throughs.

- [ ] **Step 1:** Change the signature of `UpdateWithTracking` to `(ctx, oldMed, newMed *models.Medication, userID uuid.UUID, loc *time.Location)`. Immediately after building **each** `tc` in that method (dosage branch ~118 and frequency branch ~142), set:

```go
				tc.EffectiveDate = time.Now().In(loc).Format("2006-01-02")
```

Do the same in `DiscontinueWithTracking`, `DiscontinueWithReason`, and `UpdateLogWithTracking` — add a `loc *time.Location` param and stamp `tc.EffectiveDate` before each `CreateTreatmentChange`. For any caller that lacks a location, pass `time.UTC` (preserves old behavior).

- [ ] **Step 2:** Add to `transparency_service.go`:

```go
func (s *TransparencyService) GetTreatmentChangesByDate(ctx context.Context, childID, date string) ([]models.TreatmentChange, error) {
	return s.repo.GetTreatmentChangesByDate(ctx, childID, date)
}

func (s *TransparencyService) UpdateTreatmentChangeEffectiveDate(ctx context.Context, id, childID, date string) error {
	return s.repo.UpdateTreatmentChangeEffectiveDate(ctx, id, childID, date)
}
```

- [ ] **Step 3:** Build (will fail at call sites until Task 6 — that's expected; just compile the service package):

Run: `export PATH=$PATH:/usr/local/go/bin && go build ./internal/service/`
Expected: PASS (service package compiles; callers updated in Task 6).

- [ ] **Step 4: Commit**

```bash
git add internal/service/medication_service.go internal/service/transparency_service.go
git commit -m "feat(med-change): stamp effective_date in user-local tz; add service pass-throughs (#112369)"
```

---

## Task 5: Fix schedule-change double-meds (#112397)

**Files:**
- Modify: `internal/service/medication_service.go:153-169` (the schedule-sync block in `UpdateWithTracking`)
- Modify: `internal/repository/medication_repo.go` (add `ReconcileSchedules`)
- Test: `internal/repository/medication_repo_reconcile_test.go` (create)

**Interfaces:**
- Produces: `medicationRepo.ReconcileSchedules(ctx, medicationID uuid.UUID, desired []models.MedicationSchedule) error` — updates existing active schedules in place (matched by `time_of_day`), inserts genuinely new ones, deactivates ones no longer present. Preserves schedule `id` for a kept `time_of_day` so today's `medication_logs.schedule_id` linkage survives a clock-time edit.

**Root cause:** `UpdateWithTracking` currently calls `DeactivateAllSchedules` then `CreateSchedule` for every schedule, minting new IDs. The due-meds query (`medication_repo.go:372 GetDueMedications`) joins `medication_logs ml ON ml.schedule_id = ms.id`. After a time edit, today's already-logged dose stays bound to the old (now-inactive) schedule id, so the new active schedule shows `is_logged = false` → the dose appears both logged (history) and still-due (checklist) = "double meds."

- [ ] **Step 1: Add `ReconcileSchedules`** to `medication_repo.go`:

```go
// ReconcileSchedules updates the medication's schedules in place, matched by
// time_of_day, so that an edit to the clock time of an existing slot keeps the
// same schedule id (and thus its existing medication_logs linkage). New slots
// are inserted; slots no longer present are deactivated.
func (r *medicationRepo) ReconcileSchedules(ctx context.Context, medicationID uuid.UUID, desired []models.MedicationSchedule) error {
	existing, err := r.GetSchedules(ctx, medicationID) // active schedules
	if err != nil {
		return err
	}
	byTOD := make(map[models.MedicationTimeOfDay]models.MedicationSchedule, len(existing))
	for _, e := range existing {
		byTOD[e.TimeOfDay] = e
	}
	keep := make(map[uuid.UUID]bool)
	for _, d := range desired {
		if cur, ok := byTOD[d.TimeOfDay]; ok {
			cur.ScheduledTime = d.ScheduledTime
			cur.DaysOfWeek = d.DaysOfWeek
			if err := r.UpdateSchedule(ctx, &cur); err != nil {
				return err
			}
			keep[cur.ID] = true
		} else {
			ns := &models.MedicationSchedule{MedicationID: medicationID, TimeOfDay: d.TimeOfDay, ScheduledTime: d.ScheduledTime, DaysOfWeek: d.DaysOfWeek}
			if err := r.CreateSchedule(ctx, ns); err != nil {
				return err
			}
		}
	}
	for _, e := range existing {
		if !keep[e.ID] {
			if err := r.DeactivateSchedule(ctx, e.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
```

(If `DeactivateSchedule(ctx, scheduleID)` does not exist, add it: `UPDATE medication_schedules SET is_active=false WHERE id=$1`. Confirm `UpdateSchedule` updates `scheduled_time` + `days_of_week` by id.)

- [ ] **Step 2: Swap the sync block** in `UpdateWithTracking` (`medication_service.go:154-169`) from the deactivate-all/recreate loop to:

```go
	if len(newMed.Schedules) > 0 {
		if err := s.medRepo.ReconcileSchedules(ctx, newMed.ID, newMed.Schedules); err != nil {
			return fmt.Errorf("failed to reconcile schedules: %w", err)
		}
	}
```

- [ ] **Step 3: Write a repo test** (`medication_repo_reconcile_test.go`): create a med with a `morning` schedule (capture id S1); log a dose today bound to S1; call `ReconcileSchedules` with a `morning` schedule whose `ScheduledTime` changed (e.g. 08:00→09:30); assert the active morning schedule still has id S1 (not a new id) and `GetDueMedications(child, today)` returns the morning slot with `IsLogged == true` (no duplicate). Reuse the dev-DB test harness.

- [ ] **Step 4: Build + test**

Run: `export PATH=$PATH:/usr/local/go/bin && go build ./... && go test ./internal/repository/ -run Reconcile -v`
Expected: PASS — morning schedule id unchanged, single due row, `IsLogged=true`.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/medication_repo.go internal/repository/medication_repo_reconcile_test.go internal/service/medication_service.go
git commit -m "fix(meds): reconcile schedules in place on edit to stop double-meds (#112397)"
```

---

## Task 6: Handlers + routes — pass tz, list/edit endpoints

**Files:**
- Modify: `internal/handler/api/medication_handler.go:201` (pass `loc`)
- Modify: `internal/handler/api/transparency_handler.go` (two new handlers)
- Modify: `internal/handler/api/routes.go` (register routes)

**Interfaces:**
- Consumes: service methods (Tasks 4, 5).
- Produces:
  - `GET /api/children/{childID}/treatment-changes?date=YYYY-MM-DD` → `[]TreatmentChange`.
  - `PATCH /api/treatment-changes/{id}` body `{"child_id":"…","effective_date":"YYYY-MM-DD"}` → 200.

- [ ] **Step 1:** In `medication_handler.go` (the update handler ~160-201), resolve the user's timezone the same way web handlers do and pass it:

```go
	loc, lerr := time.LoadLocation(h.userTimezone(r)) // helper: looks up app_users.timezone, default America/New_York
	if lerr != nil {
		loc = time.UTC
	}
	if err := h.medService.UpdateWithTracking(r.Context(), oldMed, &newMed, userID, loc); err != nil {
```

If no `userTimezone` helper exists on the API handler, inline: read the authenticated user's `timezone` via the user repo/service already injected, falling back to `"America/New_York"`. Update the other `*WithTracking` call sites (Discontinue paths, `UpdateLogWithTracking`) to pass a resolved `loc` (or `time.UTC` where a request tz isn't readily available).

- [ ] **Step 2:** Add handlers to `transparency_handler.go`:

```go
func (h *TransparencyHandler) GetTreatmentChangesByDate(w http.ResponseWriter, r *http.Request) {
	childID := chi.URLParam(r, "childID")
	date := r.URL.Query().Get("date")
	if date == "" {
		respondBadRequest(w, "date required (YYYY-MM-DD)")
		return
	}
	changes, err := h.service.GetTreatmentChangesByDate(r.Context(), childID, date)
	if err != nil {
		respondInternalError(w, "Failed to load changes")
		return
	}
	respondOK(w, changes)
}

func (h *TransparencyHandler) UpdateTreatmentChangeEffectiveDate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		ChildID       string `json:"child_id"`
		EffectiveDate string `json:"effective_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EffectiveDate == "" || body.ChildID == "" {
		respondBadRequest(w, "child_id and effective_date required")
		return
	}
	if _, perr := time.Parse("2006-01-02", body.EffectiveDate); perr != nil {
		respondBadRequest(w, "effective_date must be YYYY-MM-DD")
		return
	}
	if err := h.service.UpdateTreatmentChangeEffectiveDate(r.Context(), id, body.ChildID, body.EffectiveDate); err != nil {
		respondInternalError(w, "Failed to update date")
		return
	}
	respondOK(w, map[string]string{"status": "ok"})
}
```

**Authorization note:** scope must verify the requesting user has access to `body.ChildID` / `childID` the same way other child-scoped endpoints in this handler do (reuse the existing access-check helper used by sibling routes — do not skip it; this is a write).

- [ ] **Step 3:** Register in `routes.go` next to existing `/api/treatment-changes` routes:

```go
	r.Get("/api/children/{childID}/treatment-changes", transparencyHandler.GetTreatmentChangesByDate)
	r.Patch("/api/treatment-changes/{id}", transparencyHandler.UpdateTreatmentChangeEffectiveDate)
```

(Match the existing auth middleware grouping the other authenticated `/api` routes use.)

- [ ] **Step 4: Build**

Run: `export PATH=$PATH:/usr/local/go/bin && go build ./...`
Expected: PASS (all call sites now satisfied).

- [ ] **Step 5: Commit**

```bash
git add internal/handler/api/
git commit -m "feat(med-change): tz-aware effective_date + list/edit endpoints"
```

---

## Task 7: UI — clickable calendar pill → day-detail modal with editable date

**Files:**
- Modify: `templates/child_dashboard.html` (pill render ~1161-1217; add modal markup + JS)

**Interfaces:**
- Consumes: `GET /api/children/{childID}/treatment-changes?date=`, `PATCH /api/treatment-changes/{id}` (Task 6).

This task also resolves **#112379** (make the calendar pill clickable to show what changed).

- [ ] **Step 1:** Where the pill is rendered (3 spots: ~1161, ~1178, ~1212), add a click handler that calls `openMedChangeDay(day.date)` and a pointer cursor. Example for the cell wrapper: add `onclick="if('${day.has_med_change}'==='true') openMedChangeDay('${day.date}')"` and `${day.has_med_change ? 'cursor-pointer' : ''}` to the class list.

- [ ] **Step 2:** Add a modal + JS near the bottom of the dashboard script:

```html
<div id="medChangeDayModal" class="hidden fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4">
  <div class="bg-white rounded-2xl max-w-md w-full p-5">
    <div class="flex items-center justify-between mb-3">
      <h3 class="font-semibold text-stone-800">Medication changes</h3>
      <button onclick="closeMedChangeDay()" class="text-stone-400 hover:text-stone-600">✕</button>
    </div>
    <div id="medChangeDayList" class="space-y-3"></div>
  </div>
</div>
<script>
let _medChangeChildID = (document.body.dataset.childId || window.CHILD_ID || '');
async function openMedChangeDay(date) {
  const modal = document.getElementById('medChangeDayModal');
  const list = document.getElementById('medChangeDayList');
  list.innerHTML = '<p class="text-stone-400 text-sm">Loading…</p>';
  modal.classList.remove('hidden');
  try {
    const res = await fetch(`/api/children/${_medChangeChildID}/treatment-changes?date=${encodeURIComponent(date)}`, {credentials:'include'});
    const data = await res.json();
    if (!Array.isArray(data) || data.length === 0) { list.innerHTML = '<p class="text-stone-500 text-sm">No medication changes recorded for this day.</p>'; return; }
    list.innerHTML = data.map(c => `
      <div class="border border-stone-200 rounded-xl p-3">
        <p class="text-sm text-stone-700">${c.change_summary}</p>
        <label class="block mt-2 text-xs text-stone-500">Change date
          <input type="date" value="${c.effective_date}" class="mt-1 block w-full border rounded-lg px-2 py-1 text-sm"
                 onchange="saveMedChangeDate('${c.id}', this.value, '${date}')">
        </label>
      </div>`).join('');
  } catch (e) { list.innerHTML = '<p class="text-red-500 text-sm">Failed to load.</p>'; }
}
async function saveMedChangeDate(id, newDate, oldDate) {
  if (!newDate) return;
  try {
    const res = await fetch(`/api/treatment-changes/${id}`, {
      method:'PATCH', credentials:'include', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({child_id:_medChangeChildID, effective_date:newDate})
    });
    if (!res.ok) throw new Error();
    closeMedChangeDay();
    if (typeof loadCalendar === 'function') loadCalendar(); // refresh pills
  } catch (e) { alert('Could not update the date. Please try again.'); }
}
function closeMedChangeDay(){ document.getElementById('medChangeDayModal').classList.add('hidden'); }
</script>
```

(Confirm how the child id is exposed in this template — reuse the existing variable the page already uses for `/api/children/{id}/...` calls rather than `window.CHILD_ID` if a real one exists. Confirm the calendar-refresh function name.)

- [ ] **Step 2b:** Restart dev: `sudo systemctl restart carecompanion`.

- [ ] **Step 3: Manual verification on dev** (per [[reference_carecompanion_dev_ui_verification]] — Playwright over http with `MyCareCompanionApp` UA to bypass dev gate):
  1. Log in as a dev test family, edit a medication's dosage → a 💊 pill appears on **today** (user-local).
  2. Click the pill → modal lists the change with the correct summary.
  3. Change the date input to yesterday → modal closes, pill moves to yesterday.
  4. Edit a medication's morning **time** after logging the morning dose → daily checklist shows the morning dose as logged once, NOT duplicated (#112397).

- [ ] **Step 4: Commit**

```bash
git add templates/child_dashboard.html
git commit -m "feat(med-change): clickable calendar pill -> day detail with editable date (#112379, #112369)"
```

---

## Self-Review notes

- **#112402** (wrong day): Task 1 + Task 3 Step 2 (bucket on `effective_date`) + Task 4 Step 1 (stamp in user-local tz) → pill lands on the local day. ✓
- **#112369** (edit date): Task 3 (update method) + Task 6 (PATCH) + Task 7 (date input). ✓
- **#112397** (double meds): Task 5 (reconcile schedules in place). ✓
- **#112379** (clickable pill): Task 7. ✓ (bonus — was in the triage bucket)
- Open verification risk: the per-user tz lookup on the API handler (Task 6 Step 1) — confirm the API handler can resolve `app_users.timezone`; if not readily injected, default `America/New_York` and note it. The `UpdateLogWithTracking` and Discontinue paths can pass `time.UTC` initially if their request tz isn't trivially available — acceptable, since those aren't the reported symptom (dosage/schedule edits are).
- Migration backfill uses a fixed `America/Chicago` for legacy rows — historical pills may shift by a day in edge cases; acceptable and noted.
