# 2026-05-06 — Calm Hybrid UI rollout + admin bulk-delete + medication dropdown fix

## Summary

Ships the parent-facing Calm Hybrid redesign (foundation → layout → round-3
→ rewards port), the matching mobile/Capacitor shell theme alignment, an
admin bulk-select/bulk-delete tool on the support tickets page, and a
small bug fix to the medication suggestions dropdown opacity.

**No schema migrations** in this batch — pure code/templates/CSS/JS.

## Pre-deploy baseline (captured 2026-05-06 00:13 UTC)

| Item | Value |
|---|---|
| Live ECR `latest` digest (rollback target) | `sha256:f3732e4a57d019a79f7d9de9aa5e17152eb5f458f753372367f64a87c74d5bad` |
| Pre-existing tagged rollback image | `rollback-safe` → `sha256:a85a6cded1ce778d…` (2026-05-03 known-good) |
| ASG | `carecompanion-asg`, desired=1, healthy=1 |
| Running instance | `i-02bec4ff1699cfe52` (status: healthy) |
| origin/master HEAD | `cca5599 fix(deploy): use MinHealthy=100 + InstanceWarmup=180 in ASG refresh` |

Restoration target: image digest `sha256:f3732e4a57d019a7…` is what was
serving `https://www.mycarecompanion.net` immediately before this deploy.

## Commits being shipped

`origin/master..HEAD` (oldest → newest):

1. **`cf5f9d5` feat(ui): parent-facing Calm Hybrid redesign — foundation pass**
   - New `/static/css/calm.css` design system (calm-bg gradient, .glass,
     .ring-soft, .compose-fab, .display, .handwritten, .grow-ring, .seed-btn,
     .bloom).
   - `partials/head.html`: links calm.css, theme-color → orange, drops the
     dark-mode init script.
   - Bulk find/replace across 25 standalone parent templates + 4 partials:
     all `dark:*` Tailwind classes stripped, all `indigo-*` swapped to
     `orange-*`, body bg-gray-{50,100} → calm-bg.
   - `landing.html` palette repainted, login/register/landing/mascot
     gradients harmonized.
   - `settings.html` theme picker UI removed (single warm-light only).
   - Tailwind CSS rebuilt with the new orange/stone class set.
   - Verified: every `{{template}}` / `{{if}}` / `{{range}}` / `{{end}}`
     directive preserved; public routes return 200; auth routes 303.

2. **`92539ab` feat(ui): Calm Hybrid layout pass — parent pages**
   - `landing.html` handwritten greeting, display headline, glass stat
     ribbon, pill CTAs, soft FAQ details.
   - `login.html` + `register.html` glass card forms, uppercase
     tracking-widest field labels, rounded-2xl inputs, pill submit.
   - `child_dashboard.html` Today's bright spot panel, encouragement
     footer, floating compose FAB.
   - `settings.html` form chrome upgraded.
   - `billing_success.html` / `billing_cancel.html` / `error.html` glass
     hero panels.
   - `privacy.html` + `terms.html` glass content card, display headings.
   - `rewards.html` standalone gradient bg + Fraunces.

3. **`26e4b19` feat(ui): Calm Hybrid round-3 — dashboard hero, mascot polish, surface sweep**
   - **Backend (UI-driven):** New `models.FamilyWeekStats`,
     `FamilyRepository.GetWeekStats` (7-day cross-table aggregate, best-
     effort with zero fallback), `FamilyService.GetDashboard` returns
     `WeekStats`. New template helpers: `div`, `div_f`, `mod`, `list`,
     `append`, `join`, `pluralize`.
   - **Templates:** `dashboard.html` glass hero with WeekStats summary,
     `child_dashboard.html` ~1.4k-line structural rebuild, sweeps across
     `daily_logs`, `medications`, `insights`, `reports`, `alerts`,
     `alert_analysis`, `chat`, `settings`, `child_settings`, `support`,
     `beta_onboard`, `new_child`, `new_family`, `billing_*`,
     `privacy/terms/error`. Partials nav/footer/mascot harmonized.
   - **CSS/JS:** calm.css additions for dashboard tiles, tailwind.css
     rebuilt, global-search.js minor styling.

4. **`477a7c2` feat(ui): port rewards page onto Calm Hybrid design tokens**
   - Reworked the standalone `/rewards` marketing page to use shared
     calm.css instead of inline gradient/Fraunces rules. Content unchanged.

5. **`8541fde` fix(mobile): finish Calm Hybrid pass for the Capacitor shell**
   - `mobile/capacitor.config.json`: SplashScreen + StatusBar bg
     `#4F46E5` → `#fbf6ee`, style LIGHT → DARK.
   - `static/js/capacitor-bridge.js`: matching runtime override.
   - `templates/partials/head.html`: meta theme-color
     `#EA580C` → `#fbf6ee`, status-bar-style `black-translucent` → `default`.
   - `static/css/calm.css`: `body.capacitor #compose-fab-wrap` shifts
     FAB up by `env(safe-area-inset-bottom)`; same env() padding on
     `#compose-sheet` and `#meds-tray-sheet`.
   - `templates/child_dashboard.html`: added `id="compose-fab-wrap"` to
     FAB anchor.
   - 5 inline-head templates got `viewport-fit=cover` (beta_onboard,
     error, landing, privacy, terms).

6. **`50ae280` feat(admin): bulk-select + bulk-delete on support tickets**
   - `AdminRepository.DeleteTickets([]uuid.UUID) → (int64, error)`. Single
     `DELETE FROM support_tickets WHERE id = ANY($1::uuid[])` —
     ticket_messages and ticket_attachments cascade automatically;
     error_logs / roadmap_items / bounty_awards / sibling-duplicate
     references SET NULL by existing FK rules.
   - `Handler.DeleteTickets` at `DELETE /api/admin/super/tickets`:
     validates UUIDs, rejects empty arrays, caps at 200/request, drains
     S3 attachment objects via `attachService.DeleteAllForTicket` before
     the DB delete, audit-logs the requested/deleted counts and full id
     list.
   - Mounted under `/super` with `RequireSuperAdmin` middleware.
   - UI: bulk-action bar, per-row checkboxes with select-all, confirm()
     prompt, role-gated to super_admin.
   - Bryan flagged this as temporary cleanup tooling for the beta
     period; safe to revert later via `git revert 50ae280`.

7. **`2d30aca` fix(ui): make medication-suggestions dropdown fully opaque**
   - One-class fix in `templates/medications.html`: dropped duplicate
     `bg-white/80` from the suggestions overlay (was beating the solid
     `bg-white` by source order, leaking form fields underneath).

## Functional verification on dev (before this deploy)

- All 22 parent routes return 200, zero `<no value>` template leaks.
- Diet log / behavior log / medication log / profile update round-trip
  cleanly (POST → GET-back → DELETE/restore).
- Dashboard hero `WeekStats` numbers match SQL aggregate exactly
  (88 logs / 5 of 7 days / 2 of 2 kids / 30 meds / 1 meltdown for the
  Smith family); `pluralize` helper renders singular for n=1.
- Login form posts cookies, lands on dashboard; bad password → 401.
- All 11 unique `/static/` assets served 200.
- Bulk-delete admin endpoint: support role → 403, super_admin → 200
  with cascade verified, audit log captured ids, edge cases (empty
  array, bad UUID, >200 ids) all 400 cleanly.

## Rollback procedures

### Path A — Single-commit revert (preferred for one isolated regression)

If a specific feature breaks but the rest is fine, identify the offending
commit and revert it:

```bash
cd /home/carecomp/carecompanion

# Option 1: revert one commit (creates a new commit that undoes it)
git revert <sha>           # any of the 7 SHAs above

# Option 2: revert several commits at once
git revert <oldest>..<newest>

# Then re-deploy
./scripts/deploy.sh
```

Cleanest revert candidates by isolation:

| Scenario | Revert |
|---|---|
| Medication dropdown fix broke something | `git revert 2d30aca` |
| Bulk-delete misbehaving | `git revert 50ae280` |
| Mobile Capacitor shell looks wrong | `git revert 8541fde` |
| Rewards page broke | `git revert 477a7c2` |
| Calm Hybrid UI as a whole has issues | `git revert 2d30aca 50ae280 8541fde 477a7c2 26e4b19 92539ab cf5f9d5` (all 7) |

### Path B — Emergency image rollback (whole-deploy rollback, fastest)

If the new image is broken entire and we need to restore service NOW
without waiting for a new build:

```bash
# 1. Re-tag the previous image digest as :latest
aws ecr batch-get-image --region us-east-1 --repository-name carecompanion \
  --image-ids imageDigest=sha256:f3732e4a57d019a79f7d9de9aa5e17152eb5f458f753372367f64a87c74d5bad \
  --query 'images[0].imageManifest' --output text > /tmp/old.manifest

aws ecr put-image --region us-east-1 --repository-name carecompanion \
  --image-tag latest --image-manifest "$(cat /tmp/old.manifest)"

# 2. Trigger another ASG refresh
aws autoscaling start-instance-refresh \
  --auto-scaling-group-name carecompanion-asg \
  --preferences '{"MinHealthyPercentage":100,"MaxHealthyPercentage":200,"InstanceWarmup":180}' \
  --region us-east-1
```

The known-good image to roll back to:
`sha256:f3732e4a57d019a79f7d9de9aa5e17152eb5f458f753372367f64a87c74d5bad`
(was prod's `latest` immediately before this deploy)

A second known-good fallback (older, 2026-05-03) is tagged `rollback-safe`:
`sha256:a85a6cded1ce778d…`

### Path C — Database changes

**Not applicable.** This deploy includes zero migrations. The only DB
interaction the new code adds is the `GetWeekStats` SELECT (read-only,
no schema impact) and the bulk `DELETE FROM support_tickets` (only fires
when a super-admin actively clicks the new bulk-delete button).

## Monitor commands

```bash
# Instance refresh progress
aws autoscaling describe-instance-refreshes \
  --auto-scaling-group-name carecompanion-asg \
  --region us-east-1 \
  --query 'InstanceRefreshes[0].[Status,PercentageComplete]' --output table

# ALB target health
aws elbv2 describe-target-health --region us-east-1 \
  --target-group-arn arn:aws:elasticloadbalancing:us-east-1:943431294725:targetgroup/carecompanion-tg/bade3e56ae036ce7 \
  --query 'TargetHealthDescriptions[*].[Target.Id,TargetHealth.State]' --output table

# CloudWatch app logs (for runtime errors after refresh)
aws logs tail /carecompanion/app --region us-east-1 --since 5m

# Live health endpoint
curl -s https://www.mycarecompanion.net/health
```

## Post-deploy verification (smoke test plan)

Will be executed against `https://www.mycarecompanion.net` once refresh
hits 100%:

1. Anonymous: `/`, `/login`, `/privacy`, `/terms`, `/rewards` → 200
2. Authenticated as a real prod user (Bryan's account) — every parent
   route returns 200, dashboard hero renders, no `<no value>` leakage.
3. Capacitor TestFlight build → next launch picks up the new web UI
   (server.url points at prod).
