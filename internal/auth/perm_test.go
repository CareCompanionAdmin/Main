package auth

import (
	"net/http"
	"testing"

	"carecompanion/internal/models"
)

func TestMatrix_SuperAdminAlwaysFull(t *testing.T) {
	for _, sec := range Sections {
		if got := Matrix(models.SystemRoleSuperAdmin, sec); got != LevelFull {
			t.Errorf("super_admin on %s = %q, want full", sec, got)
		}
	}
	if got := Matrix(models.SystemRoleSuperAdmin, "definitely_not_a_section"); got != LevelFull {
		t.Errorf("super_admin on unknown section = %q, want full", got)
	}
}

func TestMatrix_PartnerLockedRows(t *testing.T) {
	cases := map[string]Level{
		"dashboard":             LevelFull,
		"tickets":               LevelFull,
		"users":                 LevelFull,
		"families":              LevelFull,
		"metrics_dashboard":     LevelFull,
		"copy_materials":        LevelRead,
		"beta_program":          LevelFull,
		"bounty_program":        LevelRead,
		"promo_codes":           LevelRead,
		"infrastructure_status": LevelRead,
		"error_logs":            LevelRead,
		"development_mode":      LevelNone,
		"product_roadmap":       LevelFull,
		"financials":            LevelFull,
		"subscriptions":         LevelFull,
		"admin_users":           LevelRead,
		"system_settings":       LevelNone,
		"audit_log":             LevelNone,
		"version_log":           LevelRead,
		"live_sessions":         LevelFull,
	}
	for sec, want := range cases {
		if got := Matrix(models.SystemRolePartner, sec); got != want {
			t.Errorf("partner on %s = %q, want %q", sec, got, want)
		}
	}
}

func TestMatrix_UnknownSectionForNonSuper(t *testing.T) {
	if got := Matrix(models.SystemRolePartner, "nope"); got != LevelNone {
		t.Errorf("partner on unknown section = %q, want none", got)
	}
}

func TestRequiredLevelForMethod(t *testing.T) {
	cases := map[string]Level{
		http.MethodGet:    LevelRead,
		http.MethodHead:   LevelRead,
		http.MethodPost:   LevelWrite,
		http.MethodPut:    LevelWrite,
		http.MethodPatch:  LevelWrite,
		http.MethodDelete: LevelFull,
	}
	for m, want := range cases {
		if got := RequiredLevelForMethod(m); got != want {
			t.Errorf("method %s = %q, want %q", m, got, want)
		}
	}
}

func TestAllows(t *testing.T) {
	if !Allows(models.SystemRolePartner, "tickets", http.MethodDelete) {
		t.Error("partner should DELETE on tickets")
	}
	if Allows(models.SystemRolePartner, "bounty_program", http.MethodDelete) {
		t.Error("partner must NOT DELETE on bounty_program")
	}
	if !Allows(models.SystemRolePartner, "bounty_program", http.MethodGet) {
		t.Error("partner should GET on bounty_program")
	}
	if Allows(models.SystemRolePartner, "development_mode", http.MethodGet) {
		t.Error("partner must NOT GET on development_mode")
	}
}
