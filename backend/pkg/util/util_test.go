package util

import "testing"

func TestConvertBytesUsesBytesForValuesUnderOneKB(t *testing.T) {
	if got := ConvertBytes(512); got != "512 B" {
		t.Fatalf("ConvertBytes(512) = %q, want 512 B", got)
	}
}

func TestConvertBytesUsesLargerUnits(t *testing.T) {
	if got := ConvertBytes(1536); got != "1.50 KB" {
		t.Fatalf("ConvertBytes(1536) = %q, want 1.50 KB", got)
	}
}

func TestToIntTrimsWhitespaceStrings(t *testing.T) {
	if got := ToInt(" 42 "); got != 42 {
		t.Fatalf("ToInt(%q) = %d, want 42", " 42 ", got)
	}
	if got := ToInt(" \t "); got != 0 {
		t.Fatalf("ToInt(%q) = %d, want 0", " \t ", got)
	}
}

func TestToIntRejectsPartiallyNumericStrings(t *testing.T) {
	if got := ToInt("12abc"); got != 0 {
		t.Fatalf("ToInt(%q) = %d, want 0", "12abc", got)
	}
	if got := ToInt64("12abc"); got != 0 {
		t.Fatalf("ToInt64(%q) = %d, want 0", "12abc", got)
	}
}
