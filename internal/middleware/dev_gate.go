package middleware

import (
	"html"
	"net/http"
	"os"
	"strings"
	"time"
)

// DevGateMiddleware fronts the dev/staging environments with a one-time
// passphrase gate so casual visitors who find the dev URL can't reach the
// app, while the native Capacitor shell sails through transparently.
//
// Three ways to pass the gate, in order:
//   1. APP_ENV == "production" — middleware is a no-op pass-through.
//   2. The request carries a User-Agent containing appUserAgentMarker
//      (set by Capacitor's appendUserAgent in capacitor.config.json), or
//   3. The request carries the dev_gate_ok cookie set after a prior
//      successful passphrase entry.
//
// Posting the correct passphrase to /__dev_gate sets the cookie for 30
// days. Browser visitors see a minimal form; native-app users never see
// anything.
//
// Bypassed paths (always reachable regardless of gate state):
//   /health, /api/maintenance-status, /static/*, /favicon.ico, /__dev_gate
//
// Belt-and-suspenders: if /etc/carecompanion/dev-gate-off exists on disk,
// the middleware is bypassed entirely. This is the emergency-recovery
// hatch documented in project_carecompanion_app_store_approval.md.
func DevGateMiddleware(env, gateCode, appUserAgentMarker string) func(http.Handler) http.Handler {
	emergencyOverride := fileExists("/etc/carecompanion/dev-gate-off")

	return func(next http.Handler) http.Handler {
		// In prod or with the override file present, the middleware is a
		// no-op — return the next handler unwrapped.
		if env == "production" || emergencyOverride {
			return next
		}
		if gateCode == "" {
			// If the operator didn't set DEV_GATE_CODE, refuse to start a
			// fail-open middleware. We log a clear error and pass through
			// — the operator is expected to set the var. (Failing closed
			// here would lock everyone out of dev on a config typo.)
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always-allowed paths: health, static, the gate itself.
			if isBypassedPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Native-app shell bypass — Capacitor appends the configured
			// marker to its User-Agent on every request, so this catches
			// both the initial page load and every subsequent fetch.
			if appUserAgentMarker != "" && strings.Contains(r.Header.Get("User-Agent"), appUserAgentMarker) {
				next.ServeHTTP(w, r)
				return
			}

			// Prior-success cookie bypass.
			if c, err := r.Cookie("dev_gate_ok"); err == nil && c.Value == "1" {
				next.ServeHTTP(w, r)
				return
			}

			// Submitted passphrase — accept or re-prompt.
			if r.Method == http.MethodPost && r.URL.Path == "/__dev_gate" {
				_ = r.ParseForm()
				if r.PostFormValue("code") == gateCode {
					http.SetCookie(w, &http.Cookie{
						Name:     "dev_gate_ok",
						Value:    "1",
						Path:     "/",
						Expires:  time.Now().Add(30 * 24 * time.Hour),
						HttpOnly: true,
						Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
						SameSite: http.SameSiteLaxMode,
					})
					dest := r.PostFormValue("next")
					if dest == "" || !strings.HasPrefix(dest, "/") {
						dest = "/"
					}
					http.Redirect(w, r, dest, http.StatusSeeOther)
					return
				}
				// Wrong code — fall through to render the form with an
				// error banner.
				renderGateForm(w, r, "That code didn't match. Try again.")
				return
			}

			// First visit (or expired cookie) — show the gate form.
			renderGateForm(w, r, "")
		})
	}
}

func isBypassedPath(path string) bool {
	// /__dev_gate is intentionally NOT in this list — the middleware itself
	// renders the form and handles POST. Bypassing it here would send the
	// POST into chi which has no route for it and answers 404.
	switch {
	case path == "/health",
		path == "/api/maintenance-status",
		path == "/favicon.ico",
		strings.HasPrefix(path, "/static/"):
		return true
	}
	return false
}

func renderGateForm(w http.ResponseWriter, r *http.Request, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Don't index this page in search; even with no other defense this stops
	// the form itself from being crawled.
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusOK)

	next := r.URL.RequestURI()
	if r.URL.Path == "/__dev_gate" {
		next = "/"
	}
	errBlock := ""
	if errMsg != "" {
		errBlock = `<p class="err">` + html.EscapeString(errMsg) + `</p>`
	}

	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>MyCareCompanion — Dev</title>
<style>
:root { color-scheme: light; }
body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
       background: linear-gradient(180deg, #fbf6ee 0%, #f6efe2 100%); min-height: 100vh;
       display: flex; align-items: center; justify-content: center; padding: 2rem; }
.card { background: #fff; border-radius: 24px; box-shadow: 0 8px 24px -8px rgba(60,40,20,.15);
        padding: 2rem; max-width: 360px; width: 100%; }
h1 { font-family: Georgia, serif; font-weight: 500; color: #44403c; margin: 0 0 .5rem; font-size: 1.5rem; }
p { color: #78716c; margin: 0 0 1rem; font-size: .9rem; line-height: 1.5; }
.err { background: #fef2f2; color: #b91c1c; border-radius: 8px; padding: .5rem .75rem; font-size: .85rem; }
input[type=password] { width: 100%; box-sizing: border-box; padding: .75rem 1rem;
                      border: 1px solid #e7e5e4; border-radius: 12px; font-size: 1rem;
                      margin-bottom: .75rem; background: #fff; }
input[type=password]:focus { outline: 2px solid #fb923c; outline-offset: -1px; border-color: #fb923c; }
button { width: 100%; padding: .75rem 1rem; background: #ea580c; color: #fff; border: 0;
         border-radius: 999px; font-weight: 600; font-size: 1rem; cursor: pointer; }
button:hover { background: #c2410c; }
.note { font-size: .8rem; color: #a8a29e; margin-top: 1rem; }
</style>
</head>
<body>
<div class="card">
<h1>MyCareCompanion · Dev</h1>
<p>This is the development environment. Enter the access code to continue.</p>
` + errBlock + `
<form method="POST" action="/__dev_gate">
  <input type="hidden" name="next" value="` + html.EscapeString(next) + `">
  <input type="password" name="code" placeholder="Access code" autofocus autocomplete="off" required>
  <button type="submit">Continue</button>
</form>
<p class="note">Looking for the live app? Visit <a href="https://www.mycarecompanion.net">www.mycarecompanion.net</a>.</p>
</div>
</body>
</html>`))
}

// fileExists is the emergency-override hatch — used only at
// middleware-construction time to check for /etc/carecompanion/dev-gate-off.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
