package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

func reqWithRole(method, path string, role models.SystemRole) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	claims := &service.AuthClaims{SystemRole: role}
	ctx := context.WithValue(r.Context(), middleware.AuthClaimsKey, claims)
	return r.WithContext(ctx)
}

func TestRequireSection_SuperAdminBypass(t *testing.T) {
	called := false
	mw := middleware.RequireSection("development_mode")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithRole("GET", "/x", models.SystemRoleSuperAdmin))
	if !called {
		t.Fatalf("super_admin should pass; got status %d", rec.Code)
	}
}

func TestRequireSection_PartnerAllowedOnFull(t *testing.T) {
	for _, m := range []string{"GET", "POST", "DELETE"} {
		called := false
		mw := middleware.RequireSection("tickets")
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, reqWithRole(m, "/x", models.SystemRolePartner))
		if !called {
			t.Errorf("partner %s tickets denied (status %d), want pass", m, rec.Code)
		}
	}
}

func TestRequireSection_PartnerDeniedOnNone(t *testing.T) {
	mw := middleware.RequireSection("development_mode")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqWithRole("GET", "/x", models.SystemRolePartner))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireSection_PartnerReadAllowsGetDeniesPost(t *testing.T) {
	mw := middleware.RequireSection("admin_users")
	getCalled := false
	hGet := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { getCalled = true }))
	rec := httptest.NewRecorder()
	hGet.ServeHTTP(rec, reqWithRole("GET", "/x", models.SystemRolePartner))
	if !getCalled {
		t.Errorf("partner GET admin_users denied (status %d), want pass", rec.Code)
	}

	hPost := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	rec2 := httptest.NewRecorder()
	hPost.ServeHTTP(rec2, reqWithRole("POST", "/x", models.SystemRolePartner))
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("partner POST admin_users status = %d, want 403", rec2.Code)
	}
}

func TestRequireSection_NoClaims401(t *testing.T) {
	mw := middleware.RequireSection("tickets")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
