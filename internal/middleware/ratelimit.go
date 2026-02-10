package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type ipEntry struct {
	count    int
	windowStart time.Time
}

// RateLimiter provides simple per-IP rate limiting.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
	limit   int
	window  time.Duration
}

// NewRateLimiter creates a rate limiter that allows `limit` requests per `window` per IP.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*ipEntry),
		limit:   limit,
		window:  window,
	}
	// Periodically clean up expired entries to prevent memory growth
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window * 2)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, entry := range rl.entries {
			if now.Sub(entry.windowStart) > rl.window {
				delete(rl.entries, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.entries[ip]

	if !exists || now.Sub(entry.windowStart) > rl.window {
		rl.entries[ip] = &ipEntry{count: 1, windowStart: now}
		return true
	}

	entry.count++
	return entry.count <= rl.limit
}

// RateLimit returns middleware that limits requests per IP.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	limiter := NewRateLimiter(limit, window)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}

			// Also check X-Forwarded-For for clients behind ALB
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				// Use the first IP (original client)
				if idx := len(forwarded); idx > 0 {
					parts := splitFirst(forwarded, ",")
					ip = parts
				}
			}

			if !limiter.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"Too many requests. Please try again later."}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func splitFirst(s, sep string) string {
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			return s[:i]
		}
	}
	return s
}
