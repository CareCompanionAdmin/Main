# DLC QA Setup — Design

**Date**: 2026-05-19
**Owner**: Bryan (operator), Claude (executor)
**Trigger**: Bryan secured a professional QA engineer (DLC). She needs (1) a known-good adversarial-testing environment with realistic data, (2) restorable test data so she can delete-and-restore freely, and (3) a pre-emptive stability pass so she finds new bugs, not the obvious ones we could have caught ourselves.

## Goals

1. **Stability audit + fuzz harness** — find and fix the obvious unhandled-error / input-validation / network-failure / authz issues before she starts. Two phases: (a) static audit by parallel agents producing a prioritized findings doc, (b) Go fuzz harness exercising adversarial payloads against dev.
2. **Six test accounts in a single family** with one year of heavy (~250-300/day) seeded data that triggers every scanner and insight type. Accounts:
   - `DLCparent1@test.com` — primary parent (`families.created_by`)
   - `DLCparent2@test.com` — parent
   - `DLCcaregiver1@test.com`, `DLCcaregiver2@test.com` — `caregiver` role
   - `DLCdr1@test.com`, `DLCdr2@test.com` — `medical_provider` role
   - Password for all: `TestPass1!`
   - Family: "DLC Test Family"
   - Children: 2, random names, different ages (planned: Mia ~7 with ASD-2/ADHD/epilepsy intensity; Ethan ~4 with ASD-1/SPD milder)
3. **Restore mechanism** with two layers:
   - **Layer 1**: surgical `revert.sql` + idempotent `seed.sql` (Smith Test Family precedent). Wipes only DLC family + 6 users + their tickets. Family CASCADE handles all log/child data. Re-running seed.sql recreates the entire dataset. Restore = `revert.sql && seed.sql`, takes seconds.
   - **Layer 2**: full pre-test `pg_dump` of dev and prod, stored at `/home/carecomp/secrets/db_backups/{dev,prod}_pre_dlc_qa_<ts>.dump`. pg_restore-able to whole-DB state at the start of the QA campaign.

## Approach

Fork the proven Smith Test Family generator at `/home/carecomp/secrets/db_backups/test_family/gen_seed.py` into `dlc_test_family/`. Same architecture: deterministic UUIDs (uuid5 from a fixed namespace), single bcrypt hash for all six accounts, idempotent INSERTs via ON CONFLICT, surgical revert that touches only the DLC family.

**Updates required from the Smith script**:
- Writes target `app_users` (not `users`) post-migration 00032
- Use `family_memberships` (current name) for membership rows
- Cover roles added since Smith was generated: confirm `parent`/`caregiver`/`medical_provider` enum values are unchanged (`family_role` enum, defined in migration 00001)
- Generate 365 days of data (vs Smith's 180)
- Increase daily data density to ~250-300 entries/day across all categories
- Plant deliberate patterns to trigger each insight scanner:
  - Auto-correlation: a food→behavior cluster (e.g., dairy on days N → behavior episode within 4h)
  - FDA side-effect: methylphenidate on Mia → cluster of headache logs in days following dose
  - FDA blackbox: risperidone on Mia → match the blackbox warning category
  - Medication-start-coincidence: schedule a dose change mid-year and have a metric clearly shift before/after
  - Clinical rules: hit autism-population thresholds (sleep deficit, behavior intensity, sensory)
  - Per-metric anomaly/trend/changepoint: deliberate trend in mood scale Q3-Q4, a one-week anomaly cluster, a changepoint coinciding with a fictional environmental change (e.g., school start)

**Distribution shape**: vary by day-of-week (more meals/play on weekends, more therapy/school behaviors on weekdays), seasonal patterns (winter cold cluster → more sick days), occasional bad weeks for realism.

**Where it lives**: `/home/carecomp/secrets/db_backups/dlc_test_family/` (gitignored via `secrets/`). Generates `seed.sql` + `revert.sql`. Wrapper scripts at `scripts/seed_dlc_qa.sh` and `scripts/revert_dlc_qa.sh` with explicit dev/prod env arg.

## Stability audit scope

Six parallel Explore-agent sweeps:

1. **Input validation gaps** — handlers in `internal/handler/{api,web}/` for missing type/length/format/range/required validation, XSS surfaces in chat/notes/ticket bodies, file upload edge cases, date/enum validation
2. **Unhandled errors / panic risks** — `internal/handler/`, `internal/service/`, `internal/repository/`, `internal/middleware/` for ignored errors, type assertions without `,ok`, nil derefs, slice bounds, goroutine panics, integer overflow, division by zero
3. **Network/transient failure handling** — external calls (Stripe, OpenFDA, Anthropic, S3, SES, Redis, RDS, APNs/FCM) for missing timeouts/retries/graceful degradation
4. **Concurrency hazards** — DB read-modify-write without locking, session/cookie races, shared handler state, background worker singleton, Stripe webhook idempotency, account-deletion state machine, ticket-number race
5. **Auth/authz boundary** — IDOR on child/family/log/report/ticket/thread IDs, role-check coverage, JWT edge cases, CSRF, mass assignment, admin route gating post-migration 00032, dev gate bypass
6. **Template + frontend hazards** — template injection / `template.HTML` misuse, JS template literal injection, fetch() without .catch(), error states, back-button traps, Capacitor-specific (target=_blank fixed at R8, mailto/tel, deep links), localStorage quota, polling cleanup, offline behavior

Each agent caps findings at top 25-30, classifies P0/P1/P2 with file:line + repro + recommended fix. Findings consolidated to `docs/qa/2026-05-19-stability-audit-findings.md`.

**Triage gate**: Bryan reviews findings, agrees on the fix list. P0/P1 land before QA engineer starts; P2 logged for post-QA cleanup.

## Fuzz harness (Phase C, post-audit-fix)

Go test package `cmd/qa_fuzz/` exercising dev with:
- Form-field type mismatches (phone in address, alpha in numeric, emoji/RTL/unicode in name)
- Boundary conditions (empty, max-length+1, negative, zero, future date in past-only field)
- Auth fuzzing (expired/malformed/missing JWTs, wrong-family access attempts)
- Concurrency (simultaneous edits to same record from two sessions)
- Network drops (context cancellation mid-request)
- Oversized payloads (1MB note bodies, 10k chat messages)
- File upload edge cases (wrong MIME, oversized, malformed multipart)

Reusable for regression after the QA engineer surfaces bugs we missed.

## Execution sequence

1. ✅ Broaden `.claude/settings.json` permissions (prod RDS psql, pg_dump, python3, pip3, file utils, seed-wrapper script patterns)
2. ✅ `pg_dump` dev + prod → `secrets/db_backups/{dev,prod}_pre_dlc_qa_<ts>.dump`
3. Dispatch 6 audit agents in parallel → findings doc
4. Build `dlc_test_family/gen_seed.py` (fork + adapt Smith pattern)
5. Apply seed to dev → smoke (login all 6, dashboard renders, insights run)
6. **Checkpoint with Bryan** on audit findings → triage P0/P1/P2
7. Fix P0/P1 (variable wall-clock)
8. Apply seed to prod → smoke
9. Build + run fuzz harness, fix what surfaces
10. Write handoff doc for QA engineer (credentials, env URLs, restore recipe, scope notes)

## Risk callouts

- **Scanner load on prod**: ~150k log rows hitting per-window scanners could queue significant work. Plan: disable scanners during bulk insert, trigger one clean post-seed run; OR batch inserts in small transactions.
- **RDS disk**: pre-seed check on free space. Existing prod dumps ~9MB; seed will inflate the DB but not catastrophically.
- **Lock contention**: large inserts wrapped in modestly-sized transactions to avoid blocking real-family writes.
- **AI cost**: prod has `CLAUDE_ENABLED` unset → no LLM calls. Safe.
- **Real Stripe**: DLC family gets a comp'd subscription (status='comped', migration 00024) — no Stripe API touched.

## Resume protocol

If session crashes mid-execution, next session should:
1. Read this spec doc + check `MEMORY.md` for any DLC-QA reference written between sessions.
2. Check `TaskList` for in-flight tasks.
3. Check `/home/carecomp/secrets/db_backups/` for pre_dlc_qa_*.dump presence (confirms Layer 2 backup completed).
4. Check `/home/carecomp/secrets/db_backups/dlc_test_family/` for generator presence.
5. Check `docs/qa/2026-05-19-stability-audit-findings.md` for audit completion.
6. Check `psql ...  -c "SELECT count(*) FROM app_users WHERE email LIKE 'DLC%'"` against dev and prod to determine seed state per env.
7. Pick up at next pending step.
