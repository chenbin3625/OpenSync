package handler

import "testing"

func TestParseRequiredIDRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{"", "abc", "0", "-1"} {
		if _, err := parseRequiredID(input, "id"); err == nil {
			t.Fatalf("parseRequiredID(%q) returned nil error, want error", input)
		}
	}
}

func TestParseRequiredIDAcceptsPositiveInteger(t *testing.T) {
	id, err := parseRequiredID("42", "id")
	if err != nil {
		t.Fatalf("parseRequiredID() error: %v", err)
	}
	if id != 42 {
		t.Fatalf("parseRequiredID() = %d, want 42", id)
	}
}
