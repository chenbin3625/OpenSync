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
