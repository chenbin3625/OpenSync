package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProgressHubUnsubscribeDoesNotCloseChannel(t *testing.T) {
	hub := &progressHub{
		subscribers: make(map[int64]map[chan []byte]struct{}),
		pending:     make(map[int64]struct{}),
	}
	ch := make(chan []byte, 1)

	hub.subscribe(1, ch)
	hub.unsubscribe(1, ch)

	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatalf("unsubscribe closed channel; delayed broadcaster could panic sending to a stale snapshot")
		}
	default:
	}
	if subs := hub.subscriberSnapshot(1); len(subs) != 0 {
		t.Fatalf("subscriberSnapshot() length = %d, want 0", len(subs))
	}
}

func sampleDoingItem(progress float64) streamDoingItem {
	return streamDoingItem{
		AlistTaskID: "copy-1",
		FileName:    "a.bin",
		SrcPath:     "/src",
		DstPath:     "/dst",
		FileSize:    1000,
		Type:        0,
		Status:      1,
		Progress:    progress,
		CreateTime:  100,
	}
}

func sampleCurrent(progress float64, extra ...streamDoingItem) jobCurrentPayload {
	doing := []streamDoingItem{sampleDoingItem(progress)}
	doing = append(doing, extra...)
	return jobCurrentPayload{
		TaskID:     9,
		CreateTime: 100,
		Duration:   4,
		DoingTask:  doing,
		DoneSize:   int64(progress * 10),
		RemainSize: int64(1000 - progress*10),
	}
}

func TestStreamDoingItemOmitsIdleFields(t *testing.T) {
	encoded, err := json.Marshal(sampleDoingItem(12))
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	body := string(encoded)
	if strings.Contains(body, "errMsg") || strings.Contains(body, "isPath") || strings.Contains(body, "taskId") {
		t.Fatalf("stream row still contains idle fields: %s", body)
	}
	if !strings.Contains(body, `"alistTaskId":"copy-1"`) || !strings.Contains(body, `"fileName":"a.bin"`) {
		t.Fatalf("stream row = %s", body)
	}
}

func TestPrepareStreamPayloadPatchesUnchangedFileSet(t *testing.T) {
	hub := &progressHub{
		subscribers: make(map[int64]map[chan []byte]struct{}),
		pending:     make(map[int64]struct{}),
		frames:      make(map[int64]progressFrame),
	}

	first := sampleCurrent(10)
	hub.prepareStreamPayload(7, &first, true)
	if first.DoingTask == nil {
		t.Fatalf("snapshot payload must keep doingTask")
	}
	if first.DoingPatch != nil {
		t.Fatalf("snapshot payload must not include doingPatch")
	}

	second := sampleCurrent(55)
	second.Duration = 5
	hub.prepareStreamPayload(7, &second, false)
	if second.DoingTask != nil {
		t.Fatalf("progress-only update must omit doingTask")
	}
	if len(second.DoingPatch) != 1 {
		t.Fatalf("doingPatch len = %d, want 1", len(second.DoingPatch))
	}
	if second.DoingPatch[0].AlistTaskID != "copy-1" {
		t.Fatalf("patch key = %#v", second.DoingPatch[0])
	}
	if second.DoingPatch[0].Progress != 55 {
		t.Fatalf("patch progress = %#v, want 55", second.DoingPatch[0])
	}
	if second.DoingPatch[0].FileName != "" {
		t.Fatalf("alist-keyed patch should not repeat fileName")
	}

	encoded, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	if strings.Contains(string(encoded), "errMsg") || strings.Contains(string(encoded), "a.bin") {
		t.Fatalf("patch JSON still contains snapshot fields: %s", encoded)
	}
}

func TestMarshalProgressJSONDoesNotEscapePathSlashes(t *testing.T) {
	encoded, err := marshalProgressJSON(map[string]string{"srcPath": "/src/a.bin"})
	if err != nil {
		t.Fatalf("marshalProgressJSON() error: %v", err)
	}
	if strings.Contains(string(encoded), `\/`) {
		t.Fatalf("path slashes were HTML-escaped: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"/src/a.bin"`) {
		t.Fatalf("payload = %s", encoded)
	}
	if strings.HasSuffix(string(encoded), "\n") {
		t.Fatalf("SSE JSON should not include Encoder newline: %q", encoded)
	}
}

func TestPrepareStreamPayloadSnapshotsWhenFileSetChanges(t *testing.T) {
	hub := &progressHub{
		subscribers: make(map[int64]map[chan []byte]struct{}),
		pending:     make(map[int64]struct{}),
		frames:      make(map[int64]progressFrame),
	}
	first := sampleCurrent(10)
	hub.prepareStreamPayload(7, &first, true)

	next := sampleCurrent(10, streamDoingItem{
		AlistTaskID: "copy-2",
		FileName:    "b.bin",
		SrcPath:     "/src",
		DstPath:     "/dst",
		FileSize:    2000,
		Type:        0,
		Status:      1,
		Progress:    1,
		CreateTime:  101,
	})
	hub.prepareStreamPayload(7, &next, false)
	if next.DoingTask == nil {
		t.Fatalf("file-set change must keep doingTask snapshot")
	}
	if next.DoingPatch != nil {
		t.Fatalf("file-set change must not emit doingPatch")
	}
}
