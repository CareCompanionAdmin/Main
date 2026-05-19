# CareCompanion Stability Audit Findings — Pre-QA-Engineer Pass

**Date**: 2026-05-19
**Trigger**: Pre-emptive stability scan ahead of the DLC QA engineer's adversarial testing.
**Method**: Six parallel agents (input validation, unhandled errors, network/transient, concurrency, auth/authz, template+frontend). Each scanned a defined slice of the Go + template + JS codebase.
**Outcome**: 33 P0, 45 P1, 45 P2 = **123 findings**.

This doc is the **inventory**. The triage decisions + actual fix list will live separately as a follow-on after Bryan + I walk through it together.

---

## Quick read

| Category               | P0 | P1 | P2 | Total |
| ---------------------- | -- | -- | -- | ----- |
| Auth / Authz           |  5 |  1 |  7 |    13 |
| Input validation       | 10 | 15 |  0 |    25 |
| Unhandled errors       |  7 | 11 | 12 |    30 |
| Network / transient    |  4 |  6 | 15 |    25 |
| Concurrency            |  3 |  4 |  8 |    15 |
| Template + frontend    |  4 |  8 |  3 |    15 |
| **Total**              | **33** | **45** | **45** | **123** |

**Top exploit-class concerns** (the things a QA engineer or a malicious user would actually hit):
1. **IDOR on alerts + child conditions + medication GET** — any logged-in parent can manipulate other families' data by guessing UUIDs. UUID space is huge so it's not casually exploitable, but it's a clear security finding.
2. **XSS in chat message rendering** — unescaped `${msg.message_text}` in template literal. Send a message with `<img src=x onerror="alert(1)">`, get script execution.
3. **Unbounded string fields** — `notes`, `message_text`, `description` accept arbitrary length. 100MB chat message floods DB, blocks queries.
4. **Date parsing bypasses** — `time.ParseInLocation` errors swallowed → silently defaults to today, breaking filters and bypassing signature verification on report links.
5. **Goroutines without `recover()`** — 7 instances across middleware/handlers. A panic in any of them kills the goroutine silently; under enough load, could mask cascading issues.

---

## Suggested minimum pre-QA fix list

The full P0+P1 list is 78 items, which is too much before she starts (would push QA out a week). Recommended cut for "fix before she starts" is below — prioritized by what she'll actually exercise.

| # | Category | File:line | What |
| - | -------- | --------- | ---- |
| 1 | Auth | `internal/handler/api/alert_handler.go:85-128, 131-170` | Alert ack/resolve/feedback IDOR — verify child ownership |
| 2 | Auth | `internal/handler/api/child_handler.go:268-342` | Condition update/remove IDOR — verify child ownership |
| 3 | Auth | `internal/handler/api/medication_handler.go:64-90` | Medication GET IDOR — verify child ownership |
| 4 | Frontend | `templates/chat.html:512, 481-485` | Chat XSS — escape message_text + attachment URLs |
| 5 | Frontend | `templates/medications.html:1166` | Interaction alert innerHTML XSS — use textContent |
| 6 | Validation | `internal/handler/api/chat_handler.go:140` | message_text max length cap (10k chars) |
| 7 | Validation | `internal/handler/api/log_handler.go:222-240` | Free-text fields max length cap (5k chars) |
| 8 | Validation | `internal/handler/api/log_handler.go:397-402` | Date parse error → return 400, no silent default |
| 9 | Validation | `internal/handler/api/report_handler.go:416` | ParseInt(exp) error check — signature bypass risk |
| 10 | Validation | `internal/handler/api/child_handler.go:73-76` | DOB ≤ today check |
| 11 | Validation | `internal/handler/api/log_handler.go:711-712, 831, 1335` | Numeric range bounds (weight, sleep, seizure duration) |
| 12 | Concurrency | `internal/repository/account_deletion_repo.go:157` | OTP attempt counter — SELECT FOR UPDATE |
| 13 | Concurrency | `internal/handler/web/billing_handler.go:155-188` | Stripe webhook idempotency (dedup on event.ID) |
| 14 | Network | `internal/service/drug_database.go:222,338,919,999` | OpenFDA io.LimitReader (10MB cap) |
| 15 | Network | `internal/service/email_service.go:69-144` | SMTP context timeout |
| 16 | Unhandled | All 7 goroutine sites | `defer recover()` wrappers |
| 17 | Unhandled | `internal/service/insight_per_metric.go:345,358` | Bounds check before `xs[0]` |
| 18 | Unhandled | `internal/service/stripe_service.go:304` | Bounds check before `Lines.Data[0]` |
| 19 | Auth | Caregiver role check on child condition modify | Add parent-only gate |
| 20 | Frontend | `static/js/global-search.js:81` | Show error in dropdown on fetch failure |

**Rough fix effort**: ~6-10 hours of focused work for these 20. Leaves the remaining 13 P0 and 30 P1 for post-QA cleanup.

---

## Full P0 findings — Auth / Authz

### 1. Medication GET — missing child ownership verification (P0)
- **File**: `internal/handler/api/medication_handler.go:64-90`
- **Exploit**: Any logged-in user can read medications from any family by guessing medication UUIDs. Handler fetches the medication, extracts ChildID, then checks access — but if the lookup pattern is off, info leaks.
- **Fix**: Verify `med.ChildID` belongs to authed user's family BEFORE returning. Or move to `GET /api/children/:childID/medications/:medID` form.

### 2. Child condition Update/Remove — no auth checks (P0)
- **File**: `internal/handler/api/child_handler.go:268-342`
- **Exploit**: `UpdateCondition` (269) and `RemoveCondition` (329) accept a condition ID and operate on it without `VerifyChildAccess`. Any logged-in user can UPDATE/DELETE any condition system-wide by guessing UUIDs.
- **Fix**: Fetch condition, extract child_id, call `childService.VerifyChildAccess(ctx, childID, userID)`.

### 3. Alert Acknowledge/Resolve — no child ownership check (P0)
- **File**: `internal/handler/api/alert_handler.go:85-128`
- **Exploit**: Any logged-in user can silence other families' health alerts by guessing alert UUIDs.
- **Fix**: Fetch alert, verify alert.ChildID belongs to authed family.

### 4. Alert Feedback create/get — no child ownership check (P0)
- **File**: `internal/handler/api/alert_handler.go:131-170`
- **Exploit**: Cross-family read/write of clinical feedback.
- **Fix**: Verify child access before proceeding.

### 5. Combined: condition handlers + alert handlers all bypass ownership (P0)
- Already covered above; called out as a class because the pattern is consistent and a single helper or middleware could close all of them at once.

---

## Full P0 findings — Input validation

### 6. log_handler.go:397-402 — date parse errors silently default (P0)
- `time.ParseInLocation` for start_date/end_date with errors ignored. Future dates accepted. Bypasses age/DOB validation downstream.
- Fix: Return 400 with "Invalid date format, use YYYY-MM-DD".

### 7. medication_handler.go:397-402 — same date parse bypass (P0)
- Date range filter on medication log queries silently uses default dates on parse errors.

### 8. report_handler.go:416 — ParseInt(exp) without err check (P0)
- Signed report link URLs include `?exp=<unix>&sig=<hmac>`. ParseInt returns 0 on bad input, then signature check uses 0 as expiry — signature verification can be bypassed.
- Fix: Check ParseInt err immediately, return 400.

### 9. log_handler.go (all Update*) — unbounded string fields (P0)
- Notes, descriptions, free-text fields have no max length. 10MB unicode → DB OOM or extreme query slowness.
- Fix: Cap 5k chars.

### 10. chat_handler.go:140 — unbounded message_text (P0)
- 100MB chat message would flood DB, block other queries.
- Fix: Cap 10k chars, return 413.

### 11. child_handler.go:73-76 — no future-DOB check (P0)
- POST /api/children with `{"date_of_birth": "2099-01-01"}` accepted. Breaks all age calculations downstream.
- Fix: Require DOB ≤ today.

### 12. log_handler.go:711-712 (weight), 831 (sleep), 1335 (seizure) — no numeric range checks (P0)
- weight_lbs accepts negative, 500000. seizure duration accepts 2147483647 seconds. Corrupts health data.
- Fix: Sensible ranges (weight 5-500, sleep 0-1440 min, seizure 0-3600s).

### 13. support_handler.go:56, 114 — XSS in description/message (P0)
- User-supplied HTML stored as-is, rendered unsafely in admin/user view.
- Fix: HTML-escape on output OR use safe template; cap input at 50k chars.

### 14. chat_handler.go:434-442 — Content-Disposition header injection (P0)
- Filename includes user-supplied `report.Title` without URL-encoding. Header injection possible.
- Fix: URL-encode filename.

### 15. (related) log_handler.go — silent "default to today" pattern (P0)
- Same date-parse swallow as #6 but in different handler. Consistent issue across log endpoints.

---

## Full P0 findings — Unhandled errors / panic risks

### 16-22. Goroutines without `recover()` — 7 sites (P0)
- `internal/middleware/error_tracking.go:70, 74` (logResponseTime, handleError)
- `internal/handler/admin/handlers.go:572-587` (push notification on ticket)
- `internal/handler/api/correlation_handler.go:80` (RunCorrelation)
- `internal/service/auth_service.go:248, 418` (email send, session touch)
- `internal/service/account_deletion_service.go:167, 225, 320` (email + async)
- `internal/middleware/ratelimit.go:31` (cleanup ticker loop)
- `internal/service/services.go:172` (Stripe plan sync on boot)
- A panic in any → silent goroutine death. Under load, can mask cascading failures.
- Fix: Wrap each with `defer func() { if r := recover(); r != nil { log.Printf(...) } }()`.

---

## Full P0 findings — Network / transient

### 23. Stripe webhook context not inherited to DB calls (P0)
- `internal/handler/web/billing_handler.go:180` + `stripe_service.go HandleEvent()`. Webhook has 10s timeout, but DB calls inside HandleEvent don't inherit. If DB hangs, handler times out, returns 200 to Stripe, Stripe stops retrying — silent subscription state drift.
- Fix: Pass context through, check ctx.Err() before each write.

### 24. OpenFDA no response-size limit (P0)
- `drug_database.go:222, 338, 919, 999` — `json.NewDecoder(resp.Body)` without `io.LimitReader`. Malformed/streaming JSON → OOM.
- Fix: Wrap in `io.LimitReader(resp.Body, 10*1024*1024)`.

### 25. AI Insights — no context timeout on Claude calls (P0)
- `ai_insight_service.go:641, 650, 656` — parent context has no deadline. If Claude is slow, goroutines pile up.
- Note: Prod has CLAUDE_ENABLED unset so this is dev-only risk today; becomes prod-critical after Phase 5 (Bedrock).
- Fix: `context.WithTimeout(ctx, 45*time.Second)` around the call.

### 26. S3 upload partial-failure orphans (P0)
- `report_service.go:756` + `attachment_storage.go:165`. S3 PutObject mid-upload failure leaves partial object in S3 + no cleanup. Retry uploads new copy, original orphaned.
- Fix: On error, delete S3 key if it exists. Use multipart upload with abort on error.

---

## Full P0 findings — Concurrency

### 27. OTP attempt counter race (P0)
- `internal/repository/account_deletion_repo.go:157`. Two concurrent wrong-OTP submissions both read count, both increment — bypass max-attempts.
- Fix: SELECT FOR UPDATE before check.

### 28. Ticket number collision risk if app-generated (P0)
- Migration 00033 added UNIQUE ticket_number. If app computes via MAX+1 (vs DB SERIAL), concurrent inserts collide.
- Fix: Confirm DB-generated; if not, switch to SERIAL or use SELECT FOR UPDATE.

### 29. Mirror replication orphan on local commit failure (P0)
- `internal/repository/replicating_admin_repo.go:74-82, 145-148`. Mirror write succeeds but local commit fails → mirror has admin row local doesn't. Env consistency breaks.
- Fix: Accept periodic reconciliation OR commit local first OR add mandatory reconciliation on boot.

---

## Full P0 findings — Template + frontend

### 30. Chat XSS via message_text (P0)
- `templates/chat.html:512`. `${msg.message_text}` inserted into template literal without escaping. `<img src=x onerror=...>` executes.
- Fix: `escapeHtml(msg.message_text)` before insertion.

### 31. Chat XSS via attachment URL/filename (P0)
- `templates/chat.html:481-485`. `${att.url}` and `${att.filename}` not escaped.
- Fix: `escapeHtml()` on both.

### 32. Medications interaction alert innerHTML XSS (P0)
- `templates/medications.html:1166`. `detailsDiv.innerHTML = html` with unsanitized `interaction.description` and `otherDrug`.
- Fix: Use `textContent` or escape HTML.

### 33. Offline global-search silent fetch failure (P0)
- `static/js/global-search.js:81`. Empty `.catch()` — dropdown silently shows stale results on network drop.
- Fix: Show "Network error" in dropdown on fail.

---

## P1 findings (selected — full list above by category)

### Input validation P1 (15 total)
- Whitespace-only dosage/dosage_unit (medication_handler.go:118-120)
- Password length max not enforced (user_handler.go:63-66) — 50k char password = slow bcrypt
- Email format not validated before lookup (family_handler.go:120-130)
- Behavior scale fields (mood, energy, anxiety) accept any *int (log_handler.go:137-142)
- ConditionName no max length / no ICD validation (child_handler.go:229)
- Timezone not validated (family_handler.go:446)
- Attachment filename not sanitized — path traversal possible (support_handler.go:182)
- Offset param unclamped in GetQuickSummary (log_handler.go:1551-1553)
- Attachments array unbounded (chat_handler.go:215-224)
- ... and 6 others

### Auth P1
- Caregivers can modify child conditions (parent-only action) — child_handler.go condition handlers, no role gate

### Unhandled errors P1 (11)
- JSON encode errors ignored across helpers + admin handlers (multiple files)
- JSON Unmarshal errors ignored in admin_repository.go metrics cache (lines 828, 848, 859, 876, 890, 1077, 1103, 1168)
- io.Copy errors ignored in PDF + medication image streams
- ExecContext errors ignored in error_tracking middleware

### Network/transient P1 (6)
- Claude API 503/timeout → user sees raw 500 instead of "AI temporarily unavailable"
- Sync SMTP failure on deletion flow → handler hangs full net.Dial timeout
- FCM silently fails on token invalidation — returns lastErr inconsistently
- PostgreSQL pool exhaustion → no backpressure, immediate 500
- DailyMed search hangs (3-5s timeout missing)
- Stripe checkout single-shot, no retry on transient 429/503

### Concurrency P1 (4)
- Session revocation bypass via cached "valid" entry (5min TTL window)
- OTP attempt limit TOCTOU
- Family invitation accept without status='pending' check (duplicate memberships possible)
- Password reset token reuse via two-tab race

### Frontend P1 (8)
- Message form button disabled forever on upload error
- Modal focus trap missing (keyboard escapes new-thread modal)
- No loading state on message send (button looks dead)
- Silent error on message send failure (text cleared, no toast)
- SSE EventSource auto-reconnect with no user feedback
- localStorage quota errors silently swallowed (session_guard.js)
- setInterval memory leak in capacitor-bridge.js (2 instances)
- Re-register push token spam (every second on dashboard)

---

## P2 findings (selected highlights)

- CSRF middleware not visible on web POST/PUT/DELETE routes
- Login endpoint not rate-limited (only password reset is)
- Password reset enumeration possible via timing
- Account deletion session revocation may be incomplete
- Stripe webhook event_id idempotency check missing
- Subscription state machine concurrent transitions not serialized
- Report scheduler missing distributed lock (multi-pod)
- Migration runner could double-run on simultaneous ASG boot
- String/rune conversion bugs in error tracking + audit log
- X-Forwarded-For parsing broken in rate limiter (idx logic)
- Dev gate User-Agent bypass is spoofable (acceptable trade-off?)
- Account deletion restore token expiry not checked on every use
- Redis pool sizing not tuned, no MaxRetries
- iOS-specific: localStorage quota, mobile viewport edge cases
- App Store Connect API JWT expiry not handled mid-request

---

## What I'd skip for the pre-QA pass

- All P2 items (45 of them) — most are polish, defensive hardening, or theoretical races. Log in `project_carecompanion_pending_issues.md` for post-QA cleanup.
- The "soft-delete state machine" follow-ups around account deletion — Phase 2 work, non-blocking for adversarial QA.
- CSRF middleware add — significant scope, deserves its own slice; meanwhile the JWT-only API is the surface she'll mostly exercise.
- Dev-gate User-Agent bypass hardening — acceptable as designed (defense-in-depth, not the only auth).

---

## Resume protocol

If session crashes during triage or fix work, next session should:
1. Read this doc + the spec at `docs/superpowers/specs/2026-05-19-dlc-qa-setup-design.md`.
2. Check git log for `qa-fix:` prefix commits to see what's already landed.
3. Cross-reference against the "minimum pre-QA fix list" table above.
4. Pick up at the first un-landed item.
