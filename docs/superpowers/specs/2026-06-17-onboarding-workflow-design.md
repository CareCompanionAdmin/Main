# New-User Onboarding Workflow — Design Spec

**Date:** 2026-06-17
**Status:** Approved design, pending implementation plan
**Author:** Bryan + Claude (brainstorming session)

## 1. Purpose

Give brand-new users a guided, low-friction first-run experience that gets them
to a *usable* app state immediately, then nudges (but never forces) them to
complete the rest of setup. Today a new user is dropped onto an empty dashboard
(or force-redirected to `/family/new`) with no guidance. This adds a clean,
colorful, minimal onboarding flow covering: first child (name, DOB, conditions),
inviting the care team, and basic settings.

**Design north star:** colorful, clean, minimal, effortless to use. Matches the
existing parent-facing "calm" design system (warm palette, soft cards, mascot).

## 2. Gating model — Hybrid

- **Required wizard (blocking):** the minimum needed for the app to be useful —
  a family (usually already created at signup) and a first child. The user must
  finish this before reaching the dashboard.
- **Optional dashboard checklist (non-blocking):** everything else — add another
  child, invite the care team, basic settings — presented as a dismissible
  "Finish setting up" card on the dashboard. Do it anytime; nothing blocks.

## 3. End-to-end flow

1. User registers (existing `/api/auth/register`). As today, an `app_users` row
   is created; if a family name was given, a `families` row + `family_memberships`
   (role=parent) + a 14-day trial are created.
2. On the next request to `/dashboard`, the handler checks
   `app_users.onboarding_completed_at`. If `NULL` → **redirect to `/onboarding`**.
   This supersedes today's `/family/new` forced redirect for new users.
3. `/onboarding` runs the **required JS stepper**:
   - **Welcome** (~10s, expectation-setting, single CTA).
   - **Name your family** — *shown only if the user has no family yet* (covers the
     no-family-at-signup case). Creates the family + starts the trial via the
     existing family-create endpoint.
   - **Your first child** — first name (required), date of birth (required),
     gender (optional), conditions (optional). On finish → `POST /api/children`,
     then stamp `onboarding_completed_at`, redirect to `/dashboard`.
4. The dashboard renders the colorful, dismissible **"Finish setting up"**
   checklist with the optional items. It disappears when dismissed or when all
   items are complete.

## 4. State model

One migration on `app_users`:

- `onboarding_completed_at TIMESTAMPTZ NULL` — set when the required wizard
  finishes. **Single source of truth for the dashboard gate** (`NULL` = run
  onboarding).
- `onboarding_checklist_dismissed_at TIMESTAMPTZ NULL` — set when the user
  dismisses the checklist card, or auto-set when all checklist items are done.
- Per-item checklist completion is **derived from real data** wherever possible
  (see §6), with the "basic settings" item recorded as a flag inside the existing
  `app_users.settings` JSONB (no extra columns).

**Backfill (in the migration):** any user who already has ≥1 child, or who is a
non-creator family member, gets `onboarding_completed_at = now()` so existing
users never see onboarding.

## 5. The required wizard (`/onboarding`)

**Implementation:** Approach A — a single `/onboarding` page rendering a
client-side JS stepper that calls existing JSON APIs (the same fetch/JSON pattern
already used by `new_child.html`). One step visible at a time, slim colorful
progress bar, gentle slide/fade transitions, no full-page reloads, large tap
targets, mascot accent.

**Steps:**

- **Welcome** — warm intro, "let's set up your space in about a minute," single
  "Let's go" button.
- **Name your family** *(conditional — only if no family exists)* — single input
  ("What should we call your family space?"); creates the family + trial.
- **Your first child** — the core screen:
  - First name (required)
  - Date of birth (required; friendly date picker; reuses existing
    "not in the future" validation)
  - Gender (optional select)
  - Conditions (optional): colorful quick-pick chips —
    *Autism Spectrum Disorder, ADHD, Anxiety, Epilepsy/Seizures, Sensory
    Processing Disorder, Speech/Language Delay* — tap to toggle; plus an
    "add your own" field appending a custom chip. Selected chips render as
    removable tags. Stores `condition_name` only; ICD codes / diagnosis dates /
    severity are left for later editing in the child profile.
  - On **Finish**: one `POST /api/children` (the handler already accepts a
    `conditions` array), then `POST /api/onboarding/complete`, then redirect to
    `/dashboard`.

**Mechanics:** inline field validation with kind error states; one network call
per step; on failure the step stays put with a gentle retry message (no data
loss); **Back** preserves entered values.

## 6. The dashboard "Finish setting up" checklist

A colorful card near the top of the dashboard for users who haven't dismissed it
and haven't finished all items. Tiny progress line ("1 of 3 done"); each row
expands inline or opens a light modal; completed rows collapse to a checked
state; a quiet "I'm all set / dismiss" control is always available.

**Items (all reuse existing endpoints):**

1. **Add another child** — opens the same child mini-form (name + DOB + gender +
   conditions) → `POST /api/children`. Derived done when the family has ≥2
   children; always re-openable.
2. **Invite your care team** — embedded repeatable invite form (email + role:
   *Parent/Guardian · Caregiver · Medical Provider*, optional first name) →
   `POST /api/family/members` (existing invite + auto-accept-on-signup), **plus**
   a "Manage everyone in Settings → Members" link. Derived done when ≥1 invite is
   sent or a 2nd member exists.
3. **Basic settings** — inline panel:
   - **Timezone** — pre-filled from the browser, confirm/change.
   - **Notifications** — opt-in toggles (medication reminders · alerts · chat ·
     invites).
   - **Time format** — 12h / 24h.
   - **AI Insights consent** — toggle shown with its required medical/privacy
     disclosure, recorded through the **existing consent-versioning mechanism**
     (version + disclosure SHA) so it is audit-clean.
   - Saves via existing preferences / notification-prefs / AI-consent endpoints.
     Marked done (settings JSONB flag) when saved.

**Lifecycle:** card auto-dismisses (sets `onboarding_checklist_dismissed_at`)
when all three items are done, or immediately on user dismiss. Once gone it stays
gone, but a small **"Set-up guide"** link in Settings can reopen it (dismissing
is not a one-way door).

## 7. Backend touchpoints

**New:**
- `GET /onboarding` — web handler + `templates/onboarding.html` +
  `static/js/onboarding.js`.
- `POST /api/onboarding/complete` — stamps `onboarding_completed_at`.
- `POST /api/onboarding/checklist/dismiss` — stamps
  `onboarding_checklist_dismissed_at`.
- A small state write for the "basic settings done" flag (settings JSONB), either
  its own tiny endpoint or piggybacked on the settings save.
- Migration adding the two timestamp columns + backfill.
- Dashboard checklist partial (`templates/partials/onboarding_checklist.html`)
  included by `dashboard.html`.

**Changed:**
- Dashboard web handler: add the `onboarding_completed_at IS NULL → /onboarding`
  gate; compute children count / member-or-invite count / settings-done flag to
  decide whether to render the checklist partial.

**Reused unchanged:** `POST /api/children` (child + conditions), the family-create
endpoint, `POST /api/family/members`, `PUT /api/users/me/preferences` (timezone,
time format), notification-preferences endpoint, AI-consent endpoint.

## 8. Edge cases

- **Invited members** (joining an existing family): they don't own children, so
  they get a **trimmed onboarding** — Welcome + Basic settings only, no
  "add a child" step — and are marked complete on finish. Detected by: the user
  is not the family creator and/or the family already has children.
- **Abandonment:** the gate is `onboarding_completed_at IS NULL`, so a dropped
  session re-enters cleanly. The wizard is **idempotent** — skips the family step
  if a family already exists; treats onboarding as complete if a child already
  exists.
- **Existing users:** covered by the migration backfill.
- **Multi-tab:** state is server-side; the last write wins; re-entry is safe.
- **Guards preserved:** DOB-not-future validation; free-text condition
  sanitization; AI-consent version + disclosure SHA recorded correctly.

## 9. Testing

- **Automated (targeted):** the gate/redirect logic and the migration backfill —
  this path touches *every* user's dashboard load (highest blast radius), so it
  gets real coverage despite the repo's light test tradition. Plus handler tests
  for the new onboarding endpoints.
- **Manual on dev:** full new-owner run (register → wizard → child+conditions →
  dashboard checklist → invite → settings + AI consent → dismiss); invited-member
  trimmed path; existing-user check (no onboarding shown); abandonment/re-entry.

## 10. Production rollout

This modifies the **shared dashboard redirect path for all users**, so:
- Verify thoroughly on dev first (per dev-first rule).
- Deploy **on its own**, not bundled with unrelated changes.
- Double-check the migration backfill correctly marks all existing users
  complete before any prod deploy.

We are now in production (App Store live), so stability is a priority — keep the
change focused and well-tested.

## 11. Out of scope (deferred)

- Per-condition severity / ICD codes / diagnosis dates in onboarding (handled in
  the child profile later).
- Onboarding analytics/funnel tracking.
- A/B testing of copy or step order.
- Re-onboarding flows for major feature launches.
- Localized onboarding copy (English only for now; `language` already defaults to
  `en`).

## 12. Aesthetic checklist (acceptance)

- Colorful, clean, minimal; one focused action per screen.
- Consistent with the parent-facing calm design system.
- No full-page reloads inside the wizard.
- Effortless: large tap targets, friendly microcopy, kind error states, no
  dead-ends.
