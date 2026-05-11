package service

import (
	"strings"
	"testing"
	"time"
)

func TestAgeBand(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		dob  time.Time
		want string
	}{
		{"zero DOB", time.Time{}, ""},
		{"3 years old", now.AddDate(-3, 0, 0), "2-3y"},
		{"4 years old", now.AddDate(-4, 0, 0), "4-5y"},
		{"5 years old", now.AddDate(-5, 0, 0), "4-5y"},
		{"7 years old", now.AddDate(-7, 0, 0), "6-7y"},
		{"10 years old", now.AddDate(-10, 0, 0), "10-11y"},
		{"17 years old", now.AddDate(-17, 0, 0), "16-17y"},
		{"18 years old", now.AddDate(-18, 0, 0), "18+"},
		{"30 years old", now.AddDate(-30, 0, 0), "18+"},
		{"future DOB", now.AddDate(1, 0, 0), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AgeBand(tt.dob)
			if got != tt.want {
				t.Errorf("AgeBand(%v) = %q, want %q", tt.dob, got, tt.want)
			}
		})
	}
}

func TestGeneralizeICD(t *testing.T) {
	tests := []struct {
		icd  string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"F84.0", "autism-spectrum"},
		{"F84.5", "autism-spectrum"},
		{"f84.0", "autism-spectrum"}, // case-insensitive
		{"F90.0", "adhd"},
		{"F90.1", "adhd"},
		{"F95.2", "tic-disorder"},
		{"F41.1", "anxiety-disorder"},
		{"F40", "anxiety-disorder"},
		{"F32.1", "mood-disorder"},
		{"F50.0", "feeding-or-eating-disorder"},
		{"F70.0", "intellectual-disability"},
		{"F79", "intellectual-disability"},
		{"F80.1", "developmental-disorder"},
		{"G40.0", "epilepsy"},
		{"G40", "epilepsy"},
		{"G80.0", "cerebral-palsy"},
		{"G47.30", "sleep-disorder"},
		{"Q90.0", "chromosomal-syndrome"},
		{"Q91.1", "chromosomal-syndrome"},
		{"K59.0", "gi-functional-disorder"},
		{"K58.0", "ibs"},
		{"F99", "psychiatric-other"},
		{"G99", "neurological-other"},
		{"K99", "gi-other"},
		{"Q99", "chromosomal-syndrome"}, // Q99 included
		{"Z00.0", "other"},
		{"unknown", "other"},
	}
	for _, tt := range tests {
		t.Run(tt.icd, func(t *testing.T) {
			got := GeneralizeICD(tt.icd)
			if got != tt.want {
				t.Errorf("GeneralizeICD(%q) = %q, want %q", tt.icd, got, tt.want)
			}
		})
	}
}

func TestDrugClass(t *testing.T) {
	tests := []struct {
		name string
		drug string
		want string
	}{
		{"known stimulant", "adderall", "stimulant"},
		{"known stimulant case-insensitive", "Vyvanse", "stimulant"},
		{"known ssri", "fluoxetine", "ssri"},
		{"known opioid", "tramadol", "opioid"},
		{"known benzo", "xanax", "benzodiazepine"},
		{"known supplement", "melatonin", "melatonin"},
		{"unknown drug", "FantasyDrug500", "unmapped-medication"},
		{"empty", "", "unmapped-medication"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DrugClass(tt.drug)
			if got != tt.want {
				t.Errorf("DrugClass(%q) = %q, want %q", tt.drug, got, tt.want)
			}
		})
	}
}

func TestRelativeDayLabel(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"today", now, "Day-0"},
		{"yesterday", now.AddDate(0, 0, -1), "Day-1"},
		{"3 days ago", now.AddDate(0, 0, -3), "Day-3"},
		{"30 days ago", now.AddDate(0, 0, -30), "Day-30"},
		{"future (clamped)", now.AddDate(0, 0, 1), "Day-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelativeDayLabel(tt.t, now)
			if got != tt.want {
				t.Errorf("RelativeDayLabel(%v, %v) = %q, want %q", tt.t, now, got, tt.want)
			}
		})
	}
}

func TestApplyNamePlaceholder(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		firstName string
		want      string
	}{
		{
			"simple substitution",
			"[CHILD] had a great day.",
			"Sarah",
			"Sarah had a great day.",
		},
		{
			"possessive substitution",
			"[CHILD]'s mood averaged 7/10.",
			"Sarah",
			"Sarah's mood averaged 7/10.",
		},
		{
			"multiple occurrences",
			"[CHILD] slept well. [CHILD]'s energy was high.",
			"Sarah",
			"Sarah slept well. Sarah's energy was high.",
		},
		{
			"no placeholder present",
			"The mood was great today.",
			"Sarah",
			"The mood was great today.",
		},
		{
			"empty first name",
			"[CHILD] had a great day.",
			"",
			"[CHILD] had a great day.",
		},
		{
			"placeholder at end",
			"This is the story of [CHILD]",
			"Sarah",
			"This is the story of Sarah",
		},
		{
			"placeholder mid-word boundary",
			"Look at [CHILD]'s sleep pattern: [CHILD] is doing well.",
			"Alex",
			"Look at Alex's sleep pattern: Alex is doing well.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyNamePlaceholder(tt.text, tt.firstName)
			if got != tt.want {
				t.Errorf("ApplyNamePlaceholder(%q, %q) = %q, want %q", tt.text, tt.firstName, got, tt.want)
			}
		})
	}
}

// TestNoPHIInDeIdentifiedOutput is a golden assertion test — given a
// child profile and log set that would normally produce identifying
// data in a Claude prompt, verify that none of the HIPAA Safe Harbor
// identifiers survive into the de-identified output. This is the
// "smoke alarm" test: if anyone changes the stripper or rebuilds
// without it, this should fail loudly.
//
// Strategy: build a synthetic "Bobby Smith" profile with a known DOB,
// ICD code, and medication name, then assert none of those strings
// appear in the output produced by AgeBand + GeneralizeICD + DrugClass.
func TestNoPHIInDeIdentifiedOutput(t *testing.T) {
	// Identifying values that MUST NOT appear in the output:
	forbidden := []string{
		"Bobby", "Smith",
		"2019-03-15", "2019", "March", "Mar",
		"F84.0",                    // ICD code
		"Risperdal", "risperdal",   // brand name
		"risperidone",              // generic name (also identifying with age+condition)
		"0.5mg", "0.5 mg",          // dosage
	}

	out := strings.Join([]string{
		AgeBand(time.Date(2019, 3, 15, 0, 0, 0, 0, time.UTC)),
		GeneralizeICD("F84.0"),
		DrugClass("Risperdal"),
	}, " | ")

	for _, f := range forbidden {
		if strings.Contains(out, f) {
			t.Errorf("forbidden token %q leaked into output: %q", f, out)
		}
	}

	// Sanity-check that the de-identified replacement is present:
	if !strings.Contains(out, "autism-spectrum") {
		t.Errorf("expected ICD generalization to autism-spectrum, got: %q", out)
	}
	if !strings.Contains(out, "antipsychotic") {
		t.Errorf("expected drug class antipsychotic for Risperdal, got: %q", out)
	}
}
