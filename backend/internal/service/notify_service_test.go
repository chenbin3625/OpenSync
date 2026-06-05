package service

import (
	"net/http"
	"strings"
	"testing"
)

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

func TestBuildNotifyRequestRejectsMissingURL(t *testing.T) {
	_, err := buildNotifyRequest(http.MethodPost, "", nil, "application/json")
	if err == nil {
		t.Fatalf("buildNotifyRequest() error = nil, want missing URL error")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Fatalf("error = %q, want URL context", err)
	}
}

func TestBuildNotifyRequestRejectsInvalidMethod(t *testing.T) {
	_, err := buildNotifyRequest("BAD METHOD", "https://example.test/hook", nil, "application/json")
	if err == nil {
		t.Fatalf("buildNotifyRequest() error = nil, want invalid method error")
	}
}

func TestParseNotifyParamsRejectsInvalidJSON(t *testing.T) {
	_, err := parseNotifyParams("{invalid-json")
	if err == nil {
		t.Fatalf("parseNotifyParams() error = nil, want invalid JSON error")
	}
}
