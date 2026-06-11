package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"opensync/pkg/util"
	"strings"
	"time"
)

const maxNotifyResponseBytes = 1 << 20 // 1MB

var notifyHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	},
}

// GetNotifyList returns notify list
func GetNotifyList() []map[string]interface{} {
	list, err := mapper.GetNotifyList(false)
	if err != nil {
		panic(err.Error())
	}
	return list
}

// AddNewNotify adds a new notify config
func AddNewNotify(notify map[string]interface{}) {
	_, err := mapper.AddNotify(notify)
	if err != nil {
		panic(err.Error())
	}
}

// EditNotify updates a notify config
func EditNotify(notify map[string]interface{}) {
	err := mapper.EditNotify(notify)
	if err != nil {
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

// TestNotify sends a test notification
func TestNotify(notify map[string]interface{}) {
	defer func() {
		if r := recover(); r != nil {
			panicPublic(fmt.Sprintf("%v", r))
		}
	}()
	msg := i18n.G("notify_test_msg")
	sendNotify(notify, "OpenSync Test", msg, false)
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
		4: "Aborted", 5: "Timeout", 6: "Failed", 7: "Stopped", 8: "No sync needed",
	}
	statusName := statusNames[status]
	if status < 0 || status > 8 {
		statusName = "Unknown"
	}

	needNotSync := false
	if status == 2 {
		allNum := util.ToInt(taskNum["allNum"])
		if allNum == 0 {
			needNotSync = true
			statusName = statusNames[8]
		}
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
	srcPath := strings.Join(parseSrcPaths(job["srcPath"]), "、")
	dstPath := strings.Join(parseDstPaths(job["dstPath"]), "、")

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
					msg := i18n.G("notify_error")
					msg = strings.Replace(msg, "{}", fmt.Sprintf("%v", r), 1)
					log.Printf("%s", msg)
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
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err.Error())
	}
	defer resp.Body.Close()
	bodyBytes, err := readAllWithLimit(resp.Body, maxNotifyResponseBytes)
	if err != nil {
		panic(err.Error())
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(bodyBytes))
		if msg == "" {
			msg = resp.Status
		}
		panic(msg)
	}
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
			jsonData, err := json.Marshal(body)
			if err != nil {
				panic(err.Error())
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
	sendNotifyRequest(client, req)
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
	sendNotifyRequest(client, req)
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
	tokenURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s", corpID, corpSecret)
	req, err := buildNotifyRequest(http.MethodGet, tokenURL, nil, "")
	if err != nil {
		panic(err.Error())
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err.Error())
	}
	defer resp.Body.Close()
	tokenBody, _ := io.ReadAll(resp.Body)
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
	msgURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s", tokenResult.AccessToken)
	msgReq, err := buildNotifyRequest(http.MethodPost, msgURL, bytes.NewReader(jsonData), "application/json")
	if err != nil {
		panic(err.Error())
	}
	sendNotifyRequest(client, msgReq)
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
	sendNotifyRequest(client, req)
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
