package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"-"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	Phone           NullString `json:"phone,omitempty"`
	Timezone        NullString `json:"timezone,omitempty"`
	Status          UserStatus `json:"status"`
	EmailVerifiedAt NullTime   `json:"email_verified_at,omitempty"`
	LastLoginAt     NullTime   `json:"last_login_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (u *User) IsEmailVerified() bool {
	return u.EmailVerifiedAt.Valid
}

func (u *User) FullName() string {
	if u.LastName != "" {
		return u.FirstName + " " + u.LastName
	}
	return u.FirstName
}

type Family struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedBy uuid.UUID `json:"created_by"`
	Settings  JSONB     `json:"settings"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type FamilyMembership struct {
	ID          uuid.UUID  `json:"id"`
	FamilyID    uuid.UUID  `json:"family_id"`
	UserID      uuid.UUID  `json:"user_id"`
	Role        FamilyRole `json:"role"`
	Permissions JSONB      `json:"permissions,omitempty"`
	InvitedBy   NullUUID   `json:"invited_by,omitempty"`
	InvitedAt   NullTime   `json:"invited_at,omitempty"`
	AcceptedAt  NullTime   `json:"accepted_at,omitempty"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Populated when needed
	User   *User   `json:"user,omitempty"`
	Family *Family `json:"family,omitempty"`
}

type FamilyWithRole struct {
	FamilyID   uuid.UUID  `json:"family_id"`
	FamilyName string     `json:"family_name"`
	Role       FamilyRole `json:"role"`
	IsActive   bool       `json:"is_active"`
}

type UserWithFamilies struct {
	User
	Families []FamilyMembership `json:"families"`
}

// Request/Response types
type CreateUserRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	User         UserWithFamilies `json:"user"`
	AccessToken  string           `json:"access_token"`
	RefreshToken string           `json:"refresh_token"`
	ExpiresAt    time.Time        `json:"expires_at"`
}

type SetFamilyContextRequest struct {
	FamilyID uuid.UUID `json:"family_id"`
}

type UpdateProfileRequest struct {
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	Phone     *string `json:"phone,omitempty"`
}

type FamilyContextResponse struct {
	AccessToken string     `json:"access_token"`
	FamilyID    uuid.UUID  `json:"family_id"`
	FamilyName  string     `json:"family_name"`
	Role        FamilyRole `json:"role"`
	ExpiresAt   time.Time  `json:"expires_at"`
}
