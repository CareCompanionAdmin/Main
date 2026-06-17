package service

import "carecompanion/internal/models"

// OnboardingComplete reports whether the required onboarding wizard is finished.
// A nil state (no row / brand-new user) counts as incomplete.
func OnboardingComplete(s *models.OnboardingState) bool {
	return s != nil && s.CompletedAt != nil
}

// ShouldShowChecklist reports whether the dashboard "finish setting up" card
// should render: only after the wizard is complete, and only while the user
// has neither dismissed it nor finished both the invite and settings items.
func ShouldShowChecklist(s *models.OnboardingState) bool {
	if !OnboardingComplete(s) {
		return false
	}
	if s.ChecklistDismissedAt != nil {
		return false
	}
	if s.SettingsDoneAt != nil && s.InviteDoneAt != nil {
		return false
	}
	return true
}
