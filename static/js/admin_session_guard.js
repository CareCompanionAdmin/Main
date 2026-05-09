// admin_session_guard.js — admin-portal variant of session_guard.js.
//
// Same architecture as the user-side guard, parameterized for admin
// cookies/endpoints/path predicate. Loaded ONLY on admin pages (the user-side
// guard is not loaded there, so the two don't fight over window.fetch).

(function () {
    'use strict';

    var SHOWN = false;
    var EXPIRY_TIMER = null;
    var REFRESH_PROMISE = null;

    var REFRESH_LEAD_SECONDS = 5 * 60;
    var TIMER_CAP_MS = 60 * 60 * 1000;

    function getCookie(name) {
        var v = '; ' + document.cookie;
        var parts = v.split('; ' + name + '=');
        return parts.length === 2 ? parts.pop().split(';').shift() : '';
    }

    // The admin access token is HttpOnly so document.cookie won't see it. We
    // still try (in case the deployment relaxes that), and fall back to a
    // server-supplied window.__ADMIN_TOKEN_EXP if present (not currently set
    // — left as a future hook). Without an exp, we simply skip the proactive
    // timer; reactive 401 handling still works.
    function getJWT() {
        return getCookie('admin_access_token') || '';
    }

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

    var ORIGINAL_FETCH = null;

    // Single-flight refresh. The HttpOnly admin_refresh_token cookie is sent
    // automatically because the request URL matches its Path. No body needed.
    function attemptRefresh() {
        if (REFRESH_PROMISE) return REFRESH_PROMISE;

        REFRESH_PROMISE = ORIGINAL_FETCH('/api/admin/auth/refresh', {
            method: 'POST',
            credentials: 'same-origin'
        })
            .then(function (resp) {
                if (!resp || !resp.ok) {
                    var err = new Error('admin refresh failed: ' + (resp && resp.status));
                    err.status = resp && resp.status;
                    throw err;
                }
                return resp.json();
            })
            .then(function (data) {
                scheduleProactiveRefresh();
                return data;
            })
            .catch(function (err) {
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
            attemptRefresh().catch(function () { /* banner shown */ });
            return;
        }
        var delaySec = Math.max(exp - nowSec - REFRESH_LEAD_SECONDS, 1);
        var delayMs = Math.min(delaySec * 1000, TIMER_CAP_MS);
        EXPIRY_TIMER = setTimeout(function () {
            attemptRefresh().catch(function () { /* banner shown */ });
        }, delayMs);
    }

    function showBanner() {
        if (SHOWN) return;
        SHOWN = true;
        if (window.location.pathname === '/admin/login') return;

        var ret = window.location.pathname + window.location.search;
        var loginHref = '/admin/login?return=' + encodeURIComponent(ret);

        var existing = document.getElementById('admin-session-expired-banner');
        if (existing) existing.remove();

        var banner = document.createElement('div');
        banner.id = 'admin-session-expired-banner';
        banner.className = 'border-b border-red-300 bg-red-50 text-red-900 ' +
            'px-4 py-2 text-sm sticky top-0 z-50';
        banner.innerHTML = '<div class="max-w-7xl mx-auto flex items-center gap-3">' +
            '<svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">' +
            '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" ' +
            'd="M12 9v2m0 4h.01M5.07 19h13.86c1.54 0 2.5-1.67 1.73-3L13.73 4c-.77-1.33-2.69-1.33-3.46 0L3.34 16c-.77 1.33.19 3 1.73 3z" />' +
            '</svg>' +
            '<div class="flex-1">Your admin session has expired or been terminated. Sign in again to continue.</div>' +
            '<a href="' + loginHref + '" class="inline-flex items-center px-3 py-1 rounded-md bg-red-600 hover:bg-red-700 text-white font-medium whitespace-nowrap">' +
            'Sign in again</a>' +
            '</div>';
        document.body.insertBefore(banner, document.body.firstChild);
    }

    // Guard admin API and admin UI HTML routes. Skip the auth endpoints
    // themselves — login failures and the refresh call must not recurse.
    function shouldGuard(url) {
        if (typeof url !== 'string') {
            try { url = String(url); } catch (_) { return false; }
        }
        if (url.indexOf('/api/admin/auth/refresh') >= 0) return false;
        if (url.indexOf('/admin/login') >= 0) return false;
        if (url.indexOf('/admin/logout') >= 0) return false;
        if (url.indexOf('/api/admin/') >= 0) return true;
        // Same-origin admin UI HTML — relative URLs and absolute /admin/* hits.
        if (url.indexOf('://') < 0 && url.indexOf('/admin/') === 0) return true;
        return false;
    }

    function installFetchGuard() {
        if (!window.fetch) return;
        ORIGINAL_FETCH = window.fetch.bind(window);
        window.fetch = function (input, init) {
            var url = typeof input === 'string' ? input : (input && input.url) || '';
            return ORIGINAL_FETCH(input, init).then(function (resp) {
                if (!(resp && resp.status === 401 && shouldGuard(url))) {
                    return resp;
                }
                return attemptRefresh().then(function () {
                    return ORIGINAL_FETCH(input, init);
                }).catch(function () {
                    return resp;
                });
            });
        };
    }

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
            attemptRefresh().catch(function () { /* banner shown */ });
        } else if (jwt && exp > nowSec) {
            scheduleProactiveRefresh();
        }
        // No readable token (HttpOnly is the default in this app): proactive
        // path is skipped, reactive 401 wrapper still does the work.
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
