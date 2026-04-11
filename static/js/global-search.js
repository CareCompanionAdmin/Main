/**
 * MyCareCompanion Global Search
 * Self-contained search input with dropdown results.
 * Include this script on any page with a #global-search-input element.
 */
(function() {
    'use strict';

    var input = document.getElementById('global-search-input');
    var dropdown = document.getElementById('global-search-dropdown');
    if (!input || !dropdown) return;

    var debounceTimer = null;
    var currentQuery = '';
    var activeIndex = -1;

    // Category icons (SVG paths)
    var categoryIcons = {
        behavior: 'M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
        sleep: 'M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z',
        diet: 'M3 3h2l.4 2M7 13h10l4-8H5.4M7 13L5.4 5M7 13l-2.293 2.293c-.63.63-.184 1.707.707 1.707H17m0 0a2 2 0 100 4 2 2 0 000-4zm-8 2a2 2 0 11-4 0 2 2 0 014 0z',
        medications: 'M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z',
        medication_logs: 'M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.806.547M8 4h8l-1 1v5.172a2 2 0 00.586 1.414l5 5c1.26 1.26.367 3.414-1.415 3.414H4.828c-1.782 0-2.674-2.154-1.414-3.414l5-5A2 2 0 009 10.172V5L8 4z',
        therapy: 'M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z',
        health_events: 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
        sensory: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z',
        social: 'M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z',
        seizure: 'M13 10V3L4 14h7v7l9-11h-7z',
        speech: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z',
        bowel: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2',
        chat: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z',
        alerts: 'M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9'
    };

    input.addEventListener('input', function() {
        var q = input.value.trim();
        clearTimeout(debounceTimer);
        activeIndex = -1;
        if (q.length < 2) {
            dropdown.classList.add('hidden');
            dropdown.innerHTML = '';
            return;
        }
        debounceTimer = setTimeout(function() { doSearch(q); }, 300);
    });

    input.addEventListener('focus', function() {
        if (dropdown.innerHTML && input.value.trim().length >= 2) {
            dropdown.classList.remove('hidden');
        }
    });

    document.addEventListener('click', function(e) {
        if (!input.contains(e.target) && !dropdown.contains(e.target)) {
            dropdown.classList.add('hidden');
        }
    });

    input.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') {
            dropdown.classList.add('hidden');
            input.blur();
            return;
        }
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            navigateResults(e.key === 'ArrowDown' ? 1 : -1);
            return;
        }
        if (e.key === 'Enter') {
            var active = dropdown.querySelector('.search-result-active');
            if (active) {
                e.preventDefault();
                window.location.href = active.getAttribute('href');
            }
        }
    });

    function doSearch(q) {
        currentQuery = q;
        fetch('/api/search?q=' + encodeURIComponent(q), { credentials: 'include' })
            .then(function(r) { return r.json(); })
            .then(function(data) {
                if (q !== currentQuery) return;
                renderResults(data);
            })
            .catch(function() {});
    }

    function renderResults(data) {
        if (!data.categories || data.total_count === 0) {
            dropdown.innerHTML = '<div class="px-4 py-3 text-sm text-gray-500">No results found</div>';
            dropdown.classList.remove('hidden');
            return;
        }

        var html = '';
        data.categories.forEach(function(cat) {
            if (!cat.results || cat.results.length === 0) return;
            var iconPath = categoryIcons[cat.key] || categoryIcons.alerts;
            html += '<div class="px-3 py-2 text-xs font-semibold text-gray-500 uppercase tracking-wider bg-gray-50 dark:bg-gray-900/50 flex items-center">';
            html += '<svg class="w-3.5 h-3.5 mr-1.5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="' + iconPath + '"></path></svg>';
            html += escapeHtml(cat.name) + '</div>';

            cat.results.forEach(function(r) {
                html += '<a href="' + escapeHtml(r.url) + '" class="search-result flex items-start px-3 py-2.5 hover:bg-indigo-50 dark:hover:bg-gray-700/50 cursor-pointer border-b border-gray-100 dark:border-gray-700/50 last:border-0 transition-colors" data-url="' + escapeHtml(r.url) + '">';
                html += '<div class="flex-1 min-w-0">';
                html += '<div class="text-sm text-gray-900 dark:text-gray-100 truncate">' + highlightMatch(escapeHtml(r.snippet), currentQuery) + '</div>';
                html += '<div class="text-xs text-gray-400 mt-0.5">';
                if (r.child_name) html += escapeHtml(r.child_name) + ' &middot; ';
                html += escapeHtml(r.date);
                html += '</div></div>';
                html += '<svg class="w-4 h-4 text-gray-300 ml-2 mt-1 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path></svg>';
                html += '</a>';
            });
        });

        dropdown.innerHTML = html;
        dropdown.classList.remove('hidden');
    }

    function highlightMatch(text, query) {
        if (!query) return text;
        var escaped = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        var regex = new RegExp('(' + escaped + ')', 'gi');
        return text.replace(regex, '<mark class="bg-yellow-200 dark:bg-yellow-700/50 text-inherit rounded px-0.5">$1</mark>');
    }

    function navigateResults(dir) {
        var items = dropdown.querySelectorAll('.search-result');
        if (items.length === 0) return;
        items.forEach(function(el) { el.classList.remove('search-result-active', 'bg-indigo-50'); });
        activeIndex += dir;
        if (activeIndex < 0) activeIndex = items.length - 1;
        if (activeIndex >= items.length) activeIndex = 0;
        items[activeIndex].classList.add('search-result-active', 'bg-indigo-50');
        items[activeIndex].scrollIntoView({ block: 'nearest' });
    }

    function escapeHtml(s) {
        if (!s) return '';
        var div = document.createElement('div');
        div.textContent = s;
        return div.innerHTML;
    }

    // Highlight-on-navigate: scroll to and flash a highlighted element
    var params = new URLSearchParams(window.location.search);
    var highlight = params.get('highlight');
    if (highlight) {
        setTimeout(function() {
            var el = document.getElementById(highlight);
            if (el) {
                el.scrollIntoView({ behavior: 'smooth', block: 'center' });
                el.style.outline = '3px solid #4F46E5';
                el.style.outlineOffset = '4px';
                el.style.borderRadius = '8px';
                el.style.transition = 'outline-color 0.5s';
                setTimeout(function() {
                    el.style.outlineColor = 'transparent';
                    setTimeout(function() { el.style.outline = ''; el.style.outlineOffset = ''; }, 500);
                }, 2500);
            }
        }, 300);
    }

    // Also handle hash-based highlights (#med-uuid, #alert-uuid)
    if (window.location.hash) {
        setTimeout(function() {
            var el = document.querySelector(window.location.hash);
            if (el) {
                el.style.outline = '3px solid #4F46E5';
                el.style.outlineOffset = '4px';
                el.style.transition = 'outline-color 0.5s';
                setTimeout(function() {
                    el.style.outlineColor = 'transparent';
                }, 2500);
            }
        }, 300);
    }
})();
