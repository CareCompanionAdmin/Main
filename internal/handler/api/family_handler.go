package api

import (
	"context"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type FamilyHandler struct {
	familyService *service.FamilyService
	userService   *service.UserService
	emailService  *service.EmailService
	pushService   *service.PushService
	appURL        string
}

func NewFamilyHandler(familyService *service.FamilyService, userService *service.UserService, emailService *service.EmailService, pushService *service.PushService, appURL string) *FamilyHandler {
	return &FamilyHandler{
		familyService: familyService,
		userService:   userService,
		emailService:  emailService,
		pushService:   pushService,
		appURL:        appURL,
	}
}

// GetInfo returns family information including creator ID
func (h *FamilyHandler) GetInfo(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())

	family, err := h.familyService.GetByID(r.Context(), familyID)
	if err != nil {
		respondInternalError(w, "Failed to get family info")
		return
	}
	if family == nil {
		respondNotFound(w, "Family not found")
		return
	}

	respondOK(w, family)
}

// ListMembers returns all members of the current family
func (h *FamilyHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())

	members, err := h.familyService.GetMembers(r.Context(), familyID)
	if err != nil {
		respondInternalError(w, "Failed to get family members")
		return
	}

	respondOK(w, members)
}

// AddMemberRequest represents a request to add a member
type AddMemberRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
	Mode      string `json:"mode"` // "direct" or "invite"
}

// AddMember adds a new member to the family
func (h *FamilyHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())
	userRole := middleware.GetRole(r.Context())

	// Only parents can add members
	if userRole != models.FamilyRoleParent {
		respondForbidden(w, "Only parents can add family members")
		return
	}

	var req AddMemberRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Email == "" {
		respondBadRequest(w, "Email is required")
		return
	}

	if req.Role == "" {
		respondBadRequest(w, "Role is required")
		return
	}

	// Validate role
	role := models.FamilyRole(req.Role)
	if role != models.FamilyRoleParent && role != models.FamilyRoleCaregiver && role != models.FamilyRoleMedicalProvider {
		respondBadRequest(w, "Invalid role. Must be parent, caregiver, or medical_provider")
		return
	}

	// Look up user by email
	user, err := h.userService.GetByEmail(r.Context(), req.Email)
	if err != nil {
		respondInternalError(w, "Failed to look up user")
		return
	}

	if user == nil {
		// User not found - handle invitation mode
		if req.Mode == "invite" {
			// Create pending invitation (store in database for when they register)
			err := h.familyService.CreateInvitation(r.Context(), familyID, req.Email, req.FirstName, req.LastName, role)
			if err != nil {
				respondInternalError(w, "Failed to create invitation")
				return
			}

			// Send invitation email
			claims := middleware.GetAuthClaims(r.Context())
			inviterName := claims.FirstName
			family, famErr := h.familyService.GetByID(r.Context(), familyID)
			if famErr != nil {
				log.Printf("[FAMILY] Failed to get family %s for invitation email: %v", familyID, famErr)
			}
			familyName := "your family"
			if family != nil {
				familyName = family.Name
			}
			inviteeName := req.FirstName
			if inviteeName == "" {
				inviteeName = "there"
			}
			go func() {
				if err := h.emailService.SendFamilyInvitationEmail(req.Email, inviteeName, familyName, inviterName, h.appURL); err != nil {
					log.Printf("[EMAIL] Failed to send invitation email to %s: %v", req.Email, err)
				}
			}()

			respondCreated(w, map[string]interface{}{
				"success": true,
				"message": "Invitation created and email sent. They will be added when they register.",
			})
			return
		}
		respondNotFound(w, "User not found. They must register first before being added to a family.")
		return
	}

	// Check if already a member
	existingMembership, err := h.familyService.GetMembership(r.Context(), familyID, user.ID)
	if err != nil {
		respondInternalError(w, "Failed to check membership")
		return
	}
	if existingMembership != nil {
		respondBadRequest(w, "User is already a member of this family")
		return
	}

	// Add member
	if err := h.familyService.AddMember(r.Context(), familyID, user.ID, role); err != nil {
		respondInternalError(w, "Failed to add member")
		return
	}

	// Send notification email to the added member
	family, famErr := h.familyService.GetByID(r.Context(), familyID)
	if famErr != nil {
		log.Printf("[FAMILY] Failed to get family %s for notification email: %v", familyID, famErr)
	}
	familyName := "a family"
	if family != nil {
		familyName = family.Name
	}
	go func() {
		if err := h.emailService.SendFamilyMemberAddedEmail(user.Email, user.FirstName, familyName, string(role), h.appURL); err != nil {
			log.Printf("[EMAIL] Failed to send member-added email to %s: %v", user.Email, err)
		}
	}()

	// Send push notification to the added user
	if h.pushService != nil {
		go func() {
			msg := service.PushMessage{
				Title:    "You've been added to a family",
				Body:     "You have been added to " + familyName + " as a " + string(role),
				Priority: service.PushPriorityNormal,
				Data: map[string]string{
					"type":      "family_added",
					"family_id": familyID.String(),
				},
			}
			h.pushService.Send(context.Background(), user.ID, msg)
		}()
	}

	respondCreated(w, map[string]interface{}{
		"success": true,
		"message": "Member added successfully",
	})
}

// LookupUserRequest represents a request to look up a user
type LookupUserRequest struct {
	Email string `json:"email"`
}

// LookupUserResponse represents the response for user lookup
type LookupUserResponse struct {
	Found bool         `json:"found"`
	User  *models.User `json:"user,omitempty"`
}

// LookupUser looks up a user by email for the add workflow
func (h *FamilyHandler) LookupUser(w http.ResponseWriter, r *http.Request) {
	var req LookupUserRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Email == "" {
		respondBadRequest(w, "Email is required")
		return
	}

	user, err := h.userService.GetByEmail(r.Context(), req.Email)
	if err != nil {
		respondInternalError(w, "Failed to look up user")
		return
	}

	if user == nil {
		respondOK(w, LookupUserResponse{Found: false})
		return
	}

	// Return limited user info (don't expose sensitive data)
	respondOK(w, LookupUserResponse{
		Found: true,
		User: &models.User{
			ID:        user.ID,
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
		},
	})
}

// UpdateRoleRequest represents a request to update a member's role
type UpdateRoleRequest struct {
	Role string `json:"role"`
}

// UpdateMemberRole updates a member's role
func (h *FamilyHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())
	userRole := middleware.GetRole(r.Context())

	// Only parents can update roles
	if userRole != models.FamilyRoleParent {
		respondForbidden(w, "Only parents can update member roles")
		return
	}

	memberID, err := parseUUID(chi.URLParam(r, "memberID"))
	if err != nil {
		respondBadRequest(w, "Invalid member ID")
		return
	}

	var req UpdateRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	// Validate role
	role := models.FamilyRole(req.Role)
	if role != models.FamilyRoleParent && role != models.FamilyRoleCaregiver && role != models.FamilyRoleMedicalProvider {
		respondBadRequest(w, "Invalid role. Must be parent, caregiver, or medical_provider")
		return
	}

	// Use safe update that checks for creator
	if err := h.familyService.UpdateMemberRoleSafe(r.Context(), familyID, memberID, role); err != nil {
		switch err {
		case service.ErrMemberNotFound:
			respondNotFound(w, "Member not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Member does not belong to this family")
		case service.ErrCannotChangeCreator:
			respondForbidden(w, "Cannot change the family creator's role")
		default:
			respondInternalError(w, "Failed to update member role")
		}
		return
	}

	respondOK(w, map[string]interface{}{
		"success": true,
		"message": "Member role updated successfully",
	})
}

// RemoveMember removes a member from the family
func (h *FamilyHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())
	userRole := middleware.GetRole(r.Context())

	// Only parents can remove members
	if userRole != models.FamilyRoleParent {
		respondForbidden(w, "Only parents can remove family members")
		return
	}

	memberID, err := parseUUID(chi.URLParam(r, "memberID"))
	if err != nil {
		respondBadRequest(w, "Invalid member ID")
		return
	}

	// Use safe remove that checks for creator
	if err := h.familyService.RemoveMemberSafe(r.Context(), familyID, memberID); err != nil {
		switch err {
		case service.ErrMemberNotFound:
			respondNotFound(w, "Member not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Member does not belong to this family")
		case service.ErrCannotRemoveCreator:
			respondForbidden(w, "Cannot remove the family creator")
		default:
			respondInternalError(w, "Failed to remove member")
		}
		return
	}

	respondNoContent(w)
}

// GetMember returns a specific member
func (h *FamilyHandler) GetMember(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())

	memberID, err := parseUUID(chi.URLParam(r, "memberID"))
	if err != nil {
		respondBadRequest(w, "Invalid member ID")
		return
	}

	member, err := h.familyService.GetMemberByID(r.Context(), memberID)
	if err != nil {
		respondInternalError(w, "Failed to get member")
		return
	}
	if member == nil {
		respondNotFound(w, "Member not found")
		return
	}

	// Verify member belongs to this family
	if member.FamilyID != familyID {
		respondForbidden(w, "Member does not belong to this family")
		return
	}

	respondOK(w, member)
}

// GetUserPreferences returns user display preferences
// GET /api/users/me/preferences
func (h *FamilyHandler) GetUserPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	prefs, err := h.userService.GetPreferences(r.Context(), userID)
	if err != nil {
		respondInternalError(w, "Failed to get preferences")
		return
	}

	respondOK(w, prefs)
}

// UpdateUserPreferences updates user display preferences
// PUT /api/users/me/preferences
func (h *FamilyHandler) UpdateUserPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req models.UpdatePreferencesRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	err := h.userService.UpdatePreferences(r.Context(), userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to update preferences")
		return
	}

	// Return updated preferences
	prefs, prefsErr := h.userService.GetPreferences(r.Context(), userID)
	if prefsErr != nil {
		log.Printf("[FAMILY] Failed to reload preferences after update for user %s: %v", userID, prefsErr)
	}
	respondOK(w, prefs)
}
