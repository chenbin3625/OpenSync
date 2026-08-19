package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"strings"
	"syscall"
	"time"
)

const maxNotifyResponseBytes = 1 << 20 // 1MB

var notifyHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		// DialContext intercepts the resolved address to block SSRF attempts
		// (private/loopback/link-local targets) before any connection is made.
		DialContext: ssrfSafeDialContext(&net.Dialer{Timeout: 15 * time.Second}),
	},
}

// ssrfSafeDialContext wraps a dialer so that connections to non-routable or
// internal IP ranges are rejected. The control runs after DNS resolution with
// the resolved IP, which avoids TOCTOU gaps between resolving and dialing.
func ssrfSafeDialContext(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer.Control = func(network, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("invalid notify target address: %s", address)
		}
		if isBlockedNotifyIP(ip) {
			return fmt.Errorf("notify target %s is not allowed", ip)
		}
		return nil
	}
	return dialer.DialContext
}

func isBlockedNotifyIP(ip net.IP) bool {
	if config.GetConfig().Server.AllowInternalWebhook {
		return ip.IsUnspecified()
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

// GetNotifyList returns notify list with secret fields redacted so tokens
// never reach the client DOM. Raw secrets are still held in the DB and used
// internally by SendTaskNotification; only the list response is masked.
func GetNotifyList() []map[string]interface{} {
	list, err := mapper.GetNotifyList(false)
	if err != nil {
		panic(err.Error())
	}
	for _, notify := range list {
		method := util.ToInt(notify["method"])
		notify["params"] = redactNotifyParams(method, fmt.Sprintf("%v", notify["params"]))
	}
	return list
}

// notifySecretKeys are param keys whose values are redacted in list responses.
var notifySecretKeys = map[int][]string{
	0: {"url", "headers"},           // custom webhook: URL may embed token, headers carry auth
	1: {"sendKey"},                  // Server酱
	2: {"url", "webhook"},           // 钉钉 (access_token in URL query)
	3: {"corpsecret", "corpSecret"}, // 企业微信应用密钥
	4: {"url", "webhook"},           // 飞书 (token in URL path)
}

const notifyRedactionMarker = "****"

func maskSecretValue(value string) string {
	if len(value) <= 4 {
		return notifyRedactionMarker
	}
	return notifyRedactionMarker + value[len(value)-4:]
}

// maskNotifyURL redacts token-like segments in a webhook URL while keeping
// the host visible so the card summary stays identifiable. DingTalk embeds
// the token in a query param (access_token); Lark embeds it in the path.
func maskNotifyURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return maskSecretValue(rawURL)
	}
	masked := false
	if u.User != nil {
		u.User = url.User(notifyRedactionMarker)
		masked = true
	}
	q := u.Query()
	for k, vals := range q {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "token") || strings.Contains(lk, "key") || strings.Contains(lk, "secret") {
			for i := range vals {
				vals[i] = maskSecretValue(vals[i])
			}
			q[k] = vals
			masked = true
		}
	}
	if masked {
		u.RawQuery = q.Encode()
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) > 0 {
		last := segs[len(segs)-1]
		if len(last) >= 16 {
			segs[len(segs)-1] = maskSecretValue(last)
			u.Path = "/" + strings.Join(segs, "/")
		}
	}
	return u.String()
}

// redactNotifyParams returns paramsStr with secret fields masked for display.
func redactNotifyParams(method int, paramsStr string) string {
	params, err := parseNotifyParams(paramsStr)
	if err != nil {
		return paramsStr
	}
	for _, key := range notifySecretKeys[method] {
		v, ok := params[key]
		if !ok || v == nil {
			continue
		}
		s, _ := v.(string)
		if s == "" {
			continue
		}
		switch key {
		case "headers":
			params[key] = ""
		case "url", "webhook":
			params[key] = maskNotifyURL(s)
		default:
			params[key] = maskSecretValue(s)
		}
	}
	out, err := json.Marshal(params)
	if err != nil {
		return paramsStr
	}
	return string(out)
}

// isMaskedSecretValue reports whether value looks like a redacted secret, so
// the caller should preserve the existing stored value instead of overwriting.
func isMaskedSecretValue(value string) bool {
	if strings.Contains(value, notifyRedactionMarker) {
		return true
	}
	decoded, err := url.QueryUnescape(value)
	return err == nil && strings.Contains(decoded, notifyRedactionMarker)
}

// resolveNotifyParams merges incoming params with stored secrets for fields
// that were redacted (masked) or left empty. For new configs (no id) the
// incoming params are returned unchanged. The returned map contains real
// secret values, suitable for sending a test or persisting. An explicitly
// empty headers object ({}) clears the stored headers instead of restoring
// them.
func resolveNotifyParams(notify map[string]interface{}) (map[string]interface{}, error) {
	method := util.ToInt(notify["method"])
	incoming, err := parseNotifyParams(fmt.Sprintf("%v", notify["params"]))
	if err != nil {
		return nil, err
	}
	notifyID := util.ToInt64(notify["id"])
	if notifyID <= 0 {
		return incoming, nil
	}
	existing, err := mapper.GetNotifyByID(notifyID)
	if err != nil || existing == nil {
		return incoming, nil
	}
	existingParams, _ := parseNotifyParams(fmt.Sprintf("%v", existing["params"]))
	for _, key := range notifySecretKeys[method] {
		v, ok := incoming[key]
		if !ok || v == nil {
			if ev, ok2 := existingParams[key]; ok2 && ev != nil {
				incoming[key] = ev
			}
			continue
		}
		s, isStr := v.(string)
		if isStr {
			if s == "" || isMaskedSecretValue(s) {
				if ev, ok2 := existingParams[key]; ok2 && ev != nil {
					incoming[key] = ev
				}
			}
			continue
		}
		// Non-string values (e.g. the parsed headers object) are real input
		// and replace the stored value. An explicitly-empty headers object
		// means the user cleared headers, so drop the stored value.
		if key == "headers" {
			if hMap, ok := v.(map[string]interface{}); ok && len(hMap) == 0 {
				delete(incoming, key)
			}
		}
	}
	return incoming, nil
}

func validateNotifyParams(method int, params map[string]interface{}) error {
	switch method {
	case 0:
		if err := validateNotifyWebhookURL(paramString(params, "url", "webhook")); err != nil {
			return err
		}
		return validateWebhookMethod(paramString(params, "method", "httpMethod"))
	case 1:
		if paramString(params, "sendKey") == "" {
			return errors.New(msg.NotifyParamInvalid)
		}
		return nil
	case 2, 4:
		return validateNotifyHTTPSURL(paramString(params, "url", "webhook"))
	case 3:
		if paramString(params, "corpid", "corpId") == "" ||
			paramString(params, "corpsecret", "corpSecret") == "" ||
			paramString(params, "agentid", "agentId") == "" {
			return errors.New(msg.NotifyParamInvalid)
		}
		return nil
	default:
		return errors.New(msg.NotifyMethodInvalid)
	}
}

func validateWebhookMethod(method string) error {
	if method == "" {
		return nil
	}
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodPost, http.MethodPut:
		return nil
	default:
		return errors.New(msg.NotifyParamInvalid)
	}
}

func validateNotifyWebhookURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New(msg.NotifyURLInvalid)
	}
	scheme := strings.ToLower(u.Scheme)
	if config.GetConfig().Server.AllowInternalWebhook {
		if scheme == "http" || scheme == "https" {
			return nil
		}
		return errors.New(msg.NotifyURLInvalid)
	}
	if scheme != "https" {
		return errors.New(msg.NotifyURLInvalid)
	}
	return nil
}

func validateNotifyHTTPSURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New(msg.NotifyURLInvalid)
	}
	if strings.ToLower(u.Scheme) != "https" {
		return errors.New(msg.NotifyURLInvalid)
	}
	return nil
}

// AddNewNotify adds a new notify config
func AddNewNotify(notify map[string]interface{}) {
	method := util.ToInt(notify["method"])
	params, err := parseNotifyParams(fmt.Sprintf("%v", notify["params"]))
	if err != nil {
		panic(err.Error())
	}
	if err := validateNotifyParams(method, params); err != nil {
		panicPublic(err.Error())
	}
	_, err = mapper.AddNotify(notify)
	if err != nil {
		panic(err.Error())
	}
}

// EditNotify updates a notify config. Secret fields that were redacted in
// the list view (or left empty) are preserved from the stored config so the
// user can edit other fields without re-entering credentials.
func EditNotify(notify map[string]interface{}) {
	resolved, err := resolveNotifyParams(notify)
	if err != nil {
		panic(err.Error())
	}
	if err := validateNotifyParams(util.ToInt(notify["method"]), resolved); err != nil {
		panicPublic(err.Error())
	}
	out, err := json.Marshal(resolved)
	if err != nil {
		panic(err.Error())
	}
	notify["params"] = string(out)
	if err := mapper.EditNotify(notify); err != nil {
		panic(err.Error())
	}
}

// UpdateNotifyStatus updates notify enable status
func UpdateNotifyStatus(notifyID int64, enable int) {
	err := mapper.UpdateNotifyStatus(notifyID, enable)
	if err != nil {
		panic(err.Error())
	}
}

// DeleteNotify deletes a notify config
func DeleteNotify(notifyID int64) {
	err := mapper.DeleteNotify(notifyID)
	if err != nil {
		panic(err.Error())
	}
}

// TestNotify sends a test notification. Secrets that were redacted in the
// list view are restored from the stored config (when an id is given) so the
// test sends with real credentials.
func TestNotify(notify map[string]interface{}) {
	defer func() {
		if r := recover(); r != nil {
			if publicErr, ok := r.(model.PublicError); ok {
				panic(publicErr)
			}
			log.Printf("notify test failed: %v", r)
			panicPublic(msg.NotifySendFail)
		}
	}()
	resolved, err := resolveNotifyParams(notify)
	if err != nil {
		panic(err.Error())
	}
	if err := validateNotifyParams(util.ToInt(notify["method"]), resolved); err != nil {
		panicPublic(err.Error())
	}
	out, err := json.Marshal(resolved)
	if err != nil {
		panic(err.Error())
	}
	notify["params"] = string(out)
	testMsg := msg.NotifyTestMsg
	sendNotify(notify, "OpenSync Test", testMsg, false)
}

// SendTaskNotification sends notification after task completion
func SendTaskNotification(taskID int64, status int, taskNum map[string]interface{}, duration int, createTime float64) {
	notifyList, err := mapper.GetNotifyList(true)
	if err != nil || len(notifyList) == 0 {
		return
	}

	job, err := mapper.GetJobByTaskID(taskID)
	if err != nil {
		return
	}

	statusNames := map[int]string{
		0: "Waiting", 1: "Running", 2: "Success", 3: "Partial Success",
		4: "Stopped", 5: "Timeout", 6: "System Failed", 7: "Failed", 8: "No sync needed",
	}
	statusName := statusNames[status]
	if status < 0 || status > 8 {
		statusName = "Unknown"
	}

	needNotSync := status == taskStatusNoSync.Int()
	if status == taskStatusSuccess.Int() {
		allNum := util.ToInt(taskNum["allNum"])
		if allNum == 0 {
			needNotSync = true
		}
	}
	if needNotSync {
		statusName = statusNames[8]
	}

	remark := ""
	if r, ok := job["remark"]; ok && r != nil {
		remark = fmt.Sprintf("%v", r)
	}
	if remark != "" {
		statusName = remark + ": " + statusName
	}

	title := fmt.Sprintf("OpenSync - %s", statusName)

	successNum := util.ToInt(taskNum["successNum"])
	failNum := util.ToInt(taskNum["failNum"])
	allNum := util.ToInt(taskNum["allNum"])
	srcPath := strings.Join(parsePathList(job["srcPath"]), "、")
	dstPath := strings.Join(parsePathList(job["dstPath"]), "、")

	content := fmt.Sprintf("Source: %s | Target: %s | Total: %d | Success: %d | Fail: %d",
		srcPath, dstPath, allNum, successNum, failNum)

	if createTime > 0 && duration > 0 {
		hours, minutes, seconds := util.ConvertSeconds(duration)
		durationText := fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
		sumSize := util.ToInt64(taskNum["sumSize"])
		content += fmt.Sprintf(" | Duration: %s | Size: %s", durationText, util.ConvertBytes(sumSize))
	}

	if (status > 3 && status < 6) || status == 7 {
		content += fmt.Sprintf(" | Status: %s", statusName)
	}

	for _, notify := range notifyList {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("%s", msg.NotifyError(fmt.Sprintf("%v", r)))
				}
			}()
			sendNotify(notify, title, content, needNotSync)
		}()
	}
}

// sendNotify sends a notification via the configured method
func sendNotify(notify map[string]interface{}, title, content string, needNotSync bool) {
	paramsStr := fmt.Sprintf("%v", notify["params"])
	params, err := parseNotifyParams(paramsStr)
	if err != nil {
		panic(err.Error())
	}

	method := util.ToInt(notify["method"])
	if err := validateNotifyParams(method, params); err != nil {
		panicPublic(err.Error())
	}

	// Check notSendNull flag
	if needNotSync {
		if v, ok := params["notSendNull"]; ok {
			if util.ToBool(v) {
				return
			}
		}
	}

	switch method {
	case 0: // Custom webhook
		sendWebhook(notifyHTTPClient, params, title, content)
	case 1: // ServerChan
		sendServerChan(notifyHTTPClient, params, title, content)
	case 2: // DingTalk
		sendDingTalk(notifyHTTPClient, params, title, content)
	case 3: // WeCom (Enterprise WeChat)
		sendWeCom(notifyHTTPClient, params, title, content)
	case 4: // Lark (Feishu)
		sendLark(notifyHTTPClient, params, title, content)
	}
}

func parseNotifyParams(paramsStr string) (map[string]interface{}, error) {
	paramsStr = strings.TrimSpace(paramsStr)
	if paramsStr == "" || paramsStr == "<nil>" {
		return map[string]interface{}{}, nil
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]interface{}{}
	}
	return params, nil
}

func buildNotifyRequest(method, urlStr string, body io.Reader, contentType string) (*http.Request, error) {
	method = strings.TrimSpace(method)
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return nil, fmt.Errorf("url is required")
	}
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

func sendNotifyRequest(client *http.Client, req *http.Request) {
	sendNotifyRequestBytes(client, req)
}

// sendNotifyRequestBytes sends the request, validates the HTTP status, and
// returns the response body so callers can inspect provider-specific error
// codes. DingTalk/Lark/ServerChan/WeCom return HTTP 200 with a non-zero
// errcode/code/errno in the body on failure, which a status-only check misses.
func sendNotifyRequestBytes(client *http.Client, req *http.Request) []byte {
	resp := doNotifyRequest(client, req)
	defer resp.Body.Close()
	bodyBytes, err := readAllWithLimit(resp.Body, maxNotifyResponseBytes)
	if err != nil {
		panic(err.Error())
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(bodyBytes))
		if msg != "" {
			log.Printf("notify request failed: status=%s body=%q", resp.Status, msg)
		}
		panic(fmt.Sprintf("notify request failed: %s", resp.Status))
	}
	return bodyBytes
}

// notifyProviderError returns a non-nil error when the provider's JSON response
// body indicates failure via one of the given fields being non-zero. Returns nil
// for non-JSON bodies (e.g. custom webhooks) or when no recognized field is
// present, so callers don't false-positive on arbitrary response shapes.
func notifyProviderError(body []byte, fields ...string) error {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil
	}
	for _, field := range fields {
		if v, ok := m[field]; ok {
			if code := util.ToInt(v); code != 0 {
				return fmt.Errorf("notify %s=%d: %s", field, code, strings.TrimSpace(string(body)))
			}
			return nil
		}
	}
	return nil
}

func doNotifyRequest(client *http.Client, req *http.Request) *http.Response {
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		// On a redirect error client.Do can return a non-nil resp together
		// with err; close the body or the underlying connection leaks.
		if resp != nil {
			_ = resp.Body.Close()
		}
		log.Printf("notify request failed: target=%s error=%s", notifyRequestTarget(req), notifyNetworkError(err))
		panicPublic(msg.NotifySendFail)
	}
	return resp
}

func notifyRequestTarget(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "<unknown>"
	}
	return req.URL.Scheme + "://" + req.URL.Host
}

func notifyNetworkError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Sprintf("%s: %v", urlErr.Op, urlErr.Err)
	}
	return fmt.Sprintf("%T", err)
}

func sendWebhook(client *http.Client, params map[string]interface{}, title, content string) {
	urlStr := paramString(params, "url", "webhook")
	method := "POST"
	if m := paramString(params, "method", "httpMethod"); m != "" {
		method = strings.ToUpper(m)
	}
	contentType := paramString(params, "contentType")
	if contentType == "" {
		contentType = "application/json"
	}
	titleName := paramString(params, "titleName")
	if titleName == "" {
		titleName = "title"
	}
	contentName := paramString(params, "contentName")
	if contentName == "" {
		contentName = "content"
	}
	needContent := true
	if v, ok := params["needContent"]; ok {
		needContent = util.ToBool(v)
	}

	body := map[string]interface{}{
		titleName: title,
	}
	if needContent {
		body[contentName] = content
	}
	if customBody, ok := params["body"]; ok && customBody != nil {
		bodyStr := fmt.Sprintf("%v", customBody)
		bodyStr = strings.ReplaceAll(bodyStr, "{title}", jsonStringContent(title))
		bodyStr = strings.ReplaceAll(bodyStr, "{content}", jsonStringContent(content))
		body = nil
		if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
			panic(err.Error())
		}
	}

	var req *http.Request
	var err error
	if method == "GET" {
		req, err = buildNotifyRequest(http.MethodGet, urlStr, nil, "")
		if err != nil {
			panic(err.Error())
		}
		q := req.URL.Query()
		q.Set(titleName, title)
		if needContent {
			q.Set(contentName, content)
		}
		req.URL.RawQuery = q.Encode()
	} else {
		if contentType == "application/x-www-form-urlencoded" {
			formBody := make([]string, 0, len(body))
			for k, v := range body {
				formBody = append(formBody, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(fmt.Sprintf("%v", v))))
			}
			req, err = buildNotifyRequest(method, urlStr, strings.NewReader(strings.Join(formBody, "&")), contentType)
		} else {
			jsonData, marshalErr := json.Marshal(body)
			if marshalErr != nil {
				panic(marshalErr.Error())
			}
			req, err = buildNotifyRequest(method, urlStr, bytes.NewReader(jsonData), contentType)
		}
		if err != nil {
			panic(err.Error())
		}
	}

	if headers, ok := params["headers"]; ok && headers != nil {
		if hMap, ok := headers.(map[string]interface{}); ok {
			for k, v := range hMap {
				req.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	sendNotifyRequest(client, req)
}

func jsonStringContent(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	encodedStr := string(encoded)
	if len(encodedStr) < 2 {
		return encodedStr
	}
	return encodedStr[1 : len(encodedStr)-1]
}

func sendServerChan(client *http.Client, params map[string]interface{}, title, content string) {
	sendKey := paramString(params, "sendKey")
	version := "v1"
	if v, ok := params["version"]; ok {
		version = fmt.Sprintf("%v", v)
	}

	var urlStr string
	if version == "v3" {
		urlStr = fmt.Sprintf("https://sctapi.ftqq.com/%s.send", sendKey)
	} else {
		urlStr = fmt.Sprintf("https://sc.ftqq.com/%s.send", sendKey)
	}

	body := map[string]string{
		"title": title,
		"desp":  content,
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		panic(err.Error())
	}
	req, err := buildNotifyRequest(http.MethodPost, urlStr, bytes.NewReader(jsonData), "application/json")
	if err != nil {
		panic(err.Error())
	}
	respBody := sendNotifyRequestBytes(client, req)
	if err := notifyProviderError(respBody, "code", "errno"); err != nil {
		panic(err.Error())
	}
}

func sendDingTalk(client *http.Client, params map[string]interface{}, title, content string) {
	webhook := paramString(params, "url", "webhook")
	body := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": title + "\n" + content,
		},
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		panic(err.Error())
	}
	req, err := buildNotifyRequest(http.MethodPost, webhook, bytes.NewReader(jsonData), "application/json")
	if err != nil {
		panic(err.Error())
	}
	respBody := sendNotifyRequestBytes(client, req)
	if err := notifyProviderError(respBody, "errcode"); err != nil {
		panic(err.Error())
	}
}

func sendWeCom(client *http.Client, params map[string]interface{}, title, content string) {
	corpID := paramString(params, "corpid", "corpId")
	corpSecret := paramString(params, "corpsecret", "corpSecret")
	agentID := paramString(params, "agentid", "agentId")
	toUser := "@all"
	if u := paramString(params, "touser", "toUser"); u != "" {
		toUser = u
	}

	// Get access token
	tokenURL := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?" + url.Values{
		"corpid":     {corpID},
		"corpsecret": {corpSecret},
	}.Encode()
	req, err := buildNotifyRequest(http.MethodGet, tokenURL, nil, "")
	if err != nil {
		panic(err.Error())
	}
	resp := doNotifyRequest(client, req)
	defer resp.Body.Close()
	tokenBody, err := readAllWithLimit(resp.Body, maxNotifyResponseBytes)
	if err != nil {
		panic(err.Error())
	}
	var tokenResult struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
	}
	if err := json.Unmarshal(tokenBody, &tokenResult); err != nil {
		panic(err.Error())
	}
	if tokenResult.ErrCode != 0 {
		panic(fmt.Sprintf("WeCom token error: %s", strings.TrimSpace(string(tokenBody))))
	}

	// Send message
	msgBody := map[string]interface{}{
		"touser":  toUser,
		"msgtype": "text",
		"agentid": agentID,
		"text": map[string]string{
			"content": title + "\n" + content,
		},
	}
	jsonData, err := json.Marshal(msgBody)
	if err != nil {
		panic(err.Error())
	}
	msgURL := "https://qyapi.weixin.qq.com/cgi-bin/message/send?" + url.Values{
		"access_token": {tokenResult.AccessToken},
	}.Encode()
	msgReq, err := buildNotifyRequest(http.MethodPost, msgURL, bytes.NewReader(jsonData), "application/json")
	if err != nil {
		panic(err.Error())
	}
	msgRespBody := sendNotifyRequestBytes(client, msgReq)
	if err := notifyProviderError(msgRespBody, "errcode"); err != nil {
		panic(err.Error())
	}
}

func sendLark(client *http.Client, params map[string]interface{}, title, content string) {
	webhook := paramString(params, "url", "webhook")
	body := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": title,
				},
			},
			"elements": []map[string]interface{}{
				{
					"tag":     "markdown",
					"content": content,
				},
			},
		},
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		panic(err.Error())
	}
	req, err := buildNotifyRequest(http.MethodPost, webhook, bytes.NewReader(jsonData), "application/json")
	if err != nil {
		panic(err.Error())
	}
	respBody := sendNotifyRequestBytes(client, req)
	if err := notifyProviderError(respBody, "code", "StatusCode"); err != nil {
		panic(err.Error())
	}
}

func paramString(params map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := params[key]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
