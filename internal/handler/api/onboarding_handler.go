package api

import (
	"net/http"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// OnboardingHandler handles per-user onboarding state transitions.
type OnboardingHandler struct {
	userService *service.UserService
}

// NewOnboardingHandler creates a new onboarding handler.
func NewOnboardingHandler(userService *service.UserService) *OnboardingHandler {
	return &OnboardingHandler{userService: userService}
}

// Complete handles POST /api/onboarding/complete — marks the wizard finished.
func (h *OnboardingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.CompleteOnboarding(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to complete onboarding")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Onboarding completed"})
}

// DismissChecklist handles POST /api/onboarding/checklist/dismiss.
func (h *OnboardingHandler) DismissChecklist(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.DismissChecklist(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to dismiss checklist")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Checklist dismissed"})
}

// SettingsDone handles POST /api/onboarding/settings-done.
func (h *OnboardingHandler) SettingsDone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.MarkSettingsDone(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to mark settings done")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Settings step done"})
}

// InviteDone handles POST /api/onboarding/invite-done.
func (h *OnboardingHandler) InviteDone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.userService.MarkInviteDone(r.Context(), userID); err != nil {
		respondInternalError(w, "Failed to mark invite done")
		return
	}
	respondOK(w, SuccessResponse{Success: true, Message: "Invite step done"})
}
