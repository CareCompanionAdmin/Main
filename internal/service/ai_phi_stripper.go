package service

// ai_phi_stripper.go — single point of truth for de-identifying child data
// before it leaves the server for an outbound LLM call (Anthropic / Bedrock).
//
// Threat model: Anthropic and any other third-party LLM provider could
// theoretically build context across calls and re-identify individual
// children in our small user base. To defeat this we:
//
//   1. Strip the 18 HIPAA Safe Harbor identifiers (names, dates, IDs, etc.)
//   2. Replace specific drug names with drug classes via getDrugClass()
//   3. Generalize ICD-10 codes to coarse disease categories
//   4. Drop free-text fields entirely (default path)
//   5. Use relative date labels ("Day-3") instead of calendar dates
//   6. Refer to the child only as "[CHILD]" — the real name is substituted
//      client-side after Claude returns insights, so the LLM never sees it.
//
// Internal services (insight_generator.go, drug_database.go, etc.) continue
// to operate on identified PHI as before — only the outbound HTTP boundary
// in ai_insight_service.go sees de-identified data.
//
// See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md
// for the full design.

import (
	"fmt"
	"strings"
	"time"
)

// NamePlaceholder is the exact token Claude is instructed to use in place
// of the real child name. We substitute this back to the actual first
// name in storeInsights, after the LLM has returned. The token is chosen
// to be unlikely to collide with natural English.
const NamePlaceholder = "[CHILD]"

// AgeBand converts a precise date of birth into a two-year age band
// like "4-5y" or "8-9y". 18 and over are collapsed to "18+". An invalid
// or zero DOB returns "". The two-year bucket width balances clinical
// relevance (matters for developmental expectations) against re-identification
// risk in a small population.
func AgeBand(dob time.Time) string {
	if dob.IsZero() {
		return ""
	}
	since := time.Since(dob)
	if since < 0 {
		return ""
	}
	years := int(since.Hours() / 8760)
	if years >= 18 {
		return "18+"
	}
	start := (years / 2) * 2
	end := start + 1
	return fmt.Sprintf("%d-%dy", start, end)
}

// GeneralizeICD maps an ICD-10 code to a coarse disease category. The
// goal is to keep enough clinical signal that the LLM can reason about
// medication relevance and developmental expectations, while removing
// the precision that aids re-identification of rare conditions.
//
// Examples:
//
//	F84.0 -> "autism-spectrum"
//	F90.0 -> "adhd"
//	G40.x -> "epilepsy"
//	Q90.x -> "chromosomal-syndrome"
//
// Anything unmatched returns "other" (or "" for empty input).
func GeneralizeICD(icd string) string {
	icd = strings.ToUpper(strings.TrimSpace(icd))
	if icd == "" {
		return ""
	}
	head := icd
	if i := strings.IndexByte(icd, '.'); i > 0 {
		head = icd[:i]
	}
	switch {
	case strings.HasPrefix(head, "F84"):
		return "autism-spectrum"
	case strings.HasPrefix(head, "F90"):
		return "adhd"
	case strings.HasPrefix(head, "F95"):
		return "tic-disorder"
	case strings.HasPrefix(head, "F40"), strings.HasPrefix(head, "F41"):
		return "anxiety-disorder"
	case strings.HasPrefix(head, "F32"), strings.HasPrefix(head, "F33"):
		return "mood-disorder"
	case strings.HasPrefix(head, "F50"):
		return "feeding-or-eating-disorder"
	case strings.HasPrefix(head, "F70"), strings.HasPrefix(head, "F71"),
		strings.HasPrefix(head, "F72"), strings.HasPrefix(head, "F73"),
		strings.HasPrefix(head, "F78"), strings.HasPrefix(head, "F79"):
		return "intellectual-disability"
	case strings.HasPrefix(head, "F80"), strings.HasPrefix(head, "F81"),
		strings.HasPrefix(head, "F82"):
		return "developmental-disorder"
	case strings.HasPrefix(head, "G40"):
		return "epilepsy"
	case strings.HasPrefix(head, "G80"):
		return "cerebral-palsy"
	case strings.HasPrefix(head, "G47"):
		return "sleep-disorder"
	case strings.HasPrefix(head, "Q90"), strings.HasPrefix(head, "Q91"),
		strings.HasPrefix(head, "Q92"), strings.HasPrefix(head, "Q93"),
		strings.HasPrefix(head, "Q99"):
		return "chromosomal-syndrome"
	case strings.HasPrefix(head, "K59"):
		return "gi-functional-disorder"
	case strings.HasPrefix(head, "K58"):
		return "ibs"
	case strings.HasPrefix(head, "F"):
		return "psychiatric-other"
	case strings.HasPrefix(head, "G"):
		return "neurological-other"
	case strings.HasPrefix(head, "K"):
		return "gi-other"
	case strings.HasPrefix(head, "Q"):
		return "congenital-other"
	}
	return "other"
}

// DrugClass maps a medication name to its drug class via the existing
// drug_database helper. Unknown drugs return "unmapped-medication" so
// the LLM sees that the child takes a medication, just without the
// specific identity.
func DrugClass(name string) string {
	if class := getDrugClass(name); class != "" {
		return class
	}
	return "unmapped-medication"
}

// RelativeDayLabel returns "Day-N" where N is whole days from `now` back
// to `t`. Negative results are clamped to 0 (the log is from today or
// later than now, which shouldn't happen in practice but defends against
// clock skew).
func RelativeDayLabel(t time.Time, now time.Time) string {
	days := int(now.Sub(t).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return fmt.Sprintf("Day-%d", days)
}

// ApplyNamePlaceholder substitutes the placeholder back to the real first
// name in LLM-generated text. Handles common variants Claude might emit:
//
//	[CHILD]   -> firstName
//	[CHILD]'s -> firstName + "'s"
//
// The substitution is done in possessive-first order so that "[CHILD]'s"
// matches before the bare "[CHILD]".
func ApplyNamePlaceholder(text, firstName string) string {
	if firstName == "" {
		return text
	}
	text = strings.ReplaceAll(text, NamePlaceholder+"'s", firstName+"'s")
	text = strings.ReplaceAll(text, NamePlaceholder, firstName)
	return text
}
