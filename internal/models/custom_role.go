package models

import (
	"time"

	"github.com/google/uuid"
)

// CustomRole is an admin role defined through the role-builder UI. Lives
// alongside the four built-in SystemRole values (super_admin/support/
// marketing/partner) which are still hardcoded in auth/perm.go.
type CustomRole struct {
	ID             uuid.UUID
	Name           string // machine slug, e.g. "pro_qa" — goes in admin_users.system_role
	DisplayName    string
	Description    string
	CreatedAt      time.Time
	CreatedByEmail string
	UpdatedAt      time.Time
	Permissions    map[string]string // section -> "read"|"write"
}
