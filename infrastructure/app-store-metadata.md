# MyCareCompanion — App Store Metadata

## App Name
MyCareCompanion

## Subtitle (Apple, 30 chars max)
Autism Care Tracking & Logs

## Short Description (Google Play, 80 chars max)
Track medications, behaviors, sleep, and more for children with autism.

## Full Description (both stores)
MyCareCompanion is a care-organization and journaling tool for families raising children with autism. It does not diagnose, treat, or claim to prevent any condition — it helps you organize an enormous amount of daily care information in one place so you, your caregivers, and your clinicians can make better-informed decisions together.

COMPREHENSIVE CARE TRACKING
- Log medications with schedules and reminders
- Track behaviors, moods, and triggers throughout the day
- Record sleep patterns, meals, and dietary information
- Document health events, seizures, and medical appointments
- Monitor therapy sessions and progress notes

PATTERNS WE SURFACE
- Daily summaries that highlight correlations and trends across your logs
- Correlation analysis across behavior, sleep, diet, and medications
- Visual charts and trend reports to bring to provider visits
- Observations grounded in your family's own logged data — you and your clinician decide what's meaningful

FAMILY COLLABORATION
- Invite caregivers, family members, and providers to your care team
- Built-in chat for coordinating care across your team
- Role-based access so everyone sees what they need
- Real-time alerts when important events are logged

REPORTS & SHARING
- Generate detailed PDF reports for doctor visits
- Customizable date ranges and data categories
- Share reports directly from the app
- Track progress over weeks and months

PUSH NOTIFICATIONS
- Medication reminders so nothing gets missed
- Alerts when notable patterns or events are detected in your logs
- Notifications when care team members send messages

PRIVACY FIRST
- Your family's health data is never sold or shared
- All data encrypted in transit and at rest
- No ads, no tracking, no data brokers
- You own your data — export or delete anytime
- Pattern analysis runs on de-identified data; narrative features require explicit opt-in

MyCareCompanion is designed by a family who understands the daily challenges of autism care. We built the tool we wished we had — to help us, our caregivers, and our doctors give our child the best possible care, not to replace any of them.

## Keywords (Apple, 100 chars max, comma-separated)
autism,care,tracking,medication,behavior,sleep,health,log,family,caregiver,ASD,special needs,diary

## Category
Health & Fitness

## Content Rating
- Apple: 4+ (no objectionable content)
- Google: Everyone

## App Review Notes (Apple)
MyCareCompanion is a tracking and journaling tool for parents of children with autism. It is NOT a diagnostic, treatment, or medical device — all in-app "patterns" are statistical correlations across user-logged data with prominent disclaimers to consult a physician.

Subscription is $15/month handled on the web at carecompanion.net per Apple's US-storefront external-link allowance (April 2025 guideline update, 3.1.1(a)). Tap Subscribe in the app to see the neutral redirect notice and external Safari handoff.

Family chat is private to invited family members only. Each message has a Report icon (next to non-own messages) that opens a pre-addressed email to support. Family owners may also remove members directly from Settings → Members.

Demo account for review:
Email: appreview@mycarecompanion.net
Password: MyCareReview2026!

This account has a complimentary subscription, a sample child profile with seeded behavior + sleep logs over the past 7 days, an active medication schedule, and a family-chat thread with sample messages. The account is fully functional — including the in-app Account Deletion flow at Settings → Danger Zone.

Privacy policy: https://www.mycarecompanion.net/privacy
Medical disclaimer: visible at Settings → About and at the bottom of every Insights / Alert Analysis page; PDF reports include the disclaimer in the footer of every page.
Support: support@mycarecompanion.net

The app loads a web URL (https://www.mycarecompanion.net) inside a native WebView shell. This is intentional — it allows instant updates without app store resubmission. The native shell provides push notifications, safe-area handling, app lifecycle management via Capacitor, and routes external links (FDA drug info, Stripe checkout, support email) to the system Safari / Mail apps rather than loading them inline.

## Google Play Health Apps Declaration
- App category: Care coordination / health tracking
- NOT a regulated medical device
- Does NOT connect to Health Connect
- Disclaimer: "This app is not a medical device and does not diagnose, treat, or prevent any condition."
- Privacy policy URL: https://www.mycarecompanion.net/privacy

## Google Play Data Safety
Data collected:
- Personal info: Name, email (account creation)
- Health info: Medication records, behavior logs, sleep data, health events (user-entered)
- Messages: In-app family chat
- Device info: Push notification tokens

Data sharing: None — no data shared with third parties
Data encryption: Yes, in transit and at rest
Data deletion: Users can request account and data deletion

## Apple App Privacy ("Nutrition Label") Answers
Verified 2026-05-12 by auditing `mobile/package.json`, `mobile/ios/App/Podfile`, `mobile/android/app/build.gradle`, and `static/js/**`.

**No analytics or crash-reporting SDKs are bundled.** No Sentry, Crashlytics, Firebase Analytics, Datadog, Amplitude, Mixpanel, Posthog, or Segment. The only Firebase touch point is `@capacitor/push-notifications` which uses FCM exclusively for push delivery (no analytics emission).

Data type declarations (Linked to User, App Functionality only, NOT used for tracking or advertising):
- **Health & Fitness** — medication records, behaviors, mood, sleep, seizure logs, therapy notes
- **Contact Info** — email, name (account creation + family chat display)
- **User Content** — family chat messages, free-text notes on logs, PDF report content
- **Identifiers** — internal user ID (server-issued UUID, not shared with third parties)

Not declared:
- Diagnostics — none collected (no analytics/crash SDK)
- Tracking — none (no third-party tracking)
- Advertising — none (no ad SDKs)

## App Icon Status (2026-05-12 audit)
`mobile/icons/` contains seven PNG-encoded files (icon-48 through icon-512). All under 512×512. **Apple requires 1024×1024 PNG (no transparency, no rounded corners, no Apple imagery) for the App Store listing.** A 1024×1024 master source is needed before submission — either upscale-and-resharpen the existing 512px PNG or regenerate from the source design file. Bryan TODO.

## Target Audience
Adults (18+) — the app is for adult caregivers, not children

## Privacy Policy URL
https://www.mycarecompanion.net/privacy

## Support URL
https://www.mycarecompanion.net

## Screenshots Needed
iPhone (6.7" and 6.5"):
1. Dashboard showing child overview
2. Daily log entry screen
3. Medication schedule
4. Insights/analytics view
5. Family chat

Android (phone + tablet):
Same 5 screens

Feature Graphic (Google Play, 1024x500):
Logo centered on blue (#3b82f6) background with tagline
