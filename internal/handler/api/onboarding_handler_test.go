package api

import "testing"

func TestNewOnboardingHandler_Constructs(t *testing.T) {
	h := NewOnboardingHandler(nil)
	if h == nil {
		t.Fatal("NewOnboardingHandler returned nil")
	}
}
