package service

import (
	"errors"
	"testing"
)

func TestValidateTicketFields(t *testing.T) {
	cases := []struct {
		name    string
		staff   bool
		typ     string
		prio    string
		status  string
		wantErr bool
	}{
		{"empty all ok", false, "", "", "", false},
		{"user valid type+prio", false, "bug_report", "high", "", false},
		{"user urgent rejected", false, "", "urgent", "", true},
		{"user bad type rejected", false, "nonsense", "", "", true},
		{"user bad priority rejected", false, "", "screaming", "", true},
		{"user status rejected", false, "", "", "open", true},
		{"staff urgent allowed", true, "", "urgent", "", false},
		{"staff status allowed", true, "", "", "waiting_on_user", false},
		{"staff bad status rejected", true, "", "", "nope", true},
		{"staff full combo", true, "feature_request", "urgent", "closed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateTicketFields(c.staff, c.typ, c.prio, c.status)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if c.wantErr && !errors.Is(err, ErrInvalidTicketField) {
				t.Fatalf("error should wrap ErrInvalidTicketField, got %v", err)
			}
		})
	}
}
