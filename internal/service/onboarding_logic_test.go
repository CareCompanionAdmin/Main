package service

import (
	"testing"
	"time"

	"carecompanion/internal/models"
)

func TestOnboardingComplete(t *testing.T) {
	now := time.Now()
	if OnboardingComplete(nil) {
		t.Fatal("nil state should be incomplete")
	}
	if OnboardingComplete(&models.OnboardingState{}) {
		t.Fatal("empty state should be incomplete")
	}
	if !OnboardingComplete(&models.OnboardingState{CompletedAt: &now}) {
		t.Fatal("state with CompletedAt should be complete")
	}
}

func TestShouldShowChecklist(t *testing.T) {
	now := time.Now()
	// not completed -> never show checklist (user is still in the wizard)
	if ShouldShowChecklist(&models.OnboardingState{}) {
		t.Fatal("incomplete onboarding should not show checklist")
	}
	// completed, nothing else -> show
	if !ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now}) {
		t.Fatal("completed with pending items should show checklist")
	}
	// dismissed -> hide
	if ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now, ChecklistDismissedAt: &now}) {
		t.Fatal("dismissed checklist should not show")
	}
	// both invite + settings done -> hide (auto-complete)
	if ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now, SettingsDoneAt: &now, InviteDoneAt: &now}) {
		t.Fatal("all items done should not show checklist")
	}
	// only one done -> still show
	if !ShouldShowChecklist(&models.OnboardingState{CompletedAt: &now, SettingsDoneAt: &now}) {
		t.Fatal("partial completion should still show checklist")
	}
}
