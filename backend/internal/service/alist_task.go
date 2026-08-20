package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"opensync/internal/msg"
)

// alistRemoteTask is the AList copy/move task snapshot fields the poller
// actually reads. Unknown envelope keys (name, status text, timestamps) are
// skipped so a busy host cannot force a map[string]interface{} per task.
type alistRemoteTask struct {
	ID       alistFlexibleID `json:"id"`
	State    int             `json:"state"`
	Progress float64         `json:"progress"`
	Error    string          `json:"error"`
}

func (t alistRemoteTask) idString() string {
	return string(t.ID)
}

// alistFlexibleID accepts AList's string or numeric task ids without boxing
// them as interface{}.
type alistFlexibleID string

func (id *alistFlexibleID) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		*id = ""
		return nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		*id = alistFlexibleID(text)
		return nil
	}
	*id = alistFlexibleID(raw)
	return nil
}

func (c *AlistClient) TaskUndoneListContext(ctx context.Context, copyType taskItemType) ([]alistRemoteTask, error) {
	tasks, err := c.TaskUndoneByIDsContext(ctx, copyType, nil, 0)
	if err != nil {
		return nil, err
	}
	out := make([]alistRemoteTask, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task)
	}
	return out, nil
}

func (c *AlistClient) TaskUndoneByIDsContext(ctx context.Context, copyType taskItemType, wanted map[string]struct{}, limit int) (map[string]alistRemoteTask, error) {
	apiPath := fmt.Sprintf("/api/admin/task/%s/undone", alistTaskGroup(copyType))
	resp, err := c.startRequest(ctx, http.MethodPost, apiPath, struct{}{}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s (HTTP %d)", msg.CodeNot200, resp.StatusCode)
	}
	byID, code, message, err := decodeAlistRemoteTaskMap(resp.Body, maxUndoneResponseBytes, wanted, limit)
	if err != nil {
		return nil, err
	}
	if err := c.checkAlistCode(code, message); err != nil {
		return nil, err
	}
	return byID, nil
}

func (c *AlistClient) TaskInfoContext(ctx context.Context, taskID string, copyType taskItemType) (alistRemoteTask, error) {
	data, err := c.taskActionContext(ctx, taskID, copyType, "info")
	if err != nil {
		return alistRemoteTask{}, err
	}
	var result alistRemoteTask
	if err := json.Unmarshal(data, &result); err != nil {
		return alistRemoteTask{}, err
	}
	if result.ID == "" {
		result.ID = alistFlexibleID(taskID)
	}
	return result, nil
}

func decodeAlistRemoteTaskMap(r io.Reader, limit int64, wanted map[string]struct{}, maxKeep int) (map[string]alistRemoteTask, int, string, error) {
	dec := json.NewDecoder(&capReader{r: r, limit: limit})
	if err := consumeDelim(dec, '{'); err != nil {
		return nil, 0, "", err
	}
	byID := make(map[string]alistRemoteTask)
	var code int
	var message string
	for dec.More() {
		key, err := decodeObjectKey(dec)
		if err != nil {
			return nil, 0, "", err
		}
		switch key {
		case "code":
			if err := dec.Decode(&code); err != nil {
				return nil, 0, "", err
			}
		case "message":
			if err := dec.Decode(&message); err != nil {
				return nil, 0, "", err
			}
		case "data":
			decoded, err := decodeAlistRemoteTaskArray(dec, wanted, maxKeep)
			if err != nil {
				return nil, 0, "", err
			}
			byID = decoded
		default:
			if err := skipJSONValue(dec); err != nil {
				return nil, 0, "", err
			}
		}
	}
	if err := consumeDelim(dec, '}'); err != nil {
		return nil, 0, "", err
	}
	return byID, code, message, nil
}

func decodeAlistRemoteTaskArray(dec *json.Decoder, wanted map[string]struct{}, maxKeep int) (map[string]alistRemoteTask, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok == nil {
		return map[string]alistRemoteTask{}, nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return nil, fmt.Errorf("AList undone data is not an array")
	}
	byID := make(map[string]alistRemoteTask)
	for dec.More() {
		if maxKeep > 0 && len(byID) >= maxKeep {
			if err := skipJSONValue(dec); err != nil {
				return nil, err
			}
			continue
		}
		var task alistRemoteTask
		if err := dec.Decode(&task); err != nil {
			return nil, err
		}
		id := task.idString()
		if id == "" {
			continue
		}
		if wanted != nil {
			if _, ok := wanted[id]; !ok {
				continue
			}
		}
		byID[id] = task
	}
	return byID, consumeDelim(dec, ']')
}
