package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"opensync/internal/i18n"
	"opensync/internal/model"
	"opensync/pkg/util"
	"strings"
	"testing"
)

type notifyErrorTransport struct{}

func (notifyErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, &url.Error{
		Op:  "Post",
		URL: req.URL.String(),
		Err: errors.New("dial tcp timeout"),
	}
}

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

func TestMaskNotifyURLRedactsUserInfo(t *testing.T) {
	got := maskNotifyURL("https://bot:secret@example.test/hook")
	if strings.Contains(got, "bot") || strings.Contains(got, "secret") {
		t.Fatalf("maskNotifyURL leaked userinfo: %s", got)
	}
}

func TestSendNotifyRequestDoesNotPanicWithSecretURL(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.test/hook?access_token=very-secret-token", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	client := &http.Client{Transport: notifyErrorTransport{}}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("sendNotifyRequest() panic = nil, want public notify failure")
		}
		publicErr, ok := recovered.(model.PublicError)
		if !ok {
			t.Fatalf("panic type = %T, want model.PublicError", recovered)
		}
		if string(publicErr) != i18n.G("notify_send_fail") {
			t.Fatalf("panic = %q, want generic notify failure", publicErr)
		}
		if strings.Contains(string(publicErr), "very-secret-token") || strings.Contains(string(publicErr), "access_token") {
			t.Fatalf("panic leaked secret URL: %q", publicErr)
		}
	}()

	sendNotifyRequest(client, req)
}
