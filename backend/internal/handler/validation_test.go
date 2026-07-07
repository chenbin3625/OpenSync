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

func TestParseEnableValueRejectsMissingOrInvalidValues(t *testing.T) {
	for _, input := range []interface{}{nil, "", "abc", 2, -1, 1.5} {
		if _, err := parseEnableValue(input); err == nil {
			t.Fatalf("parseEnableValue(%#v) returned nil error, want error", input)
		}
	}
}

func TestParseEnableValueAcceptsExplicitBooleanOrBinaryValues(t *testing.T) {
	tests := []struct {
		input interface{}
		want  int
	}{
		{true, 1},
		{false, 0},
		{1, 1},
		{0, 0},
		{"1", 1},
		{"0", 0},
		{"true", 1},
		{"false", 0},
	}

	for _, tt := range tests {
		got, err := parseEnableValue(tt.input)
		if err != nil {
			t.Fatalf("parseEnableValue(%#v) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("parseEnableValue(%#v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
