package service

import (
	"testing"

	"carecompanion/internal/models"
)

// computeDateRange must accept the canonical values AND legacy/alias period
// names that older app builds send, instead of 500ing (#112392).
func TestComputeDateRange_AcceptsAliases(t *testing.T) {
	cases := map[string]string{
		"day":          "day",
		"week":         "week",
		"month":        "month",
		"last_30_days": "month",
		"last_7_days":  "week",
		"weekly":       "week",
		"monthly":      "month",
		"today":        "day",
		"daily":        "day",
	}
	for input, wantNormalized := range cases {
		req := &models.GenerateReportRequest{PeriodType: input}
		start, end, err := computeDateRange(req)
		if err != nil {
			t.Errorf("period %q: unexpected error %v", input, err)
			continue
		}
		if end.Before(start) {
			t.Errorf("period %q: end %v before start %v", input, end, start)
		}
		if req.PeriodType != wantNormalized {
			t.Errorf("period %q: normalized to %q, want %q", input, req.PeriodType, wantNormalized)
		}
	}

	// A genuinely unknown value still errors (so we notice new bad clients).
	if _, _, err := computeDateRange(&models.GenerateReportRequest{PeriodType: "fortnight"}); err == nil {
		t.Errorf("expected error for unknown period_type")
	}
}
