# DLC QA Handoff Guide

**Target QA engineer**: DLC
**Prepared by**: Bryan + Claude
**Date**: 2026-05-19

You have a DLC Test Family seeded on **both** dev and prod environments with a full year of activity. Goal: adversarial stability testing — find unhandled errors, crashes, validation gaps, broken UX. We've already pre-emptively patched what we could spot in a static audit; your job is to find what slipped through.

---

## Environment URLs

### Production (preferred for native app testing)
- **Web**: https://www.mycarecompanion.net
- **iOS native**: TestFlight build `mobile-v1.0.13` (request access from Bryan)
- **Android native**: pre-release APK / Play internal track (request from Bryan)
- **Backed by**: prod RDS

### Development (faster iteration; web only)
- **Web**: https://dev.mycarecompanion.net
- **Dev gate**: browser visits require the `DEV_GATE_CODE` — request from Bryan, lasts 30 days as a cookie
- **Native app bypass**: any browser User-Agent containing `MyCareCompanionApp` skips the gate (Capacitor builds set this automatically)
- **Backed by**: separate dev Postgres on the admin EC2

Both environments share the **same DLC Test Family seed** — same accounts, same children, same year of data. You can switch between them without re-learning the data.

---

## Credentials — six accounts in one family

All share password: **`TestPass1!`**

| Email                          | Role             | Name           | Permissions                          |
| ------------------------------ | ---------------- | -------------- | ------------------------------------ |
| `DLCparent1@test.com`          | parent (primary) | Diana Lawson   | All actions including billing/admin  |
| `DLCparent2@test.com`          | parent           | Marcus Lawson  | All parent actions                   |
| `DLCcaregiver1@test.com`       | caregiver        | Renee Tucker   | Can log + view; cannot modify family |
| `DLCcaregiver2@test.com`       | caregiver        | Owen Bennett   | Can log + view                       |
| `DLCdr1@test.com`              | medical_provider | Priya Shah     | View-only                            |
| `DLCdr2@test.com`              | medical_provider | Felix Moreno   | View-only                            |

**Family**: "DLC Test Family"
**Subscription**: Family plan, comp'd (no real billing involvement)

---

## The data you're working with

### Children
- **Mia Lawson** — age 7, DOB 2018-09-12. Heavier medical profile: ASD Level 2 + ADHD-combined + focal epilepsy + generalized anxiety. Methylphenidate-responsive (planted: headache cluster days 203-210 after a dose increase). Risperidone for behavior (planted: FDA blackbox alert eligibility). Levetiracetam for seizures (planted: 2 seizure events in the winter crisis window). Sensory-defensive.
- **Ethan Lawson** — age 4, DOB 2021-06-30. Milder: ASD Level 1 + SPD. Tree-nut allergy (no anaphylaxis history, EpiPen carried). Verbal. Picky eater.

### Year of data — planted to trigger every insight scanner
- **~2,920 medication_log entries** (4 meds for Mia, 2 for Ethan, twice-daily schedules)
- **~3,224 behavior_logs** (4-6/day per kid, mood/energy/anxiety/meltdowns/triggers/positives)
- **~730 sleep_logs** (1/day per kid)
- **~4,025 diet_logs** (3 meals + 2-3 snacks per day)
- **~978 bowel_logs**
- **~364 sensory_logs**, **~242 social_logs**, **~313 therapy_logs** (ABA + Speech + OT)
- **~106 weight_logs**, **~106 speech_logs** (weekly)
- **2 seizure_logs** for Mia (winter crisis window — should trigger frequency alert)
- **~18 health_event_logs** (cold, ear infection, GI bug, flu-like, plus Mia's headache cluster)
- **~18,346 chat_messages** across 3 threads (Daily Care, Medical Team, School + Therapy)
- **3 support_tickets** (bug + feature requests)

### Phases planted across the year
The seeded data shifts through distinct phases so the AI scanners light up:
1. **Days 1-60** — baseline (steady, ~95% med adherence)
2. **Days 61-90** — autumn dip (mood drift, sleep dip)
3. **Days 91-150** — winter crisis (low adherence, multiple meltdown days, 2 seizures for Mia, weight dip)
4. **Days 151-180** — recovery
5. **Days 181-220** — med change window (Mia's methylphenidate up at day 200, headache cluster days 203-210)
6. **Days 221-280** — summer break (different rhythms)
7. **Days 281-320** — back-to-school (sensory uptick, transition stress)
8. **Days 321-365** — steady

Expect insights, alerts, and pattern detections to be plentiful — they're designed to fire.

---

## What we already know (don't waste your time finding these)

We ran a static audit and pre-emptively fixed ~78 P0+P1 issues. The remaining ~45 P2 items + a small handful of intentionally-deferred items live in `docs/qa/2026-05-19-stability-audit-findings.md` (request access). Skip these classes if you encounter them:

- **CSRF protection on web routes** — known gap, deferred to its own slice
- **Login endpoint rate-limiting** — known, deferred
- **Password reset enumeration timing** — known
- **iOS-specific localStorage quota nuance** — known
- **Migration runner double-run on simultaneous ASG boot** — known
- **App Store Connect JWT expiry mid-request** — known, low-traffic surface

If you find ONE OF THESE, log briefly but skip detailed repro.

---

## Restore + reset mechanics

If you delete data or want to start fresh:

### Layer 1 — Surgical reset (your day-to-day reset)
Wipes only the DLC family + its 6 users + their tickets. Doesn't touch anything else.

```bash
# Dev
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
  -f /home/carecomp/secrets/db_backups/dlc_test_family/revert.sql

# Reapply seed
PGPASSWORD=carecompanion psql -h localhost -U carecompanion -d carecompanion \
  -f /home/carecomp/secrets/db_backups/dlc_test_family/seed.sql
```

For prod, same SQL files with prod connection details (request from Bryan; password lives in AWS Secrets Manager).

The seed is **idempotent** — re-running just re-asserts the data without duplicate rows. Restore takes ~10s on dev, ~30-60s on prod.

### Layer 2 — Full-DB pg_restore (the "things went really weird" safety net)
We took pre-test pg_dumps before applying the DLC seed:
- `/home/carecomp/secrets/db_backups/dev_pre_dlc_qa_20260519T042014Z.dump` (2.9MB)
- `/home/carecomp/secrets/db_backups/prod_pre_dlc_qa_20260519T042014Z.dump` (9.2MB)

pg_restore-able to whole-DB state. Don't use this unless Layer 1 isn't enough — it wipes whatever else is in the DB at that time.

---

## What to focus on

We're especially interested in:

1. **Input validation gaps we missed** — try every form field with mismatched type (phone in address, alpha in numeric, emoji/RTL/unicode in name), oversized payloads (1MB notes, 10k-char chat), boundary values
2. **Network failure handling** — kill Wi-Fi mid-submit, switch from Wi-Fi to cellular, slow network throttle in DevTools, server reboot mid-request
3. **Concurrency / multi-tab** — open the same form in two tabs, race submissions; have two users edit the same record simultaneously
4. **Multi-user scenarios** — one user removes another mid-session; family invitation accept-twice; role change while a session is active
5. **Mobile-specific** — background → foreground transitions, low-memory iOS warnings, deep-link / push notification handling, biometric auth (when added), camera permission denied
6. **Stripe + subscription flows** — note: prod Stripe is still in TEST mode (see open issues). DON'T submit real-money flows on prod yet.
7. **Account deletion flow** — exercise the OTP, the 14-day restore window, edge cases (delete the primary parent, etc.)
8. **Report PDF generation** — long date ranges, edge dates, special characters in titles
9. **AI Insights** — verify the planted patterns actually trigger insights/alerts. Note: Phase 5 (Bedrock + AI narrative) is not yet shipped, so the "narrative analysis" toggle is hidden.

---

## What's actively off-limits

- **Prod Stripe LIVE flow** — prod is in TEST mode, but DON'T attempt real-money purchases until Bryan confirms the LIVE flip
- **Other real families on prod** — the DLC family is yours; don't touch any others
- **Admin portal on prod** — request specific admin credentials from Bryan if you need admin testing; don't poke around with the DLC accounts (they don't have admin)
- **Mobile app keystore / signing** — out of scope

---

## How to report

For each issue found:
1. **Title** (concise: "[Bug] Chat message accepts 1MB unicode payload, locks UI")
2. **Environment**: dev / prod / both
3. **Account used**: which DLC account
4. **Steps to reproduce**: numbered, deterministic
5. **Expected vs Actual**
6. **Severity**: P0 (crash/data loss/security) / P1 (broken feature) / P2 (polish)
7. **Logs / screenshots** if helpful

Submit via the in-app support ticket form (Settings → Support) — that's also a feature you're testing. Or directly to Bryan if the ticket form itself is the bug.

---

## Quick reference

| Thing | Where |
| ----- | ----- |
| App | https://www.mycarecompanion.net |
| Dev | https://dev.mycarecompanion.net |
| Privacy policy | https://www.mycarecompanion.net/privacy |
| Terms | https://www.mycarecompanion.net/terms |
| Support email | support@mycarecompanion.net |
| Status | (none yet — known TODO) |

Happy testing. We made the data weird on purpose.
