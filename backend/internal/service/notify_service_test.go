package service

import "testing"

func TestParamStringSupportsLegacyAndRefactorKeys(t *testing.T) {
	params := map[string]interface{}{
		"webhook": "https://example.test/hook",
	}

	if got := paramString(params, "url", "webhook"); got != "https://example.test/hook" {
		t.Fatalf("paramString() = %q, want legacy webhook value", got)
	}
}

func TestToBoolSupportsStoredIntegerFlags(t *testing.T) {
	if !toBool(float64(1)) {
		t.Fatalf("toBool(float64(1)) = false, want true")
	}
	if toBool(float64(0)) {
		t.Fatalf("toBool(float64(0)) = true, want false")
	}
}
