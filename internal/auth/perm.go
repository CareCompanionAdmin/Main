// Package auth holds cross-cutting authorization data — currently the
// section-keyed permission matrix that drives admin route gates and sidebar
// visibility. Single source of truth: editing the matrix here updates every
// consumer.
package auth

import (
	"net/http"
	"strings"

	"carecompanion/internal/models"
)

type Level string

const (
	LevelNone  Level = "none"
	LevelRead  Level = "read"
	LevelWrite Level = "write"
	LevelFull  Level = "full"
)

func levelRank(l Level) int {
	switch l {
	case LevelFull:
		return 3
	case LevelWrite:
		return 2
	case LevelRead:
		return 1
	default:
		return 0
	}
}

// RankAtLeast reports whether `have` meets or exceeds `need`.
func RankAtLeast(have, need Level) bool { return levelRank(have) >= levelRank(need) }

// Sections is the canonical list of section keys. Used by the sidebar so
// section ordering stays stable.
var Sections = []string{
	"dashboard", "tickets", "users", "families",
	"metrics_dashboard", "copy_materials", "beta_program", "bounty_program",
	"promo_codes", "infrastructure_status", "error_logs", "development_mode",
	"product_roadmap", "financials", "subscriptions",
	"admin_users", "system_settings", "audit_log", "version_log",
	"live_sessions",
}

// matrix encodes the locked 2026-05-09 permission table. Roles not listed for
// a section default to LevelNone. super_admin is in every row at LevelFull
// belt-and-suspenders, but Matrix() also short-circuits super_admin
// independent of the table.
var matrix = map[string]map[models.SystemRole]Level{
	"dashboard": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"tickets": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRoleMarketing:  LevelRead,
		models.SystemRolePartner:    LevelFull,
	},
	"users": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"families": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"metrics_dashboard": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"copy_materials": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"beta_program": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"bounty_program": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"promo_codes": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleMarketing:  LevelRead,
		models.SystemRolePartner:    LevelRead,
	},
	"infrastructure_status": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"error_logs": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"development_mode": {
		models.SystemRoleSuperAdmin: LevelFull,
	},
	"product_roadmap": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"financials": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"subscriptions": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
	"admin_users": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"system_settings": {
		models.SystemRoleSuperAdmin: LevelFull,
	},
	"audit_log": {
		models.SystemRoleSuperAdmin: LevelFull,
	},
	"version_log": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRolePartner:    LevelRead,
	},
	"live_sessions": {
		models.SystemRoleSuperAdmin: LevelFull,
		models.SystemRoleSupport:    LevelFull,
		models.SystemRolePartner:    LevelFull,
	},
}

// Matrix returns the access level for (role, section). Super admin is always
// LevelFull regardless of the table. Unknown role or unknown section returns
// LevelNone — fail closed.
func Matrix(role models.SystemRole, section string) Level {
	if role == models.SystemRoleSuperAdmin {
		return LevelFull
	}
	row, ok := matrix[section]
	if !ok {
		return LevelNone
	}
	if lvl, ok := row[role]; ok {
		return lvl
	}
	return LevelNone
}

// RequiredLevelForMethod maps an HTTP method to the level required to call
// it. GET/HEAD/OPTIONS need read; POST/PUT/PATCH need write; DELETE needs
// full.
func RequiredLevelForMethod(method string) Level {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return LevelRead
	case http.MethodDelete:
		return LevelFull
	default:
		return LevelWrite
	}
}

// Allows is the high-level helper: does (role) have permission to perform
// (method) on (section)?
func Allows(role models.SystemRole, section, method string) bool {
	return RankAtLeast(Matrix(role, section), RequiredLevelForMethod(method))
}
