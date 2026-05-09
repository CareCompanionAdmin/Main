package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"carecompanion/internal/middleware"
)

func TestResolveCookieNames_PathSelectsCookieKind(t *testing.T) {
	cases := []struct {
		path  string
		first string
	}{
		{"/admin/dashboard", "admin_access_token"},
		{"/admin/login", "admin_access_token"},
		{"/api/admin/users", "admin_access_token"},
		{"/api/children/123", "user_access_token"},
		{"/dashboard", "user_access_token"},
		{"/", "user_access_token"},
	}
	for _, c := range cases {
		got := middleware.ResolveCookieNamesForTest(c.path)
		if len(got) != 2 {
			t.Fatalf("path=%s len=%d, want 2", c.path, len(got))
		}
		if got[0] != c.first {
			t.Errorf("path=%s first=%s want %s", c.path, got[0], c.first)
		}
		if got[1] != "access_token" {
			t.Errorf("path=%s legacy=%s want access_token", c.path, got[1])
		}
	}
}

// Smoke test: middleware rejects a request with no auth cookie regardless of
// path. Uses a nil AuthService — the no-token branch returns before any
// service call, so nil is safe here.
func TestAuthMiddleware_RejectsMissingCookie(t *testing.T) {
	mw := middleware.AuthMiddleware(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	req := httptest.NewRequest("GET", "/api/children/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no_token") {
		t.Fatalf("body = %q, want to contain no_token", rec.Body.String())
	}
}
