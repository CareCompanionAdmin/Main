package service

import "testing"

func TestExtractJSONArray_Plain(t *testing.T) {
	in := `[{"a":1},{"b":2}]`
	out := extractJSONArray(in)
	if out != in {
		t.Errorf("expected exact passthrough, got %q", out)
	}
}

func TestExtractJSONArray_LeadingFence(t *testing.T) {
	in := "```json\n[{\"a\":1}]\n```"
	out := extractJSONArray(in)
	want := "[{\"a\":1}]"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestExtractJSONArray_TrailingFenceOnly(t *testing.T) {
	in := "[{\"a\":1}]\n```\n"
	out := extractJSONArray(in)
	want := "[{\"a\":1}]"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestExtractJSONArray_BracketInString(t *testing.T) {
	// The literal "[CHILD]" placeholder inside a JSON string must NOT
	// confuse the extractor — it should still find the OUTER array close.
	in := `[{"title":"[CHILD]'s mood","desc":"see ranges [3-5, 6-9]"}]`
	out := extractJSONArray(in)
	if out != in {
		t.Errorf("BracketInString: got %q\nwant %q", out, in)
	}
}

func TestExtractJSONArray_ChildPlaceholderProseFails(t *testing.T) {
	// When Claude returns prose starting with "[CHILD]'s ..." (no array),
	// the extractor should return "" rather than misinterpret the
	// "[CHILD]" as a JSON array opener.
	in := `[CHILD]'s data is incomplete. I cannot generate insights without log entries.`
	out := extractJSONArray(in)
	if out != "" {
		t.Errorf("expected empty for prose response, got %q", out)
	}
}

func TestExtractJSONArray_ChildPlaceholderThenRealArray(t *testing.T) {
	// "[CHILD]" prose followed by a real JSON array — should find the array.
	in := "Here are insights for [CHILD]:\n```json\n[{\"a\":1}]\n```"
	out := extractJSONArray(in)
	want := `[{"a":1}]`
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestExtractJSONArray_TruncatedReturnsEmpty(t *testing.T) {
	in := `[{"a":1`
	out := extractJSONArray(in)
	if out != "" {
		t.Errorf("expected empty for truncated array, got %q", out)
	}
}

func TestExtractJSONArray_NestedArrayInsideString(t *testing.T) {
	// Nested array notation inside a string ("ages: [3-5, 6-9]") must
	// not confuse the depth counter.
	in := `[{"desc":"age ranges: [3-5, 6-9, 10-12]","tier":1}]`
	out := extractJSONArray(in)
	if out != in {
		t.Errorf("NestedInString: got %q\nwant %q", out, in)
	}
}

func TestExtractJSONArray_EscapedQuoteInString(t *testing.T) {
	// An escaped quote shouldn't end string-state mid-walk.
	in := `[{"desc":"she said \"hi\""}]`
	out := extractJSONArray(in)
	if out != in {
		t.Errorf("EscapedQuote: got %q\nwant %q", out, in)
	}
}

func TestExtractJSONArray_EmptyInput(t *testing.T) {
	if out := extractJSONArray(""); out != "" {
		t.Errorf("expected empty on empty input, got %q", out)
	}
}

func TestTruncateLog(t *testing.T) {
	if got := truncateLog("hello", 10); got != "hello" {
		t.Errorf("truncate to longer = %q", got)
	}
	if got := truncateLog("hello world", 5); got != "hello..." {
		t.Errorf("truncate to shorter = %q", got)
	}
	if got := truncateLog("", 5); got != "" {
		t.Errorf("truncate empty = %q", got)
	}
}
