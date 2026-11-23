package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
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

func TestValidateNotifyParamsRejectsIncompleteConfigs(t *testing.T) {
	cases := []struct {
		name   string
		method int
		params map[string]interface{}
	}{
		{
			name:   "unknown method",
			method: 99,
			params: map[string]interface{}{},
		},
		{
			name:   "serverchan missing sendKey",
			method: 1,
			params: map[string]interface{}{},
		},
		{
			name:   "wecom missing secret",
			method: 3,
			params: map[string]interface{}{"corpid": "corp", "agentid": "agent"},
		},
		{
			name:   "webhook unsupported method",
			method: 0,
			params: map[string]interface{}{"url": "https://example.test/hook", "method": "PATCH"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateNotifyParams(tc.method, tc.params); err == nil {
				t.Fatalf("validateNotifyParams() error = nil, want error")
			}
		})
	}
}

func TestValidateNotifyParamsAcceptsCompleteConfigs(t *testing.T) {
	cases := []struct {
		name   string
		method int
		params map[string]interface{}
	}{
		{
			name:   "custom webhook",
			method: 0,
			params: map[string]interface{}{"url": "https://example.test/hook", "method": "PUT"},
		},
		{
			name:   "serverchan",
			method: 1,
			params: map[string]interface{}{"sendKey": "send-key"},
		},
		{
			name:   "wecom",
			method: 3,
			params: map[string]interface{}{"corpid": "corp", "corpsecret": "secret", "agentid": "agent"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateNotifyParams(tc.method, tc.params); err != nil {
				t.Fatalf("validateNotifyParams() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateNotifyParamsAcceptsInternalHTTPSWebhook(t *testing.T) {
	// A reverse proxy can expose an HTTPS hostname that resolves to a LAN
	// address. Such targets are valid for self-hosted services and must not be
	// rejected solely because of the resolved address.
	if err := validateNotifyParams(0, map[string]interface{}{
		"url":    "https://ntfy.lan.example:8443/publish",
		"method": "POST",
	}); err != nil {
		t.Fatalf("validateNotifyParams() error = %v, want nil for internal HTTPS webhook", err)
	}
}

func TestValidateNotifyParamsAcceptsHTTPWebhook(t *testing.T) {
	if err := validateNotifyParams(0, map[string]interface{}{
		"url":    "http://127.0.0.1:8080/notify",
		"method": "POST",
	}); err != nil {
		t.Fatalf("validateNotifyParams() error = %v, want nil for HTTP webhook", err)
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

func TestSendWebhookEmptyBodyAndSinglePassPlaceholderReplacement(t *testing.T) {
	var got map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = nil
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// Empty body string must not panic and should fall back to default payload.
	sendWebhook(server.Client(), map[string]interface{}{
		"url":  server.URL,
		"body": "",
	}, "hello", "world")
	if got["title"] != "hello" || got["content"] != "world" {
		t.Fatalf("empty body fallback payload = %#v, want default title/content", got)
	}

	// Single-pass replacement: {content} inside title must NOT be expanded into content.
	sendWebhook(server.Client(), map[string]interface{}{
		"url":  server.URL,
		"body": `{"t":"{title}","c":"{content}"}`,
	}, "Alert {content}", "REAL-CONTENT")
	if got["t"] != "Alert {content}" || got["c"] != "REAL-CONTENT" {
		t.Fatalf("double replacement occurred: %#v", got)
	}
}

func TestNotifyHTTPClientAllowsLoopbackWebhook(t *testing.T) {
	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("sendWebhook() panic = %v, want loopback target to be reachable", recovered)
		}
	}()
	sendWebhook(notifyHTTPClient, map[string]interface{}{"url": server.URL}, "title", "content")
	select {
	case <-received:
	default:
		t.Fatal("webhook handler was not called")
	}
}

func TestMaskNotifyURLRedactsUserInfoAndNonFinalTokenSegments(t *testing.T) {
	got := maskNotifyURL("https://bot:secret@example.test/hook")
	if strings.Contains(got, "bot") || strings.Contains(got, "secret") {
		t.Fatalf("maskNotifyURL leaked userinfo: %s", got)
	}

	tg := maskNotifyURL("https://api.telegram.org/bot123456789:AAHSuperSecretToken/sendMessage")
	if strings.Contains(tg, "123456789") || strings.Contains(tg, "AAHSuperSecret") {
		t.Fatalf("maskNotifyURL leaked Telegram bot token: %s", tg)
	}
	if !strings.HasSuffix(tg, "/sendMessage") {
		t.Fatalf("maskNotifyURL unexpectedly masked action segment: %s", tg)
	}

	bark := maskNotifyURL("https://api.day.app/AbCdEfGhIjKlMnOpQrSt/push")
	if strings.Contains(bark, "AbCdEfGhIjKlMnOp") {
		t.Fatalf("maskNotifyURL leaked non-final Bark token: %s", bark)
	}
}

func TestRedactNotifyParamsRedactsObjectHeadersBodyWebhookAndFailsClosed(t *testing.T) {
	raw := `{"webhook":"https://api.telegram.org/bot12345:AAHSecretTokenValue/sendMessage","headers":{"Authorization":"Bearer super-secret-token","X-Api-Key":"key-12345"},"body":{"apiKey":"nested-secret","msg":"{title}"}}`
	redacted := redactNotifyParams(0, raw)
	for _, secret := range []string{"super-secret-token", "key-12345", "nested-secret", "AAHSecretToken"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redactNotifyParams leaked %q in %s", secret, redacted)
		}
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(redacted), &parsed); err != nil {
		t.Fatalf("unmarshal redacted params: %v", err)
	}
	if parsed["headers"] != "" {
		t.Fatalf("redacted headers = %#v, want empty string", parsed["headers"])
	}
	if parsed["body"] != notifyBodyRedacted {
		t.Fatalf("redacted body = %#v, want %q", parsed["body"], notifyBodyRedacted)
	}

	// Malformed JSON must fail closed instead of returning raw secrets.
	broken := `{"headers":{"Authorization":"Bearer leaked-raw-secret"`
	if got := redactNotifyParams(0, broken); strings.Contains(got, "leaked-raw-secret") || got != "{}" {
		t.Fatalf("redactNotifyParams(malformed) = %q, want fail-closed {}", got)
	}
}

func TestResolveNotifyParamsBlocksOriginAndMethodChangeExfiltrationAndClearsBody(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE notify(
		id integer primary key autoincrement,
		enable integer,
		method integer,
		params text,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create notify: %v", err)
	}
	storedParams := `{"url":"https://hooks.internal.test/api/real-secret-token-1234","headers":{"Authorization":"Bearer real-auth-token"},"body":"{\"secret\":\"real-body-secret\",\"text\":\"{title}\"}"}`
	if _, err := db.Exec(`INSERT INTO notify(id, enable, method, params) VALUES (1, 1, 0, ?)`, storedParams); err != nil {
		t.Fatalf("insert notify: %v", err)
	}
	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)

	// 1. Different host must NOT inherit stored headers or body.
	resolvedOtherHost, err := resolveNotifyParams(map[string]interface{}{
		"id":     int64(1),
		"method": 0,
		"params": map[string]interface{}{
			"url": "https://evil.attacker.test/steal",
		},
	})
	if err != nil {
		t.Fatalf("resolveNotifyParams(new host explicit) error = %v", err)
	}
	if _, hasHeaders := resolvedOtherHost["headers"]; hasHeaders {
		t.Fatalf("cross-origin change inherited headers: %#v", resolvedOtherHost)
	}
	if _, hasBody := resolvedOtherHost["body"]; hasBody {
		t.Fatalf("cross-origin change inherited body: %#v", resolvedOtherHost)
	}

	// 2. Different host with masked body must return an error.
	if _, err := resolveNotifyParams(map[string]interface{}{
		"id":     int64(1),
		"method": 0,
		"params": map[string]interface{}{
			"url":  "https://evil.attacker.test/steal",
			"body": notifyBodyRedacted,
		},
	}); err == nil {
		t.Fatal("cross-origin change with masked body returned nil error, want error")
	}

	// 3. Scheme downgrade (https -> http) must NOT inherit stored credentials.
	resolvedDowngrade, err := resolveNotifyParams(map[string]interface{}{
		"id":     int64(1),
		"method": 0,
		"params": map[string]interface{}{
			"url": "http://hooks.internal.test/api/hook",
		},
	})
	if err != nil {
		t.Fatalf("resolveNotifyParams(downgrade) error = %v", err)
	}
	if _, hasHeaders := resolvedDowngrade["headers"]; hasHeaders {
		t.Fatalf("scheme downgrade inherited headers: %#v", resolvedDowngrade)
	}

	// 4. Method change must NOT inherit stored credentials.
	resolvedMethodChange, err := resolveNotifyParams(map[string]interface{}{
		"id":     int64(1),
		"method": 2,
		"params": map[string]interface{}{
			"url": "https://oapi.dingtalk.com/robot/send?access_token=fresh",
		},
	})
	if err != nil {
		t.Fatalf("resolveNotifyParams(method change) error = %v", err)
	}
	if _, hasHeaders := resolvedMethodChange["headers"]; hasHeaders {
		t.Fatalf("method change inherited headers: %#v", resolvedMethodChange)
	}

	// 5. Same origin with redacted values restores stored URL, headers, and body.
	resolvedSameOrigin, err := resolveNotifyParams(map[string]interface{}{
		"id":     int64(1),
		"method": 0,
		"params": map[string]interface{}{
			"url":     "https://hooks.internal.test/api/****1234",
			"headers": "",
			"body":    notifyBodyRedacted,
		},
	})
	if err != nil {
		t.Fatalf("resolveNotifyParams(same origin) error = %v", err)
	}
	if resolvedSameOrigin["url"] != "https://hooks.internal.test/api/real-secret-token-1234" {
		t.Fatalf("url not restored on same origin: %#v", resolvedSameOrigin["url"])
	}
	if h, ok := resolvedSameOrigin["headers"].(map[string]interface{}); !ok || h["Authorization"] != "Bearer real-auth-token" {
		t.Fatalf("headers not restored on same origin: %#v", resolvedSameOrigin["headers"])
	}
	if resolvedSameOrigin["body"] != `{"secret":"real-body-secret","text":"{title}"}` {
		t.Fatalf("body not restored on same origin: %#v", resolvedSameOrigin["body"])
	}

	// 6. Explicit empty string body clears stored body while keeping same-origin headers/url.
	resolvedClearBody, err := resolveNotifyParams(map[string]interface{}{
		"id":     int64(1),
		"method": 0,
		"params": map[string]interface{}{
			"url":  "https://hooks.internal.test/api/****1234",
			"body": "",
		},
	})
	if err != nil {
		t.Fatalf("resolveNotifyParams(clear body) error = %v", err)
	}
	if _, hasBody := resolvedClearBody["body"]; hasBody {
		t.Fatalf("explicit empty body was not cleared: %#v", resolvedClearBody["body"])
	}
}

func TestNotifyHTTPClientRejectsCrossHostAndDowngradeRedirects(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	var panicked interface{}
	func() {
		defer func() { panicked = recover() }()
		sendWebhook(notifyHTTPClient, map[string]interface{}{
			"url":     redirector.URL,
			"headers": map[string]interface{}{"Authorization": "Bearer secret"},
		}, "t", "c")
	}()
	if panicked == nil {
		t.Fatal("cross-host redirect unexpectedly succeeded")
	}
	if targetCalled {
		t.Fatal("cross-host redirect target was contacted")
	}

	// Verify HTTPS -> HTTP downgrade on same host is rejected.
	prevReq, _ := http.NewRequest(http.MethodPost, "https://example.test/hook", nil)
	nextReq, _ := http.NewRequest(http.MethodPost, "http://example.test/hook", nil)
	if err := checkNotifyRedirect(nextReq, []*http.Request{prevReq}); err == nil {
		t.Fatal("checkNotifyRedirect allowed HTTPS -> HTTP downgrade")
	}
}

func TestNotifyBlocksCloudMetadataSSRF(t *testing.T) {
	for _, blocked := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://[fd00:ec2::254]/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://100.100.100.200/latest/meta-data/",
		"http://[fe80::1]/hook",
	} {
		if err := validateNotifyParams(0, map[string]interface{}{"url": blocked}); err == nil {
			t.Fatalf("validateNotifyParams(%q) = nil, want metadata SSRF rejection", blocked)
		}
	}

	for _, ipStr := range []string{"169.254.169.254", "169.254.170.2", "fd00:ec2::254", "fe80::1", "100.100.100.200", "0.0.0.0"} {
		if !isBlockedMetadataIP(net.ParseIP(ipStr)) {
			t.Fatalf("isBlockedMetadataIP(%s) = false, want true", ipStr)
		}
	}
	for _, ipStr := range []string{"127.0.0.1", "::1", "192.168.1.50", "10.0.0.5"} {
		if isBlockedMetadataIP(net.ParseIP(ipStr)) {
			t.Fatalf("isBlockedMetadataIP(%s) = true, want false", ipStr)
		}
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
		if string(publicErr) != msg.NotifySendFail {
			t.Fatalf("panic = %q, want generic notify failure", publicErr)
		}
		if strings.Contains(string(publicErr), "very-secret-token") || strings.Contains(string(publicErr), "access_token") {
			t.Fatalf("panic leaked secret URL: %q", publicErr)
		}
	}()

	sendNotifyRequest(client, req)
}
