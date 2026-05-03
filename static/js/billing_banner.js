// billing_banner.js — renders the trial / past-due banner on every authenticated page.
//
// Data source: /api/auth/me — already includes the entitlement block computed
// by middleware.LoadEntitlement. We render the banner only for `parent` role
// users (per the billing spec — admins, doctors, caregivers don't see it).
//
// Banner shows when ANY of:
//   - entitlement.mode == "read_only" (trial lapsed or payment failed)
//   - entitlement.mode == "blocked"
//   - entitlement.mode == "full" AND trial_end is within 7 days
//
// Dismiss: stored in sessionStorage keyed by trial_end / period_end so the
// banner re-shows in a new session OR when the trial deadline changes (e.g.
// user added a 2nd child and got a fresh 14-day trial → new key, banner
// re-shows). Admin users (is_admin=true) never see it.

(function () {
    'use strict';

    function getAuthToken() {
        return localStorage.getItem('access_token') ||
            (function () {
                var v = '; ' + document.cookie;
                var parts = v.split('; access_token=');
                return parts.length === 2 ? parts.pop().split(';').shift() : '';
            })();
    }

    function fmtDateTime(iso) {
        try {
            var d = new Date(iso);
            return d.toLocaleString([], { weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
        } catch (_) { return iso; }
    }

    function humanRemaining(iso) {
        var deadline = new Date(iso).getTime();
        var ms = deadline - Date.now();
        if (ms <= 0) return 'now';
        var hours = ms / (1000 * 60 * 60);
        if (hours < 24) return Math.max(1, Math.round(hours)) + ' hours';
        var days = hours / 24;
        return (Math.round(days * 10) / 10) + ' days';
    }

    function bannerCopy(ent) {
        var mode = ent.mode || '';
        if (mode === 'blocked') {
            return {
                tone: 'red',
                msg: 'Your subscription has expired. Subscribe now to keep using MyCareCompanion.',
                cta: 'Subscribe',
                key: 'blocked'
            };
        }
        if (mode === 'read_only') {
            var until = ent.read_only_until;
            return {
                tone: 'amber',
                msg: 'Your account is read-only. Subscribe within ' + humanRemaining(until) +
                    ' (by ' + fmtDateTime(until) + ') to avoid losing access.',
                cta: 'Subscribe',
                key: 'readonly:' + until
            };
        }
        if (mode === 'full' && ent.trial_end) {
            var trialEnd = new Date(ent.trial_end).getTime();
            var daysLeft = (trialEnd - Date.now()) / (1000 * 60 * 60 * 24);
            if (daysLeft > 0 && daysLeft <= 7) {
                var unit = daysLeft >= 1 ? humanRemaining(ent.trial_end) + ' until expiration'
                                          : humanRemaining(ent.trial_end) + ' until expiration';
                return {
                    tone: 'amber',
                    msg: unit + ' (' + fmtDateTime(ent.trial_end) + '). Subscribe to keep your data.',
                    cta: 'Subscribe',
                    key: 'trial:' + ent.trial_end
                };
            }
        }
        return null;
    }

    function render(ent, role) {
        if (role !== 'parent') return;
        if (ent.is_admin) return;
        var copy = bannerCopy(ent);
        if (!copy) return;
        var dismissedFor = sessionStorage.getItem('bcc.bannerDismissed');
        if (dismissedFor === copy.key) return;

        var existing = document.getElementById('billing-banner');
        if (existing) existing.remove();

        var bgClass = copy.tone === 'red'
            ? 'bg-red-50 dark:bg-red-900/30 border-red-200 dark:border-red-800 text-red-900 dark:text-red-100'
            : 'bg-amber-50 dark:bg-amber-900/30 border-amber-200 dark:border-amber-800 text-amber-900 dark:text-amber-100';

        var banner = document.createElement('div');
        banner.id = 'billing-banner';
        banner.className = 'border-b ' + bgClass + ' px-4 py-2 text-sm';
        banner.innerHTML = '<div class="max-w-7xl mx-auto flex items-center gap-3">' +
            '<svg class="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">' +
            '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>' +
            '</svg>' +
            '<div class="flex-1">' + copy.msg + '</div>' +
            '<a href="/settings" class="font-medium underline hover:no-underline whitespace-nowrap">' + copy.cta + '</a>' +
            '<button id="billing-banner-dismiss" class="text-current opacity-60 hover:opacity-100 ml-2" aria-label="Dismiss">×</button>' +
            '</div>';
        document.body.insertBefore(banner, document.body.firstChild);
        document.getElementById('billing-banner-dismiss').addEventListener('click', function () {
            sessionStorage.setItem('bcc.bannerDismissed', copy.key);
            banner.remove();
        });
    }

    function init() {
        var tok = getAuthToken();
        if (!tok) return; // Not logged in — no banner.
        fetch('/api/auth/me', {
            headers: { 'Authorization': 'Bearer ' + tok },
            credentials: 'include'
        }).then(function (r) {
            if (!r.ok) return null;
            return r.json();
        }).then(function (data) {
            if (!data || !data.entitlement) return;
            render(data.entitlement, data.role || '');
        }).catch(function () { /* silent — banner is best-effort */ });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
