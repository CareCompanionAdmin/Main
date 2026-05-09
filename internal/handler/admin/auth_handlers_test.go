package admin_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"carecompanion/internal/handler/admin"
)

// We test three pure-handler concerns here without spinning up the full
// service:
//   1. Empty body + missing cookie → 400
//   2. Unparseable JSON body + missing cookie → 400
//   3. Body present but blank refresh + missing cookie → 400 (treated as missing)
//
// The signature-validation path is exercised by the existing RefreshToken
// service tests; we don't re-test it here.

func TestAdminRefreshToken_NoCookieNoBody_Returns400(t *testing.T) {
	h := admin.NewHandler(nil, nil)
	req := httptest.NewRequest("POST", "/api/admin/auth/refresh", nil)
	rec := httptest.NewRecorder()
	h.AdminRefreshToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminRefreshToken_GarbageBodyNoCookie_Returns400(t *testing.T) {
	h := admin.NewHandler(nil, nil)
	req := httptest.NewRequest("POST", "/api/admin/auth/refresh", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.AdminRefreshToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminRefreshToken_EmptyRefreshFieldNoCookie_Returns400(t *testing.T) {
	h := admin.NewHandler(nil, nil)
	req := httptest.NewRequest("POST", "/api/admin/auth/refresh", strings.NewReader(`{"refresh_token":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.AdminRefreshToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
