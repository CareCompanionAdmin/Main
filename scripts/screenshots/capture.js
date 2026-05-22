// App Store screenshot rig — logs into prod as the Smith Test Family
// (synthetic test data, no real PHI) and captures 5 portrait shots at
// 1290×2796 (iPhone 16 Pro Max App Store spec). Hides the help-bear mascot
// via injected CSS and pre-generates a real report so /reports has live state.
//
// Run:  node capture.js
// Output: ../../infrastructure/app-store-screenshots/*.png

const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const BASE = process.env.BASE_URL || 'https://www.mycarecompanion.net';
// Smith Test Family — synthetic data, 6mo of heavy use on prod
const EMAIL = process.env.DEMO_EMAIL || 'joe_parent1@test.com';
const PASSWORD = process.env.DEMO_PASSWORD || 'TestPass1!';
const OUT_DIR = path.resolve(__dirname, '../../infrastructure/app-store-screenshots');

const VIEWPORT = { width: 1290, height: 2796 };
const USER_AGENT =
  'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) ' +
  'AppleWebKit/605.1.15 (KHTML, like Gecko) MyCareCompanionApp Version/17.0 ' +
  'Mobile/15E148 Safari/604.1';

// CSS injected on every page to suppress floating affordances that would
// distract App Store reviewers / shoppers in the screenshots.
const HIDE_CHROME_CSS = `
  #helpMascotDesktop, #helpMascotMobile,
  #helpMascotCollapsed, #helpMascotMobileCollapsed { display: none !important; }
`;

async function shot(page, name) {
  fs.mkdirSync(OUT_DIR, { recursive: true });
  const out = path.join(OUT_DIR, `${name}.png`);
  await page.screenshot({ path: out, fullPage: false });
  console.log(`  → ${out}  (${(fs.statSync(out).size / 1024).toFixed(1)} KB)`);
}

async function settle(page, extraMs = 1500) {
  try {
    await page.waitForLoadState('networkidle', { timeout: 15000 });
  } catch (_) {}
  // Inject the mascot-hiding CSS after every navigation
  await page.addStyleTag({ content: HIDE_CHROME_CSS }).catch(() => {});
  await page.waitForTimeout(extraMs);
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 1,
    userAgent: USER_AGENT,
    isMobile: true,
    hasTouch: true,
    locale: 'en-US',
    timezoneId: 'America/Chicago',
  });
  const page = await context.newPage();

  console.log('1. Login as', EMAIL);
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' });
  await page.fill('#email', EMAIL);
  await page.fill('#password', PASSWORD);
  await Promise.all([
    page.waitForURL(/\/dashboard/, { timeout: 20000 }),
    page.click('button[type="submit"]'),
  ]);
  await settle(page);

  // Pick Joe (5bd37432-...) — heavier seed data, generated insights tonight
  const JOE_ID = '5bd37432-c986-5730-9d13-bb9b1318bf6b';
  const HOLLY_ID = '4a20eed4-126d-5754-b296-0f9a47040b63';
  const childPath = `/child/${JOE_ID}`;

  // Pre-generate a real PDF report (custom range covering Joe's seeded April
  // "stable+events" phase: seizure on Apr 16, ear infection Apr 26 — produces
  // rich charts). Period_type values accepted by server: day|week|month|custom.
  console.log('2. Pre-generate an April report for Joe');
  const token = await page.evaluate(() => localStorage.getItem('access_token'));
  const genResult = await page.evaluate(
    async ({ base, childId, token }) => {
      const r = await fetch(`${base}/api/children/${childId}/reports/generate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
        body: JSON.stringify({
          period_type: 'custom',
          start_date: '2026-04-01',
          end_date: '2026-04-30',
          data_filters: ['behavior', 'sleep', 'diet', 'medications', 'bowel', 'sensory', 'social', 'therapy', 'speech', 'seizure', 'weight', 'health'],
        }),
      });
      const data = await r.json().catch(() => ({}));
      return { ok: r.ok, status: r.status, report: data.report || data, error: data.error || data.message };
    },
    { base: BASE, childId: JOE_ID, token }
  );
  console.log('   gen status:', genResult.status, 'report id:', genResult.report?.id, genResult.error || '');

  // ---- 1. Child dashboard (hero "Today, Joe is X/10")
  console.log('3. Capture: child dashboard (Joe)');
  await page.goto(`${BASE}${childPath}/`, { waitUntil: 'domcontentloaded' });
  await settle(page, 3000);
  await shot(page, '01-child-dashboard');

  // ---- 2. Daily logs / quick log
  console.log('4. Capture: daily logs (Joe)');
  await page.goto(`${BASE}${childPath}/logs`, { waitUntil: 'domcontentloaded' });
  await settle(page, 2500);
  await shot(page, '02-quick-log');

  // ---- 3. Insights (R7-C disclaimer at bottom)
  console.log('5. Capture: insights (Joe)');
  await page.goto(`${BASE}${childPath}/insights`, { waitUntil: 'domcontentloaded' });
  await settle(page, 3000);
  await shot(page, '03-insights');

  // ---- 4. Medications (schedule + adherence — concrete functional screen,
  // replaces the chat screenshot which has no seeded messages on test family).
  console.log('6. Capture: medications (Joe)');
  await page.goto(`${BASE}${childPath}/medications`, { waitUntil: 'domcontentloaded' });
  await settle(page, 2500);
  await shot(page, '04-medications');

  // ---- 5. Reports (now shows Past Reports populated by our generate call)
  console.log('7. Capture: reports (Joe)');
  await page.goto(`${BASE}${childPath}/reports`, { waitUntil: 'domcontentloaded' });
  await settle(page, 2500);
  await shot(page, '05-reports');

  await browser.close();
  console.log('\nDone. PNGs in', OUT_DIR);
})().catch((err) => {
  console.error('FAILED:', err);
  process.exit(1);
});
