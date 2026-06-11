package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"opensync/pkg/util"
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
	if !util.ToBool(float64(1)) {
		t.Fatalf("util.ToBool(float64(1)) = false, want true")
	}
	if util.ToBool(float64(0)) {
		t.Fatalf("util.ToBool(float64(0)) = true, want false")
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

func TestSendWebhookCustomBodyEscapesPlaceholderValues(t *testing.T) {
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is invalid JSON: %v\n%s", err, string(body))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("sendWebhook() panic = %v, want escaped JSON body", recovered)
		}
	}()

	sendWebhook(server.Client(), map[string]interface{}{
		"url":  server.URL,
		"body": `{"text":"{title}: {content}"}`,
	}, `title "quoted"`, `content with "quotes"`)

	want := `title "quoted": content with "quotes"`
	if got["text"] != want {
		t.Fatalf("text = %q, want %q", got["text"], want)
	}
}
