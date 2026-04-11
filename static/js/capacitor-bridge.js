/**
 * MyCareCompanion Capacitor Bridge
 * Connects the web app to native mobile features when running inside the Capacitor shell.
 * All calls are no-ops when running in a regular browser.
 */
(function() {
    'use strict';

    // Only initialize if running inside Capacitor
    if (!window.Capacitor || !window.Capacitor.isNativePlatform()) {
        return;
    }

    console.log('[Capacitor] Native platform detected:', window.Capacitor.getPlatform());

    // Add capacitor class to body for CSS safe-area handling
    document.body.classList.add('capacitor');
    document.body.classList.add('capacitor-' + window.Capacitor.getPlatform());

    // =========================================================================
    // Status Bar
    // =========================================================================
    if (window.Capacitor.Plugins.StatusBar) {
        var StatusBar = window.Capacitor.Plugins.StatusBar;
        StatusBar.setBackgroundColor({ color: '#3b82f6' }).catch(function() {});
        StatusBar.setStyle({ style: 'LIGHT' }).catch(function() {});
    }

    // =========================================================================
    // Push Notifications
    // =========================================================================
    var PushNotifications = window.Capacitor.Plugins.PushNotifications;
    if (PushNotifications) {
        // Request permission and register for push notifications
        PushNotifications.requestPermissions().then(function(result) {
            if (result.receive === 'granted') {
                PushNotifications.register();
            } else {
                console.log('[Capacitor] Push notification permission denied');
            }
        });

        // Handle registration success - send token to backend
        PushNotifications.addListener('registration', function(token) {
            console.log('[Capacitor] Push token received');
            registerDeviceToken(token.value);
        });

        // Handle registration error
        PushNotifications.addListener('registrationError', function(err) {
            console.error('[Capacitor] Push registration error:', err);
        });

        // Handle notification received while app is in foreground
        PushNotifications.addListener('pushNotificationReceived', function(notification) {
            console.log('[Capacitor] Push received in foreground:', notification.title);
            showInAppNotification(notification);
        });

        // Handle notification tap (app opened from notification)
        PushNotifications.addListener('pushNotificationActionPerformed', function(action) {
            console.log('[Capacitor] Push notification tapped');
            var data = action.notification.data;
            if (data && data.type) {
                handleDeepLink(data);
            }
        });
    }

    // =========================================================================
    // Network Status
    // =========================================================================
    var Network = window.Capacitor.Plugins.Network;
    if (Network) {
        Network.addListener('networkStatusChange', function(status) {
            var banner = document.getElementById('capacitor-offline-banner');
            if (!status.connected) {
                if (!banner) {
                    banner = document.createElement('div');
                    banner.id = 'capacitor-offline-banner';
                    banner.style.cssText = 'position:fixed;top:0;left:0;right:0;z-index:99999;' +
                        'background:#EF4444;color:white;text-align:center;padding:8px;font-size:14px;' +
                        'padding-top:calc(8px + env(safe-area-inset-top))';
                    banner.textContent = 'No internet connection';
                    document.body.prepend(banner);
                }
            } else if (banner) {
                banner.remove();
            }
        });
    }

    // =========================================================================
    // App Lifecycle
    // =========================================================================
    var App = window.Capacitor.Plugins.App;
    if (App) {
        // Handle back button on Android
        App.addListener('backButton', function(data) {
            if (window.history.length > 1) {
                window.history.back();
            } else {
                App.minimizeApp();
            }
        });
    }

    // =========================================================================
    // External Links
    // =========================================================================
    var Browser = window.Capacitor.Plugins.Browser;
    if (Browser) {
        document.addEventListener('click', function(e) {
            var link = e.target.closest('a[href]');
            if (!link) return;

            var href = link.getAttribute('href');
            if (!href) return;

            // Open external links in system browser
            var isExternal = link.target === '_blank' ||
                (href.startsWith('http') &&
                 !href.includes('mycarecompanion.net') &&
                 !href.includes('98.88.131.147'));

            if (isExternal) {
                e.preventDefault();
                Browser.open({ url: href });
            }
        }, true);
    }

    // =========================================================================
    // Environment Switcher (QA testers - tap version 7 times)
    // =========================================================================
    var envTapCount = 0;
    var envTapTimer = null;
    var ENVIRONMENTS = {
        production: 'https://www.mycarecompanion.net',
        development: 'http://98.88.131.147:8090'
    };

    window.carecompanionEnvSwitch = function(element) {
        envTapCount++;
        clearTimeout(envTapTimer);
        envTapTimer = setTimeout(function() { envTapCount = 0; }, 2000);

        if (envTapCount >= 7) {
            envTapCount = 0;
            showEnvironmentSwitcher();
        }
    };

    function showEnvironmentSwitcher() {
        var Preferences = window.Capacitor.Plugins.Preferences;
        if (!Preferences) return;

        Preferences.get({ key: 'server_url' }).then(function(result) {
            var currentUrl = result.value || ENVIRONMENTS.production;
            var currentEnv = currentUrl.includes('98.88.131.147') ? 'development' : 'production';
            var targetEnv = currentEnv === 'production' ? 'development' : 'production';

            if (confirm('Switch from ' + currentEnv.toUpperCase() + ' to ' + targetEnv.toUpperCase() + '?\n\n' +
                         'Current: ' + currentUrl + '\n' +
                         'Switch to: ' + ENVIRONMENTS[targetEnv])) {
                Preferences.set({ key: 'server_url', value: ENVIRONMENTS[targetEnv] }).then(function() {
                    window.location.href = ENVIRONMENTS[targetEnv];
                });
            }
        });
    }

    // Show environment banner if on dev
    function showEnvironmentBanner() {
        var Preferences = window.Capacitor.Plugins.Preferences;
        if (!Preferences) return;

        Preferences.get({ key: 'server_url' }).then(function(result) {
            if (result.value && result.value.includes('98.88.131.147')) {
                var banner = document.createElement('div');
                banner.style.cssText = 'position:fixed;bottom:0;left:0;right:0;z-index:99998;' +
                    'background:#F59E0B;color:#000;text-align:center;padding:4px;font-size:12px;font-weight:bold;' +
                    'padding-bottom:calc(4px + env(safe-area-inset-bottom))';
                banner.textContent = 'DEVELOPMENT ENVIRONMENT';
                document.body.appendChild(banner);
            }
        });
    }

    // =========================================================================
    // Helper Functions
    // =========================================================================

    function registerDeviceToken(token) {
        var platform = window.Capacitor.getPlatform(); // 'ios' or 'android'
        fetch('/api/devices/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                token: token,
                platform: platform,
                device_name: platform === 'ios' ? 'iPhone' : 'Android'
            })
        }).then(function(resp) {
            if (resp.ok) {
                console.log('[Capacitor] Device token registered');
            } else if (resp.status === 401) {
                // Not logged in yet - will retry after login
                console.log('[Capacitor] Not authenticated, will register token after login');
            }
        }).catch(function(err) {
            console.error('[Capacitor] Failed to register device token:', err);
        });
    }

    function showInAppNotification(notification) {
        var toast = document.createElement('div');
        toast.style.cssText = 'position:fixed;top:env(safe-area-inset-top,20px);left:16px;right:16px;' +
            'z-index:99999;background:white;border-radius:12px;padding:16px;box-shadow:0 4px 20px rgba(0,0,0,0.15);' +
            'cursor:pointer;transition:opacity 0.3s;border-left:4px solid #3b82f6';
        toast.innerHTML = '<div style="font-weight:600;font-size:14px;margin-bottom:4px">' +
            escapeHtml(notification.title || '') + '</div>' +
            '<div style="font-size:13px;color:#6B7280">' + escapeHtml(notification.body || '') + '</div>';
        toast.onclick = function() {
            toast.remove();
            if (notification.data) {
                handleDeepLink(notification.data);
            }
        };
        document.body.appendChild(toast);
        setTimeout(function() {
            toast.style.opacity = '0';
            setTimeout(function() { toast.remove(); }, 300);
        }, 5000);
    }

    function handleDeepLink(data) {
        switch (data.type) {
            case 'chat_message':
                window.location.href = '/chat';
                break;
            case 'alert':
                if (data.child_id) {
                    window.location.href = '/child/' + data.child_id + '/alerts';
                }
                break;
            case 'ticket_reply':
                window.location.href = '/support';
                break;
            case 'family_added':
                window.location.href = '/dashboard';
                break;
            case 'medication_reminder':
                if (data.child_id) {
                    window.location.href = '/child/' + data.child_id + '/medications';
                }
                break;
            default:
                window.location.href = '/dashboard';
        }
    }

    function escapeHtml(text) {
        var div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Re-register push token after login (listen for page navigations to dashboard)
    var lastPath = window.location.pathname;
    setInterval(function() {
        if (window.location.pathname !== lastPath) {
            lastPath = window.location.pathname;
            if (lastPath === '/dashboard' && PushNotifications) {
                // User just logged in, re-register token
                PushNotifications.register();
            }
        }
    }, 1000);

    // Initialize environment banner on load
    document.addEventListener('DOMContentLoaded', showEnvironmentBanner);
})();
