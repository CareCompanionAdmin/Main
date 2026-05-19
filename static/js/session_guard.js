// session_guard.js — graceful session-expiry handling with silent refresh.
//
// Problem we solve:
//   The server issues short-lived access tokens (typically 8h) and a longer
//   refresh token (7d). Without this guard, when the access token lapses the
//   user mid-session sees a "session expired" banner — even though their
//   refresh token is still perfectly valid. They lose context (and possibly
//   typed-but-unsubmitted form data if they click "Sign in again").
//
// Strategy:
//   1. Proactive — schedule a background refresh ~5 minutes before exp.
//      Capped at 1 hour out so we don't sit on huge timers across browser
//      sleeps. The refresh swaps in a new token without any UI change.
//   2. Reactive — if a 401 still slips through (network flake at the
//      proactive moment, clock skew, etc.), intercept the response, attempt
//      a refresh, and silently retry the original request once. The user
//      sees their save succeed without any banner.
//   3. Concurrency — multiple background polls (chat-unread, etc.) can fire
//      401s in parallel. We collapse all in-flight refresh attempts behind
//      a single shared promise so the server only sees one refresh call.
//   4. The "session expired" banner now only fires if the refresh itself
//      fails — i.e. the refresh token is genuinely lapsed (after 7 days of
//      true inactivity) or the user account is disabled. That should be the
//      rare case, not the common case.
//
// The banner UI is unchanged from before — single CTA "Sign in again" that
// navigates to /login?return=<current-path> so the user lands back on the
// same page after re-authenticating.

(function () {
    'use strict';

    var SHOWN = false;            // single-shot — once shown, never re-render
    var EXPIRY_TIMER = null;
    var REFRESH_PROMISE = null;   // single in-flight refresh; concurrent
                                  // 401s share one network call.

    // Refresh this many seconds before the access token expires. Anything
    // smaller risks the token expiring mid-flight; anything larger wastes
    // freshly-issued time.
    var REFRESH_LEAD_SECONDS = 5 * 60;

    // Cap on the proactive timer so a multi-hour-sleeping browser doesn't
    // hold a stale setTimeout target. When the cap fires we'll just
    // re-evaluate and either refresh or reschedule.
    var TIMER_CAP_MS = 60 * 60 * 1000;

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

    // Persist the new tokens that the refresh endpoint returns. The server
    // also re-sets the HttpOnly cookies on its end, so localStorage stays in
    // sync for the JS-side decoding above.
    function storeTokens(data) {
        if (data && data.access_token) {
            try {
                localStorage.setItem('access_token', data.access_token);
            } catch (err) {
                console.warn('[session_guard] failed to persist access_token (quota or disabled storage):', err);
            }
        }
        if (data && data.refresh_token) {
            try {
                localStorage.setItem('refresh_token', data.refresh_token);
            } catch (err) {
                console.warn('[session_guard] failed to persist refresh_token (quota or disabled storage):', err);
            }
        }
    }

    // Single-flight token refresh. Returns the same promise to every caller
    // until it settles, then resets so the next 401 (if any) can try again.
    // We send the refresh_token in the body when present in localStorage —
    // older sessions only have the HttpOnly cookie, so we fall back to no
    // body and let the server pull it from r.Cookie("refresh_token").
    function attemptRefresh() {
        if (REFRESH_PROMISE) return REFRESH_PROMISE;

        var refreshTok = '';
        try {
            refreshTok = localStorage.getItem('refresh_token') || '';
        } catch (err) {
            console.warn('[session_guard] failed to read refresh_token from localStorage:', err);
        }

        var init = {
            method: 'POST',
            credentials: 'same-origin'
        };
        if (refreshTok) {
            init.headers = { 'Content-Type': 'application/json' };
            init.body = JSON.stringify({ refresh_token: refreshTok });
        }

        // Use the underlying fetch (window.fetch may already be wrapped by
        // installFetchGuard; calling it through the wrapper would re-trigger
        // 401-handling and recurse). We capture the original below in init().
        REFRESH_PROMISE = ORIGINAL_FETCH('/api/auth/refresh', init)
            .then(function (resp) {
                if (!resp || !resp.ok) {
                    var err = new Error('refresh failed: ' + (resp && resp.status));
                    err.status = resp && resp.status;
                    throw err;
                }
                return resp.json();
            })
            .then(function (data) {
                storeTokens(data);
                scheduleProactiveRefresh();
                return data;
            })
            .catch(function (err) {
                // True session lapse. Surface the banner so the user can
                // re-authenticate. Re-throw so callers (e.g. the fetch
                // wrapper's retry path) know not to retry.
                showBanner();
                throw err;
            })
            .then(function (data) {
                REFRESH_PROMISE = null;
                return data;
            }, function (err) {
                REFRESH_PROMISE = null;
                throw err;
            });

        return REFRESH_PROMISE;
    }

    // Schedule the next proactive refresh based on the current token's exp.
    // Re-callable: cancels any pending timer first.
    function scheduleProactiveRefresh() {
        if (EXPIRY_TIMER) {
            clearTimeout(EXPIRY_TIMER);
            EXPIRY_TIMER = null;
        }
        var jwt = getJWT();
        var exp = tokenExp(jwt);
        if (!exp) return;
        var nowSec = Math.floor(Date.now() / 1000);
        if (exp <= nowSec) {
            // Already expired by the time we got here — don't wait.
            attemptRefresh().catch(function () { /* banner shown */ });
            return;
        }
        var delaySec = Math.max(exp - nowSec - REFRESH_LEAD_SECONDS, 1);
        var delayMs = Math.min(delaySec * 1000, TIMER_CAP_MS);
        EXPIRY_TIMER = setTimeout(function () {
            // The timer firing always tries a refresh. If the cap (1h) hit
            // before exp, the refresh will succeed and reschedule. If exp is
            // within REFRESH_LEAD_SECONDS, the refresh extends the session.
            attemptRefresh().catch(function () { /* banner shown */ });
        }, delayMs);
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
        } catch (err) {
            console.warn('[session_guard] failed to clear tokens from localStorage:', err);
        }
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

    // Is this URL one whose 401 should trigger refresh-and-retry? Only
    // protected app endpoints. Login/register/refresh themselves return 401
    // on bad creds — those are user typos, not a session expiry.
    function shouldGuard(url) {
        if (typeof url !== 'string') {
            try { url = String(url); } catch (_) { return false; }
        }
        if (url.indexOf('/api/auth/login') >= 0) return false;
        if (url.indexOf('/api/auth/register') >= 0) return false;
        if (url.indexOf('/api/auth/refresh') >= 0) return false;
        if (url.indexOf('/api/auth/request-reset') >= 0) return false;
        if (url.indexOf('/api/auth/reset-password') >= 0) return false;
        if (url.indexOf('/api/auth/validate-reset-token') >= 0) return false;
        return url.indexOf('/api/') >= 0 || url.indexOf('://') < 0;
    }

    // Captured at install time so attemptRefresh can call the real fetch
    // without re-entering its own 401 handler.
    var ORIGINAL_FETCH = null;

    // Wrap fetch() so any 401 from a protected endpoint triggers refresh +
    // silent retry. If refresh succeeds, the original call is re-issued and
    // the caller sees the retry's response (success or otherwise). If
    // refresh fails, the caller sees the original 401 and the banner is up.
    function installFetchGuard() {
        if (!window.fetch) return;
        ORIGINAL_FETCH = window.fetch.bind(window);
        window.fetch = function (input, init) {
            var url = typeof input === 'string' ? input : (input && input.url) || '';
            return ORIGINAL_FETCH(input, init).then(function (resp) {
                if (!(resp && resp.status === 401 && shouldGuard(url))) {
                    return resp;
                }
                // 401 on a guarded endpoint → try refresh, then retry once.
                return attemptRefresh().then(function () {
                    return ORIGINAL_FETCH(input, init);
                }).catch(function () {
                    // Refresh failed; banner already shown by attemptRefresh.
                    // Hand the original 401 back so the caller's normal
                    // error path still runs.
                    return resp;
                });
            });
        };
    }

    // htmx fires `htmx:responseError` for any non-2xx response. We can't
    // safely auto-retry the original htmx call from here (htmx has already
    // swapped in the error UI / handled the target), so on 401 we just
    // attempt a background refresh — if it succeeds the next user action
    // works without the banner; if it fails the banner fires.
    function installHtmxGuard() {
        if (!document.body) return;
        document.body.addEventListener('htmx:responseError', function (evt) {
            var status = evt.detail && evt.detail.xhr && evt.detail.xhr.status;
            if (status === 401) {
                attemptRefresh().catch(function () { /* banner shown */ });
            }
        });
    }

    function init() {
        installFetchGuard();
        installHtmxGuard();

        var jwt = getJWT();
        var exp = tokenExp(jwt);
        var nowSec = Math.floor(Date.now() / 1000);

        if (jwt && exp > 0 && exp <= nowSec) {
            // Token already expired by the time the page rendered. Try a
            // refresh first — if it succeeds the page just keeps working.
            attemptRefresh().catch(function () { /* banner shown */ });
        } else if (jwt && exp > nowSec) {
            scheduleProactiveRefresh();
        }
        // No token + on /login = nothing to do.
        // No token + elsewhere = the server-side auth middleware will
        // redirect them on the next nav; banner here would be premature.
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
