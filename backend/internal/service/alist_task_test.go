package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAlistFlexibleIDAcceptsStringAndNumber(t *testing.T) {
	var stringID alistFlexibleID
	if err := json.Unmarshal([]byte(`"task-9"`), &stringID); err != nil {
		t.Fatalf("string id: %v", err)
	}
	if stringID != "task-9" {
		t.Fatalf("string id = %q", stringID)
	}
	var numericID alistFlexibleID
	if err := json.Unmarshal([]byte(`42`), &numericID); err != nil {
		t.Fatalf("numeric id: %v", err)
	}
	if numericID != "42" {
		t.Fatalf("numeric id = %q", numericID)
	}
}

func TestTaskUndoneByIDsContextKeepsOnlyWantedTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":[{"id":"keep","state":1,"progress":10,"name":"ignored"},{"id":123,"state":1,"progress":20},{"id":"drop","state":1,"progress":90}]}`))
	}))
	defer server.Close()

	client := &AlistClient{URL: server.URL, client: server.Client()}
	wanted := map[string]struct{}{"keep": {}, "123": {}}
	got, err := client.TaskUndoneByIDsContext(context.Background(), taskItemTypeCopy, wanted, maxUndonePickups)
	if err != nil {
		t.Fatalf("TaskUndoneByIDsContext() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (%#v)", len(got), got)
	}
	if _, ok := got["drop"]; ok {
		t.Fatal("unwanted task kept")
	}
	if got["keep"].Progress != 10 || got["123"].Progress != 20 {
		t.Fatalf("tasks = %#v", got)
	}
}

func TestTaskInfoContextDecodesTypedProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"id":"copy-task","state":1,"progress":55.5,"error":""}}`))
	}))
	defer server.Close()

	client := &AlistClient{URL: server.URL, client: server.Client()}
	task, err := client.TaskInfoContext(context.Background(), "copy-task", taskItemTypeCopy)
	if err != nil {
		t.Fatalf("TaskInfoContext() error: %v", err)
	}
	if task.idString() != "copy-task" || task.State != 1 || task.Progress != 55.5 {
		t.Fatalf("task = %#v", task)
	}
}
