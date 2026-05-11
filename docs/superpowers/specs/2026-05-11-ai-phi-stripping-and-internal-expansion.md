# AI Privacy Hardening, Internal Analysis Expansion, and Bedrock Migration

**Date:** 2026-05-11
**Owner:** Bryan
**Driver:** App Store Risk R6 (AI/pattern-detection language softening) — escalated to a structural privacy initiative after audit
**Status:** Plan approved by Bryan 2026-05-11; execution starting Phase 1 same day

---

## Executive Summary

What started as a small App-Store-friendly copy-softening item (R6: replace "AI-powered" with "pattern analysis") turned into a structural privacy and capability initiative when we audited the AI Insights service and discovered the application is currently sending **identified PHI about minors** to a third-party LLM (Anthropic's API) **without user consent, without disclosure in the privacy policy, and without a Business Associate Agreement.**

Rather than disable the AI feature (which Bryan rejected because the Steinmetz family is actively using it for care), the plan is to:

1. **Make the existing Anthropic call safe by stripping all PHI before send** — same Claude, same insights, same family experience, but de-identified inputs.
2. **Massively expand internal (on-server, deterministic) analysis** so the LLM is a polish layer rather than the analytical core.
3. **Build opt-in consent for the few features that genuinely require sending free-text** (e.g., narrative interpretation of parent notes), defaulted OFF.
4. **Migrate to AWS Bedrock at the end** so all of this falls under the AWS BAA.

This document is meant to be **resume-safe** — if this session crashes or compacts, the next session can pick up cold from this file alone. It is intentionally verbose because Bryan asked for that.

---

## Why this exists — full context for resume-safety

### What the application is

MyCareCompanion is a Go web app + Capacitor mobile shell used by families caring for children with autism spectrum disorder and related conditions. Parents log behaviors (mood, energy, anxiety, meltdowns, stimming, aggression), sleep (hours, quality, wakings, nightmares, bedwetting), medications (names, dosages, frequencies, adherence), diet, sensory issues, social events, therapy, bowel movements, seizures, health events, and speech. The app surfaces "insights" — patterns it notices across these streams — to help parents organize and correlate care data for themselves, their caregivers, and their clinicians.

**Bryan's mission statement, in his words (2026-05-11):**

> "Our goal is not to be providing medical diagnostics or to be a replacement for thoughtful and talented medical professionals. Instead our goal is simply to give a family the ability to put in an astronomical amount of work organizing, checking, researching and correlating both their own experiences and the latest available and relevant at a level that would be otherwise impossible to help the ACTUAL care taker (usually parents), caregivers and medical professionals provide the best level of care possible."

This is the framing that drives every design decision in this initiative. We are a care-organization and correlation tool. We do not diagnose. We do not predict. We do not recommend treatment. We surface patterns; humans (parents + clinicians) decide what they mean.

### Why this came up now

The App Store approval initiative (separate document: `memory/project_carecompanion_app_store_approval.md`) flagged R6 as a strong-risk item — Apple's medical-app reviewers scrutinize "AI" claims heavily under guideline 1.4.1, and 5.1.2(i) requires explicit consent before sharing PHI with third parties *including third-party AI*.

When auditing R6, I expected to find marketing-copy issues only. Instead I found that `internal/service/ai_insight_service.go` is making live calls to `api.anthropic.com/v1/messages` containing, for each child:

- Real first name
- Age in years + months (computed from DOB)
- Gender
- Diagnosed conditions with ICD codes and severity
- Active medications with dosages and frequencies
- 7–30 days of behavior logs (mood/energy/anxiety/meltdown/stimming/aggression counts)
- **Free-text parent notes truncated to 100 chars**
- Sleep logs (hours, quality, wakings, nightmares, bedwetting flags)
- Medication adherence including names of missed medications
- Diet, sensory, social, therapy, bowel, seizure, health-event, and speech logs with descriptions

This is unambiguously PHI about a minor with a diagnosed condition, going to a third-party processor with no user consent, no privacy-policy disclosure, no BAA, and no de-identification. The internal prompts to Claude already correctly say "do NOT diagnose, recommend consulting a healthcare provider" — but the inputs themselves are the problem.

### Why we can't simply disable the feature

Bryan's response when offered the "disable until post-approval" option (2026-05-11):

> "The Steinmetz family is actively using this application to support the care of their children. I can't just turn stuff off that will have an effect on their care in the interest of getting Apple approval."

The patient comes first. The fix is at the data layer (de-identify), not at the feature level (disable).

---

## Bryan's research questions and the answers we landed on

Bryan asked five specific questions before approving this plan. Capturing them here verbatim because they shape the architecture.

### Q1: Does our messaging cross into diagnostic territory?

**Audit finding:** The mission statement is fine. The internal prompts to Claude are fine ("do NOT diagnose"). The internal `/insights` page is fine ("Patterns we notice"). But landing.html and app-store-metadata.md have several phrases that imply diagnostic or treatment-recommendation capability:

- `landing.html:245` — "Validate treatment insights"
- `landing.html:318` — "Smart Medication"
- `landing.html:468` — "Real AI that understands your child"
- `landing.html:501, 524` — "AI pattern detection"
- `landing.html:546` — "Validate treatment insights - Confirm or refute pattern hypotheses with clinical expertise"
- `landing.html:614` — "The more you log, the smarter it gets"
- `app-store-metadata.md:22-23` — "SMART INSIGHTS — AI-powered daily insights"

These get rewritten in Phase 1 to align with the mission framing (patterns we surface, you decide). The disclaimer on `app-store-metadata.md:64` ("does not diagnose, treat, or claim to prevent any condition...") stays exactly as is — it's already correct.

### Q2: Could we move analytical work internal and skip outside calls?

**Audit finding:** The current internal insight generator (`internal/service/insight_generator.go`, 378 lines) is genuinely anemic — five hardcoded checks (mood trend, sleep deficit, meltdown frequency, medication adherence, missed-med streak), all simple threshold rules. There's zero correlation analysis, no baselines, no anomaly detection, no cross-stream insight.

**What "several orders of magnitude more" looks like internally:**

- **Tier A — Statistical analysis (deterministic):** Rolling personal baselines + z-score outlier flags. Cross-stream Pearson/Spearman correlation (sleep ↔ next-day mood, diet ↔ behavior, medication-timing ↔ alertness). Cyclical pattern detection via autocorrelation. Trend-slope significance testing. Time-of-day pattern detection. Seasonal decomposition for longer-running children.
- **Tier B — Clinical-rule engine:** Medication side-effect tracking using existing `drug_database.go`. Drug-drug interaction surfacing (already coded in `drug_database.go` — see `getDrugClass()` + `knownInteractions`). Age-band developmental milestones. Known autism-comorbidity patterns. Symptom-cluster detection.
- **Tier C — Privacy-preserving cohort comparison:** Nightly aggregate rollup keyed on age-band × diagnosis × medication-class. "Children with similar profiles typically..." stats. K-anonymity enforcement (minimum cohort size = 5 or 10 to prevent re-identification).

**Compute / cost impact at our scale:** Most stats are O(N log N) over user-own data. ~100ms per child for the full Tier A pass. Cohort rollup is a nightly job. Current infra (single t3.medium-class EC2 + db.t3.medium RDS) easily handles this for 10K+ daily-active users. Estimated infra delta: ~$100/mo at 10K users, **less than current Anthropic spend at that scale.**

**Decision: build the internal expansion (Phase 2 below).**

### Q3: How do we get a BAA?

Three paths, in order of preference:

| Path | Status | Cost | Effort |
|---|---|---|---|
| AWS Bedrock under AWS BAA | Available as of 2026-02-10; Claude is a HIPAA-eligible model on Bedrock | Free; on-demand token pricing matches Anthropic-direct | ~50-100 LOC swap, single IAM permission grant, one-time BAA click-through in AWS Artifact |
| Anthropic direct BAA | Available via Claude Enterprise plan, click-to-accept since 2025-12-02 | Enterprise contract pricing (opaque, custom $) | Quote + legal cycle |
| No LLM | N/A | N/A | Eliminate the issue rather than mitigate it |

**Decision: Bedrock.** We're already on AWS, the BAA is free, the model is the same Claude Sonnet, and the cost is functionally identical to Anthropic-direct.

### Q4: How consequential is the LLM call to user value?

**Honest assessment:** ~70-80% of the substance of current Claude insights is replicable in deterministic code (numerical correlation, trend detection, threshold alerts, adherence percentages, outlier flags, sorting). The 20-30% that's NOT replicable is:

- **Free-text note interpretation** — parents sometimes write "had a birthday party yesterday and stayed up late at grandma's" which explains a numerical anomaly. Pure statistics miss this.
- **Plain-language synthesis** — turning numerical patterns into warm parent-facing prose.
- **Novel cross-domain pattern recognition** — occasionally finding links predefined rules miss.

After Phase 2 (internal expansion) and Phase 1 (PHI stripping), Claude's role becomes: **narrative polish on top of internal statistics**, optionally enhanced with free-text interpretation IF the parent has opted in.

### Q5: How does accuracy suffer if we strip ALL PHI?

**Accuracy impact, by insight type:**

| Insight type | Loss when PHI stripped |
|---|---|
| Numerical trend detection (mood declining) | None |
| Cross-stream correlation (sleep ↔ mood) | None |
| Medication adherence tracking | None — we track on-time/missed events, not the drug name |
| Threshold alerts (3+ meltdowns) | None |
| Specific medication side-effect warnings | Moderate — replace specific drug name with drug class via `getDrugClass()` (still actionable but generic) |
| Drug-drug interaction surfacing | **None** — we do this entirely internally now via `drug_database.go`; results can be sent to Claude as already-detected facts |
| Free-text note interpretation | Total loss (notes removed entirely) UNLESS user opted into narrative analysis |
| Narrative warmth of output | Moderate — Claude can still write personable copy referring to "the child" |

**Key realization:** Once PHI is stripped, the LLM is doing pure statistical-narrative work on anonymous numerical data. At that point you have to ask what the LLM is adding that good template-driven output on top of strong internal analytics can't. Answer: not much, except for the opt-in free-text path.

---

## Architectural Decisions (with rationale)

### Decision 1: Keep the Anthropic LLM call (don't disable for App Store)

Reason: Steinmetz family is depending on it for care. Disabling = patient harm. The fix is at the data layer.

### Decision 2: Stay on Anthropic direct (api.anthropic.com) through Phases 1-4, swap to Bedrock in Phase 5

Reason: Bryan's preference (2026-05-11):
> "Move the AWS BAA part to the final stage so everything is built as it is now (even if our documentation or whatever is technically untrue) then switch once everything is stable."

Important nuance: this works because **after Phase 1 (PHI stripping), the data flowing to Anthropic is no longer PHI** — so the BAA isn't legally required during Phases 2-4. The BAA becomes load-bearing only when the opt-in narrative path (with free-text) is enabled in production, which is gated to Phase 5.

**Critical:** the Privacy Policy / App Store privacy nutrition label MUST NOT be updated to claim "we use AWS Bedrock under a BAA" until both are actually true. Documentation updates are sequenced into Phase 5 *after* BAA signing. Until then, the Privacy Policy stays as is.

### Decision 3: All PHI stripping happens at one boundary — the `callClaude()` function

Reason: Single point of enforcement = easier to audit, harder to leak. Internal services continue to work with real PHI; only the outbound HTTP call sees de-identified data.

### Decision 4: De-identification uses HIPAA Safe Harbor as the floor

Strip the 18 HIPAA identifiers:
- Names → "the child" or "C"
- Dates (DOB, log dates) → relative/banded (age band like "5-7y", date offsets like "Day -3")
- Geographic identifiers (none currently sent)
- Account/medical record numbers (UUIDs) → rotating SHA-256 hash with weekly server-side salt
- Telephone, email, etc. (none currently sent)

Plus an extra layer for re-identification risk specific to our small user base:
- Drug names → drug classes via `getDrugClass()` from existing `drug_database.go`
- ICD codes → general category (e.g., "ASD" instead of "F84.0")
- Free-text notes → REMOVED entirely (no opt-in: removed; opt-in: included)
- Health-event descriptions → REMOVED entirely (no opt-in: removed; opt-in: included)
- Rotating child→token hash weekly: same child appears as different opaque ID across weeks, preventing cross-call pattern stitching

### Decision 5: Massively expand internal analysis

Build Tier A (statistical) + Tier B (clinical-rule) + Tier C (cohort) internally. The Steinmetz family should see *more* insights, not fewer, after this work. The LLM becomes a polish layer on top of strong internal analytics.

### Decision 6: Opt-in narrative analysis defaults OFF, with clear disclosure

For features that genuinely benefit from free-text being sent to Claude (e.g., interpreting parent narrative notes), build an opt-in toggle in Settings with a clear disclosure modal. Default OFF for all users (including grandfathered existing users). When OFF, the system still works — it just doesn't get free-text interpretation. When ON, parents explicitly consent to their narrative content being processed by AWS Bedrock under the AWS BAA.

### Decision 7: Keep JB2S Enterprises, LLC out of brand-facing surfaces

Per Bryan's preference (2026-05-11) — JB2S is a holding LLC and the plan is to spin MyCareCompanion out into its own dedicated LLC post-beta. The legal entity is named only in Terms preamble + Privacy preamble (legal disclaimers). NOT in Contact Us, footers, marketing copy, app metadata, or anywhere else. This was settled in commit `9b1e277` (2026-05-11).

### Decision 8: Stay on current AWS infrastructure for now

Bryan's direction (2026-05-11):
> "Keep the current server infrastructure since we only have a few users but make sure that all changes are made with the understanding that we will be upgrading very soon which probably means a little better capacity monitoring in the admin section."

Implementation: Phase 4 adds capacity-monitoring widgets to the admin dashboard (EC2/RDS/Bedrock-spend/insights-per-day/active-users + headroom indicators) so we know when to upgrade. Code changes are designed to scale with infra; no premature optimization.

### Decision 9: Expand Phase 2 to include open-ended internal pattern discovery

Bryan's question, mid-session (2026-05-11):
> "Intelligent analysis of this sort of data without a static list was the kind of thing I would assume was being handled by the AI calls, but shouldn't it also be able to be handled by our internal AI engine since one of the primary changes being accomplished here is a drastic increase in on-box or in-app AI reasoning capability?"

The original Phase 2 plan was scoped to "predefined factor pairs" (e.g., sleep_quality → next-day mood) — a manageable starter but not what Bryan was imagining when he agreed to "internal AI expansion." Bryan correctly intuited that **statistics CAN do open-ended pattern discovery without predefined hypotheses** — that's what exhaustive correlation, anomaly detection, trend detection, and change-point detection are.

Approved expanded scope (2026-05-11):
1. **Exhaustive auto-correlation** across all factor pairs × 4 lag windows, with Benjamini-Hochberg FDR correction across all ~hundreds of hypotheses tested. Surviving pairs become FamilyPattern records. No "what should we look for?" needed — the data tells us.
2. **Per-metric scanners**: z-score anomaly (vs rolling baseline), linear-regression trend slope (with p-value + effect size gate), change-point detection (pooled-stderr split-mean scan).
3. **Clinical-rule layer with 3 sources**: (a) FDA-auto pulled live from openFDA via existing `drug_database.go`, (b) admin-curated rules (DEFERRED to follow-up), (c) medication-start ↔ recent change-point co-occurrence flagging.

Scope cost: extended from ~1.5 days to ~3.5 days of new code. Bryan: "go with the expanded scope for sure."

### Decision 10: JB2S Enterprises, LLC named ONLY in legal-disclaimer surfaces

Bryan's preference (2026-05-11):
> "I don't really want the JB2S mentioned on the contact us page or anywhere else in the app or on the webpage other than legal disclaimers etc. The reason why is that as soon as we get most of the way through our beta program and this whole thing passes the 'is this a real business or not' sniff test, we're going to re-incorporate it into its own entity so it's not under the JB2S umbrella any more."

Implementation: `templates/terms.html` preamble and `templates/privacy.html` preamble name JB2S Enterprises, LLC d/b/a MyCareCompanion (commit `9b1e277`). Contact Us section was reverted to plain "MyCareCompanion" the same session. No JB2S references in marketing copy, footers, app metadata, or anywhere else. When the post-beta spinout happens, only those two preambles need to change.

### Decision 11: Don't separate to a new dev server for Bedrock — only for actual isolation reasons later

Bryan's question (2026-05-11):
> "I use the server that we are having this session on as a central dev server for several other projects that are not related to MyCareCompanion. Am I correct to assume that once we move to Bedrock this server will not have access to those non-Bedrock resources and that I should probably spin up a new central dev server for everything not related to MyCareCompanion?"

Conclusion: The Bedrock migration does NOT restrict this server. Bedrock is just another AWS API; the AWS BAA is a contractual document at the account level, not a firewall. Non-CareCompanion projects can keep running on this box.

However, separate isolation IS recommended over time, for non-Bedrock reasons:
- **Blast radius** — PHI of children with autism + side projects on the same box is a risk multiplier
- **AWS BAA best practice** — designate a dedicated HIPAA account under Organizations
- **Compliance audit ease** — clean story for future enterprise customers
- **Cost allocation** — track CareCompanion's true unit economics

Three-stage plan:
1. **Now**: Sign BAA, switch endpoint, keep operating. No infra changes.
2. **3-6 months**: Spin up a separate small EC2/Lightsail for non-CareCompanion projects. This box becomes CareCompanion-only.
3. **At ~1,000 users or first enterprise pilot**: Move CareCompanion to its own AWS account under Organizations. Other projects in sibling accounts with no BAA exposure.

Note for Phase 5: the IAM policy granting `bedrock-runtime:InvokeModel` will be scoped to the CareCompanion EC2 instance role (the auto-scaling group's role), not to the dev server's user role. So Bedrock permission is properly fenced at the IAM level regardless of host-level isolation.

---

## The Five-Phase Plan

### Phase 1 — PHI stripping at the Anthropic boundary + diagnostic-language copy fixes

**Goal:** Make the existing Anthropic call safe. Steinmetz family sees no change in insight quality; the data flowing out becomes HIPAA Safe Harbor de-identified.

**Files to change:**
- `internal/service/ai_insight_service.go` — new `buildDeIdentifiedProfileContext()` and `buildDeIdentifiedLogContext()` functions; existing `buildProfileContext()`/`buildLogContext()` get a deprecation comment and are routed only to internal storage, not Anthropic.
- New file: `internal/service/ai_phi_stripper.go` — single-point-of-truth for all PHI removal logic.
- New file: `internal/service/ai_phi_stripper_test.go` — comprehensive tests for the stripper.
- `templates/landing.html` — Q1 copy fixes (7 lines).
- `infrastructure/app-store-metadata.md` — Q1 copy fix.

**Schema changes:** None required for Phase 1. The rotating child→token hash uses application-state (in-memory + periodic refresh from a salt in env var or AWS Secrets Manager).

**Tests:**
- Unit tests on the stripper: ensure first names stripped, age banded, drug names mapped to classes, ICD codes generalized, free-text removed, UUIDs rotated.
- Golden file test: snapshot of the actual payload sent to Claude — assert no PHI patterns leak.
- Integration test: call the stripper with a full child profile and verify the round-trip is safe.

**Verification:**
- Run dev with a real Steinmetz-like child profile, trigger AI analysis, log the de-identified payload sent.
- Diff the de-identified payload against the old payload to confirm:
  - First name not present
  - DOB not present (only age band)
  - No drug names (only classes)
  - No free-text notes
  - No raw UUIDs
  - Numerical data preserved
- Confirm Claude still returns useful insights from the de-identified input.

**Commit:** `feat(ai-insights): strip all PHI from Anthropic API calls`

**Acceptance:** No identifying data leaves the server in any `callClaude()` invocation.

**Estimated effort:** 1-1.5 days.

---

### Phase 2 — Internal AI expansion (Tier A + B + C)

**Goal:** Build the deterministic internal-analysis layer so the LLM is a polish layer, not the analytical core. Steinmetz family sees *more* insights, generated more frequently, with better statistical rigor.

**Files to change:**
- New: `internal/service/insight_statistics.go` — `RollingBaseline`, `ZScoreOutlier`, `PearsonCorrelation`, `SpearmanCorrelation`, `LinearRegressionSlope`, `Autocorrelation`, `EWMA` helpers.
- New: `internal/service/insight_cross_stream.go` — analyzes correlations across log types (sleep ↔ mood, diet ↔ behavior, etc.).
- New: `internal/service/insight_clinical_rules.go` — rule engine for medication side effects, drug interactions, age-band milestones, autism comorbidity patterns. Hooks into existing `drug_database.go`.
- New: `internal/service/insight_cohort_aggregator.go` — nightly job that builds aggregate stats from all logs (k-anonymity enforced).
- New tables (migration `00035_insight_cohort_aggregates.sql`):
  - `cohort_aggregates` — keyed on (age_band, primary_diagnosis_category, medication_class_set, log_type, metric); columns for count, mean, stddev, percentiles, last_computed_at.
  - `cohort_index` — bookkeeping for which child contributed when (for invalidation, not for re-identification).
- Updates to `insight_generator.go` — wire in the new analyzers; produce additional insight types.

**Tests:**
- Unit tests for each statistical helper (compare against expected values from known inputs).
- Integration test: synthetic child with known patterns → assert specific insights are produced.
- K-anonymity test: ensure cohort queries with N<5 return null, not the small-cohort stat.

**Verification:**
- Deploy to dev. Run AI analysis. Compare insight count before/after — should be substantially higher.
- Quality check by Bryan: do the new insights make sense for the Steinmetz family?

**Commit:** `feat(insights): expand internal analysis — statistical + clinical-rule + cohort tiers`

**Acceptance:** Insight generator produces at least 3-5x more insights per child per day; cohort comparisons respect k-anonymity.

**Estimated effort:** 2-4 days.

---

### Phase 3 — Opt-in narrative consent flow

**Goal:** Build the consent infrastructure for the few features that genuinely require free-text being sent to Claude. **Built in code but kept disabled in production** until Phase 5.

**Files to change:**
- New migration `00036_ai_narrative_consent.sql`:
  - `app_users` adds columns: `ai_narrative_consent_enabled bool default false`, `ai_narrative_consent_at timestamptz`, `ai_narrative_consent_version int`, `ai_narrative_consent_disclosure_text_hash text` (records exactly which disclosure they accepted).
  - New table `ai_narrative_consent_audit` for tracking enable/disable events with timestamp, user, version, IP.
- `internal/repository/user_repository.go` — add accessors + audit-write methods.
- `internal/service/ai_insight_service.go` — `callClaude()` checks consent state of the *family primary* before including free-text. Default path remains free-text-stripped.
- `templates/settings.html` — new "AI Narrative Analysis" section with toggle + disclosure modal explaining exactly what gets sent.
- New: `templates/partials/ai_narrative_disclosure.html` — the disclosure text, version-controlled so we can detect when re-consent is needed.

**Feature flag:** A server-side env var `AI_NARRATIVE_OPT_IN_AVAILABLE` (default false) controls whether the Settings toggle is visible to users. Stays false in prod through Phase 4; flipped to true in Phase 5.

**Tests:**
- Unit: consent gate denies free-text inclusion when consent is off.
- Unit: consent revocation logs the audit event correctly.
- E2E: enable in dev, walk through Settings → toggle → modal → submit → verify DB state.

**Acceptance:** Feature exists in code, off by default for all users, gated by `AI_NARRATIVE_OPT_IN_AVAILABLE`.

**Estimated effort:** 1 day.

---

### Phase 4 — Admin capacity monitoring

**Goal:** Add monitoring so we know when to upgrade infra. Per Bryan's direction, this is preparation for the upgrade, not the upgrade itself.

**Files to change:**
- New: `internal/handler/admin/capacity_handler.go` — fetches and renders metrics.
- New: `templates/admin/capacity.html` — dashboard view with:
  - EC2 CPU + memory (CloudWatch, 24h rolling)
  - RDS connections + CPU (CloudWatch)
  - ElastiCache memory usage
  - Insights generated /24h (DB count query)
  - Active users /24h (sessions table query)
  - LLM API spend rolling 7-day (CloudWatch metric or estimated from logs)
  - Headroom indicators with thresholds (green ≤70%, amber ≤85%, red >85%)
- New route in admin handler.
- New nav item in admin layout.

**Tests:**
- Unit test on the metric-aggregation functions.
- Manual: load the admin page, confirm widgets populate.

**Acceptance:** Bryan can answer "are we close to needing to upgrade?" by looking at one page.

**Estimated effort:** 0.5 day.

---

### Phase 5 — Bedrock migration + AWS BAA signing + final docs + ship

**Goal:** Switch the LLM endpoint to AWS Bedrock under the AWS BAA, enable the opt-in narrative feature in production, update privacy docs to reflect the new architecture, ship to prod, and submit to App Store.

**Files to change:**
- `go.mod` — add `github.com/aws/aws-sdk-go-v2/service/bedrockruntime`.
- `internal/service/ai_insight_service.go` — replace HTTPS POST to `api.anthropic.com` with `bedrockruntime.InvokeModel` call. Same prompt, same response parsing.
- `internal/config/config.go` — `ClaudeConfig` → `BedrockConfig`; model ID format changes (Bedrock IDs like `anthropic.claude-sonnet-4-5-20241022-v1:0`).
- IAM policy update — EC2 instance role needs `bedrock-runtime:InvokeModel` permission. **This requires Bryan's explicit approval (Cat 1 privilege grant).**
- `templates/privacy.html` — new section disclosing AWS Bedrock as processor of de-identified data, BAA in place, opt-in narrative requires explicit consent.
- `infrastructure/app-store-metadata.md` — privacy nutrition label questions answered to match.
- Production environment — flip `AI_NARRATIVE_OPT_IN_AVAILABLE=true`.

**Bryan's actions (manual):**
1. Sign AWS BAA in AWS Artifact (free, ~5 minutes).
2. Enable Claude model access in AWS Bedrock console (one-time grant per region; us-east-1).
3. Confirm IAM policy change before I apply it.

**Tests:**
- Bedrock smoke test on dev: invoke Claude via Bedrock SDK, confirm same response shape.
- End-to-end on dev: full AI analysis flow via Bedrock, verify insights produced.
- Cost verification: compare a single Bedrock call cost against Anthropic-direct (should match).

**Acceptance:**
- All LLM traffic flows through Bedrock.
- AWS BAA is signed and in force.
- Privacy Policy and App Store privacy nutrition label accurately describe the architecture.
- Bryan explicitly approves prod deploy with the word "ship" / "deploy" / "prod".

**Estimated effort:** 1 day code + admin actions + testing.

---

### Submission

After Phase 5 prod ship + 1-2 day soak period:
- New `mobile-v*` git tag → Codemagic build → internal QA pass on TestFlight.
- App Store Connect submission with updated metadata and privacy nutrition label.
- Address any Resolution Center feedback within 24h.

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Small user base means even "de-identified" data is re-identifiable through call-pattern stitching | High | Moderate | Rotate child→token hash weekly with server-side salt; same child appears as different opaque ID across weeks |
| Claude on Bedrock lags Anthropic-direct on new model availability | Moderate | Low | We use Sonnet 4.5 which has been on Bedrock for months; verify model availability in us-east-1 before Phase 5 |
| Consent revocation semantics — what happens to insights already generated from opt-in data | Certain | Low | Leave already-produced insights in place; log consent state at insight generation time for audit trail |
| Cohort comparison reveals small-population data | Low if mitigated | High if not | Enforce k-anonymity floor of 5 (or 10) before surfacing any cohort stat |
| App Store reviewer asks for BAA documentation | Possible | Low | After Phase 5 we *have* the BAA; provide BAA reference in privacy nutrition label answers |
| Bedrock unavailability causes insight outage | Low | Moderate | Fall back to internal-only insights if Bedrock call fails; user sees no error, just slightly less narrative polish |
| Cost surprise from Bedrock pricing | Very Low | Low | On-demand pricing matches Anthropic-direct; CloudWatch alarms on Bedrock spend if it ever exceeds $50/day |
| Phase 2 internal expansion produces too many or too noisy insights | Possible | Moderate | Apply confidence thresholds; cap insights-per-day; bake in user feedback signals (insight dismissal rate) to tune |
| Schema migration 00035 fails on prod | Very Low | Low | Migration runner has rollback path; tested first on dev |

---

## Files to Read When Resuming This Work

If session crashes or compacts, read these in order:

1. **This file** — top to bottom, all context.
2. `memory/project_carecompanion_app_store_approval.md` — broader App Store initiative state.
3. `internal/service/ai_insight_service.go` — current LLM call code, look for `callClaude()`.
4. `internal/service/insight_generator.go` — current internal generator (5 rules, the "anemic" baseline).
5. `internal/service/drug_database.go` — has `getDrugClass()` (line 454) used by Phase 1 de-identification.
6. `internal/config/config.go` — look for `ClaudeConfig` struct.
7. `git log --oneline -20` — recent commits.
8. Check what phase we're on by examining the progress tracker at the bottom of this file.

---

## Permissions / Authorization State (recorded 2026-05-11)

Bryan has pre-approved (no need to re-ask) for all of Phases 1-4:
- Code edits to all `internal/`, `templates/`, `migrations/`, `infrastructure/` files
- New file creation in those trees
- New tables, new migrations, applied to dev DB
- Go builds, tests, vet
- Dev systemctl restart/reload
- Git operations including push to origin
- AWS read-only describe/list/get for monitoring (already in settings.json)
- AWS Bedrock list/get/describe (newly added)
- AWS Bedrock-Runtime InvokeModel (newly added — for Phase 5 testing)

Bryan must explicitly approve (still requires asking):
- IAM policy changes (Cat 1 privilege grant) — Phase 5 Bedrock permission grant
- Prod deploy via `./scripts/deploy.sh` (Cat 5 dev-first HARD RULE) — needs the word "ship"/"deploy"/"prod"
- AWS BAA signing (Bryan does this manually in AWS Artifact)
- TestFlight tag and Codemagic build (Bryan controls release cadence)
- App Store Connect submission (Bryan controls)

The `.claude/settings.json` has been updated to reduce permission-prompt friction for routine work in Phases 1-4. The patterns added are narrow and specific — no arbitrary-code-execution wildcards.

---

## Progress Tracker (live state — update as work completes)

### Phase 1 — PHI stripping at the Anthropic boundary
- [ ] Create `internal/service/ai_phi_stripper.go`
- [ ] Write tests in `internal/service/ai_phi_stripper_test.go`
- [ ] Rewire `ai_insight_service.go` to use the stripper before all Claude calls
- [ ] Add rotating child→token hash (weekly salt)
- [ ] Map drug names to classes via `getDrugClass()`
- [ ] Generalize ICD codes to category labels
- [ ] Drop free-text notes from outbound payload (default path)
- [ ] Run AI analysis on dev with a real child profile; capture before/after payload diff
- [ ] Fix Q1 marketing copy in landing.html (7 lines)
- [ ] Fix Q1 marketing copy in app-store-metadata.md (1 section)
- [ ] Commit
- [ ] Verify dev still produces useful Claude insights

### Phase 2 — Internal AI expansion
**Scope expanded 2026-05-11** after Bryan's "shouldn't internal be able to do open-ended analysis?" question. Original "predefined factor pairs" was too narrow — replaced with exhaustive discovery + anomaly/trend/change-point + clinical rules. Bryan-approved expanded scope; see decisions log entries 2026-05-11.

**Scope reduced** after audit found existing `correlation_service.go` (411 lines, has Pearson w/ lag), `cohort_service.go` (290 lines, has cohort matching + anonymous hashing), and `realtime_detection.go` (446 lines, on-event detection). Built new files for the actual gaps; did NOT duplicate.

- [x] Create `internal/service/insight_statistics.go` — Mean, StdDev, ZScore, RollingMean, LinearRegression w/ p-value, PearsonPValue, BenjaminiHochberg FDR, DetectChangePoint
- [x] Create `internal/service/insight_autoscan.go` — exhaustive correlation scanner across all factor pairs × 4 lags with FDR correction
- [x] Create `internal/service/insight_per_metric.go` — anomaly + trend + change-point detection per metric
- [x] Create `internal/service/insight_clinical_rules.go` — FDA-auto layer + change-point/medication-start coincidence layer; Source 2 (admin-editable rules) deferred to follow-up commit per design (~1 day of admin handler work)
- [x] Skip cohort aggregator rebuild — existing `cohort_service.go` already covers it
- [x] Wire all three scanners into `insight_generator.go` runInternalScans method
- [x] Tests: insight_statistics_test.go (Mean, StdDev, ZScore, RollingMean, LinearRegression, PearsonPValue, BenjaminiHochberg, DetectChangePoint) + ai_insight_json_extractor_test.go
- [x] Fix Claude prose-with-[CHILD] JSON parsing edge case (extractor walks strings properly)
- [x] Build + service tests green
- [x] Deploy to dev
- [ ] Commit (in flight)
- [ ] Observe insight count after dev run

**Phase 2 follow-up (own commit, ~1 day)**: Source 2 admin-curated rules — migration `00035_clinical_rules.sql`, repo + admin handlers, simple DSL evaluator. TODO marker left in `insight_clinical_rules.go`.

### Phase 3 — Opt-in narrative consent flow
- [ ] Migration `00036_ai_narrative_consent.sql`
- [ ] Repository accessors + audit-write methods
- [ ] Service-level consent gate in `callClaude()`
- [ ] Settings page UI with toggle + disclosure modal
- [ ] Feature flag `AI_NARRATIVE_OPT_IN_AVAILABLE` (default false)
- [ ] Tests
- [ ] Commit (feature gated off)

### Phase 4 — Admin capacity monitoring
- [ ] Capacity handler + template
- [ ] CloudWatch metric integration
- [ ] DB query for insights/users counts
- [ ] Threshold-based color indicators
- [ ] Admin nav update
- [ ] Commit

### Phase 5 — Bedrock migration + BAA + ship
- [ ] (Bryan) Sign AWS BAA in AWS Artifact
- [ ] (Bryan) Enable Claude model access in Bedrock console (us-east-1)
- [ ] Add `aws-sdk-go-v2/service/bedrockruntime` to go.mod
- [ ] Replace `callClaude()` HTTP code with Bedrock InvokeModel
- [ ] (Bryan approves) Apply IAM policy granting `bedrock-runtime:InvokeModel` to EC2 instance role
- [ ] Update `ClaudeConfig` → `BedrockConfig`
- [ ] Smoke test on dev: full AI analysis flow via Bedrock
- [ ] Update `templates/privacy.html` with Bedrock + BAA disclosure
- [ ] Update `infrastructure/app-store-metadata.md` privacy nutrition label answers
- [ ] Flip `AI_NARRATIVE_OPT_IN_AVAILABLE=true` in prod env
- [ ] Commit
- [ ] (Bryan says "ship"/"deploy"/"prod") Deploy to prod
- [ ] New `mobile-v*` git tag → Codemagic build
- [ ] Internal QA pass on TestFlight
- [ ] App Store Connect submission

---

## Commit Lineage

All commits in this initiative will reference back to this design doc in the commit message footer:
> See `docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md` for full design.

Shipped commits (in chronological order):
1. **`9b1e277`** — `docs(legal): name JB2S Enterprises, LLC d/b/a MyCareCompanion in Terms + Privacy preambles (App Store Blocker 3)`. Blocker 3 sign-off material; sets up the legal entity placement decision.
2. **`0b1bb86`** — `feat(ai-insights): Phase 1 — strip all PHI before outbound Anthropic calls`. The de-identification boundary work, drug-class extensions, copy fixes, JSON parser robustness pass 1.
3. **`9e9347e`** — `feat(insights): Phase 2 — internal-AI scanners (correlation/anomaly/trend/change-point/clinical)`. ~2000 LOC across 6 new files. JSON parser robustness pass 2 (`[CHILD]`-in-prose handling).
4. **`11d8c01`** — `fix(insights): clamp per-metric pattern values to fit numeric(4,3) DB column`. Z-scores were overflowing the existing schema.
5. **`d406b0c`** — `feat(ai-insights): Phase 3 backend — opt-in narrative consent gate`. Consent service + migration 00035 + AI service gate, all dormant by default.

All five shipped to prod via instance refresh `1ae14d81-047a-4f60-83c5-cbd35ce2e29d` on 2026-05-11 22:59 UTC under ECR digest `cbe35ae3206e808a...`. Verified live.

---

## Technical Findings During the Session

Things discovered during the work that are not strictly architectural decisions but matter for understanding the system state.

### Finding 1: Prod has `CLAUDE_ENABLED` unset — Anthropic was never being called in prod

Discovered during the post-Phase 3 smoke test (2026-05-11). The CareCompanion prod EC2 environment does not have `CLAUDE_ENABLED=true` set, so the AI Insights pipeline has never called `api.anthropic.com` in production. Steinmetz family (and all other prod families) have only been getting the simple 5-rule `insight_generator.go` output until today.

This reshapes the impact narrative substantially:
- **Phase 1 (PHI stripping)** is forward-protection — it'll matter when Bedrock comes online in Phase 5. There is no urgent PHI-leak to remediate in prod because no PHI was being sent.
- **Phase 2 (internal scanners) is the immediate user-facing win** — 79 correlation patterns + 41 clinical insights generated for 13 prod children on the very first scan, with zero LLM cost.

Practical implication: when we eventually enable Claude (via Bedrock) in prod, all the de-identification machinery from Phase 1 is sitting there waiting to be tested under real load. Until then, prod is running internal-only intelligence — which is genuinely valuable on its own.

### Finding 2: `drug_database.go`'s `getDrugClass()` had no antipsychotics/anticonvulsants — critical gap for autism population

The original `getDrugClass()` map covered stimulants, SSRIs, SNRIs, opioids, benzodiazepines, anticoagulants, NSAIDs, and PPIs. The autism population disproportionately uses antipsychotics (Risperdal, Abilify) and anticonvulsants (Depakote, Tegretol) — none of which were mapped. Fix in commit `0b1bb86`: added 40+ medications across antipsychotic, anticonvulsant, alpha-agonist, non-stimulant-adhd, atypical-antidepressant, tricyclic, antihistamine, and mood-stabilizer classes.

This was a pre-existing gap that affected the drug-drug interaction detection in `CheckInteractions()` too — not just our new PHI stripping path. The extension benefits both code paths.

### Finding 3: Claude responses contain literal `[CHILD]` after Phase 1, breaking byte-naive JSON parsing

After we instructed Claude to use the `[CHILD]` placeholder, Claude sometimes returns prose containing the placeholder (e.g., "I'm ready to analyze [CHILD]'s profile but don't see the data..."). The naive byte-level `strings.IndexByte('[')` JSON extractor in `ai_insight_service.go` would find the `[` of `[CHILD]` and try to parse the description as a JSON array, failing.

Final fix in commit `9e9347e`: new `extractJSONArray()` function that finds the first `[` followed (after whitespace) by `{` and walks the string with proper JSON string-state tracking. Handles markdown fences, prose-with-placeholder, nested brackets in strings, and escaped quotes. 10 dedicated unit tests in `ai_insight_json_extractor_test.go`.

Side benefit: Emma's prod insight count went from 3 (pre-Phase 1) → 6 (post-fix) on the very next analysis run.

### Finding 4: `family_patterns.correlation_strength` is `numeric(4,3)` — z-scores overflow

Per-metric anomaly and change-point detection store z-score-style magnitudes. The existing schema's `correlation_strength` column is `numeric(4,3)` (max ±9.999) because it was originally designed for Pearson r in [-1, 1]. Z-scores can exceed 10 in extreme cases.

Initial Phase 2 first prod run showed 0 per-metric patterns even though clear anomalies existed — silent DB INSERT failures with `ERROR: numeric field overflow (SQLSTATE 22003)`. Fix in commit `11d8c01`: `clampForDB()` helper clamps values to ±9.999 before insert. Anything above means "extremely anomalous" without quantitative resolution loss for our use.

Confirmed working on second prod run — 3 per-metric patterns surfaced where 0 had before.

### Finding 5: Clinical-rule scanner dedupe is too loose — surfacing duplicates per scan

Open follow-up. The clinical-rule scanner's `alreadySurfaced()` predicate matches by title substring containing the medication name + kind tag. If titles don't exactly match the previous insight's title, duplicates leak through. Result: 14 clinical insights per scan even when previous scans had already covered the same meds.

Not fixed in this session (low priority — duplicates are a small annoyance, not a correctness issue). Tighten the dedupe predicate so the same (medication, kind) tuple doesn't re-emit within the dedupe window, regardless of title text variations.

### Finding 6: AWS BAA is contractual, not technical — doesn't restrict shared servers

Discussed at length during the Bedrock multi-project conversation. The AWS BAA is signed at the AWS account level via AWS Artifact and is a passive contractual safety net. It does NOT prohibit non-PHI workloads in the same account or on the same EC2 host. The actual technical restriction is "use HIPAA-eligible services when handling PHI" — and almost everything you'd want is eligible (EC2, RDS, S3, Bedrock, Lambda, etc.).

Practical implication: this dev server can keep serving multiple projects post-Bedrock. Separation is recommended over time for blast-radius/audit reasons (see Decision 11), not because Bedrock forces it.

---

## Open Items / Phase 3+ Backlog

Tracked here so future sessions can find them without re-reading the whole doc.

**Phase 3 follow-up (still in own commit):**
- Settings page UI: toggle + disclosure modal (only visible when `AI_NARRATIVE_OPT_IN_AVAILABLE=true`)
- API handler to POST consent changes (`/api/account/ai-narrative-consent`)
- Estimated: ~0.5 day
- Currently consent can only change via direct SQL (acceptable for dev; not user-facing)

**Phase 4 — Admin capacity monitoring:**
- New file `internal/handler/admin/capacity_handler.go`
- New template `templates/admin/capacity.html` with EC2 CPU/memory, RDS connections, ElastiCache memory, insights generated /24h, active users /24h, LLM API spend rolling 7-day, color-coded headroom indicators
- Existing `cloudwatch_service.go` is the integration point
- Estimated: ~0.5 day

**Phase 5 — Bedrock migration + ship:**
- (Bryan) Sign AWS BAA in AWS Artifact — ~5 min
- (Bryan) Enable Claude model access in Bedrock console (us-east-1) — ~5 min
- Add `aws-sdk-go-v2/service/bedrockruntime` to go.mod
- Replace `callClaude()` HTTP code with Bedrock InvokeModel
- (Bryan approves) Apply IAM policy granting `bedrock-runtime:InvokeModel` to EC2 instance role — cat #1
- Update `ClaudeConfig` → `BedrockConfig`
- Smoke test on dev: full AI analysis flow via Bedrock
- Update `templates/privacy.html` with Bedrock + BAA disclosure
- Update `infrastructure/app-store-metadata.md` privacy nutrition label answers
- Flip `AI_NARRATIVE_OPT_IN_AVAILABLE=true` in prod env
- Commit
- (Bryan says "ship"/"deploy"/"prod") Deploy to prod
- New `mobile-v*` git tag → Codemagic build
- Internal QA pass on TestFlight
- App Store Connect submission

**Cleanup items:**
- Finding 5 — clinical-rule scanner dedupe tightening
- Source 2 of clinical rules — admin-curated rules table + admin UI + DSL evaluator (~1 day)
- Consider whether to set `CLAUDE_ENABLED=true` in prod env post-Bedrock so the Steinmetz family also gets the narrative-polish layer (currently they only get internal scans, no LLM output, even though Phase 1 makes it safe)

---

*End of design document. Begin Phase 1 work.*
