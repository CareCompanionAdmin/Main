// session_guard.js — graceful session-expiry handling.
//
// Two failure modes covered:
//   1. Page already loaded; access_token expires while user is on the page.
//      The next button click hits the API, gets 401. Without this guard, the
//      user sees "Unauthorized" plain text or a silently-failing request.
//   2. User opens a stale tab the morning after the previous day's login.
//      The /<page> GET would normally just redirect to /login, but if the JWT
//      is *just* about to expire while the page is loading, an API call from
//      that page can still fire 401 between paint and click.
//
// Strategy:
//   - On page load, decode the JWT exp from cookie or localStorage.
//   - If exp is in the past, show the "logged out" banner immediately.
//   - If exp is in the future, set a one-shot timer to fire at exp.
//   - Globally intercept fetch() responses; on 401 from /api/, show banner.
//   - Listen for htmx:responseError; on 401, show banner.
//   - Banner is unsticky: a single CTA "Sign in again" navigates to
//     /login?return=<current-path> so the user lands back where they were.

(function () {
    'use strict';

    var SHOWN = false;       // single-shot — once shown, never re-render
    var EXPIRY_TIMER = null;

    function getCookie(name) {
        var v = '; ' + document.cookie;
        var parts = v.split('; ' + name + '=');
        return parts.length === 2 ? parts.pop().split(';').shift() : '';
    }

    function getJWT() {
        return localStorage.getItem('access_token') || getCookie('access_token') || '';
    }

    // Decode the JWT exp claim (seconds since epoch). Returns 0 if the token
    // is missing or malformed — caller treats 0 as "no token, banner if a
    // protected request lands."
    function tokenExp(jwt) {
        if (!jwt) return 0;
        var parts = jwt.split('.');
        if (parts.length !== 3) return 0;
        try {
            var pad = parts[1] + '==='.slice((parts[1].length + 3) % 4);
            var json = atob(pad.replace(/-/g, '+').replace(/_/g, '/'));
            var payload = JSON.parse(json);
            return typeof payload.exp === 'number' ? payload.exp : 0;
        } catch (_) {
            return 0;
        }
    }

    function showBanner() {
        if (SHOWN) return;
        SHOWN = true;
        // On the login page itself the banner is noise.
        if (window.location.pathname === '/login') return;
        // Wipe the local copy so a hard refresh actually goes to /login.
        try {
            localStorage.removeItem('access_token');
            localStorage.removeItem('refresh_token');
        } catch (_) {}
        // Note: HttpOnly cookies can't be cleared from JS. The server-side
        // redirect handles those — the user landing on /login via the CTA
        // will overwrite the stale cookie on next login.

        var ret = window.location.pathname + window.location.search;
        var loginHref = '/login?return=' + encodeURIComponent(ret);

        var existing = document.getElementById('session-expired-banner');
        if (existing) existing.remove();

        var banner = document.createElement('div');
        banner.id = 'session-expired-banner';
        banner.className = 'border-b border-red-300 dark:border-red-800 bg-red-50 dark:bg-red-900/40 ' +
            'text-red-900 dark:text-red-100 px-4 py-2 text-sm sticky top-0 z-50';
        banner.innerHTML = '<div class="max-w-7xl mx-auto flex items-center gap-3">' +
            '<svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">' +
            '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" ' +
            'd="M15 12a3 3 0 11-6 0 3 3 0 016 0z M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />' +
            '</svg>' +
            '<div class="flex-1">Your session has expired. Sign in again to continue.</div>' +
            '<a href="' + loginHref + '" class="inline-flex items-center px-3 py-1 rounded-md bg-red-600 hover:bg-red-700 text-white font-medium whitespace-nowrap">' +
            'Sign in again</a>' +
            '</div>';
        document.body.insertBefore(banner, document.body.firstChild);
    }

    // Is this URL one whose 401 should trigger the banner? Only protected
    // app endpoints. Login/register/refresh themselves return 401 on bad
    // creds — those are user typos, not a session expiry.
    function shouldGuard(url) {
        if (typeof url !== 'string') {
            try { url = String(url); } catch (_) { return false; }
        }
        // Match /api/* but skip the auth endpoints whose 401 means
        // "your password was wrong" — not "your session expired."
        if (url.indexOf('/api/auth/login') >= 0) return false;
        if (url.indexOf('/api/auth/register') >= 0) return false;
        if (url.indexOf('/api/auth/refresh') >= 0) return false;
        if (url.indexOf('/api/auth/request-reset') >= 0) return false;
        if (url.indexOf('/api/auth/reset-password') >= 0) return false;
        if (url.indexOf('/api/auth/validate-reset-token') >= 0) return false;
        return url.indexOf('/api/') >= 0 || url.indexOf('://') < 0;
    }

    // Wrap fetch() so any 401 from a protected endpoint surfaces the banner.
    // Preserves the original return value so caller-side error paths still
    // work — the banner is purely additive UI.
    function installFetchGuard() {
        if (!window.fetch) return;
        var orig = window.fetch.bind(window);
        window.fetch = function (input, init) {
            var url = typeof input === 'string' ? input : (input && input.url) || '';
            return orig(input, init).then(function (resp) {
                if (resp && resp.status === 401 && shouldGuard(url)) {
                    showBanner();
                }
                return resp;
            });
        };
    }

    // htmx fires `htmx:responseError` for any non-2xx response. We can't get
    // the URL reliably from the event, so we just trust the status.
    function installHtmxGuard() {
        document.body && document.body.addEventListener('htmx:responseError', function (evt) {
            var status = evt.detail && evt.detail.xhr && evt.detail.xhr.status;
            if (status === 401) showBanner();
        });
    }

    function init() {
        var jwt = getJWT();
        var exp = tokenExp(jwt);
        var nowSec = Math.floor(Date.now() / 1000);

        if (jwt && exp > 0 && exp <= nowSec) {
            // Token already expired by the time the page rendered.
            showBanner();
        } else if (jwt && exp > nowSec) {
            // Schedule a one-shot trigger at the moment of expiry. Cap at
            // ~1 hour out — refreshing a stale tab the next morning will
            // re-arm via this same code path on the new page load. The cap
            // also avoids browser timer drift causing the banner to fire
            // at a wildly wrong time after a long sleep.
            var delayMs = Math.min((exp - nowSec) * 1000 + 1000, 60 * 60 * 1000);
            EXPIRY_TIMER = setTimeout(showBanner, delayMs);
        }
        // No token + on /login = don't show banner (redundant on login page).
        // No token + elsewhere = the auth middleware will redirect them on
        // next nav; banner here would be premature.

        installFetchGuard();
        installHtmxGuard();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
