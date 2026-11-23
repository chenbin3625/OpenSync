package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProgressHubUnsubscribeDoesNotCloseChannel(t *testing.T) {
	hub := &progressHub{
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
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

func TestProgressHubUnsubscribeReleasesFrame(t *testing.T) {
	hub := &progressHub{
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
	}
	ch := make(chan []byte, 1)
	hub.subscribe(1, ch)
	current := sampleCurrent(10)
	hub.prepareStreamPayload(1, &current, true)
	if _, ok := hub.frames[1]; !ok {
		t.Fatal("prepareStreamPayload() did not retain frame")
	}

	hub.unsubscribe(1, ch)
	if _, ok := hub.frames[1]; ok {
		t.Fatal("unsubscribe() retained frame after the last subscriber left")
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

func TestJobCurrentPayloadEmitsEmptyDoingTaskSnapshot(t *testing.T) {
	payload := jobCurrentPayload{DoingTask: []streamDoingItem{}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	if !strings.Contains(string(encoded), `"doingTask":[]`) {
		t.Fatalf("empty doingTask snapshot was omitted: %s", encoded)
	}
}

func TestPrepareStreamPayloadPatchesUnchangedFileSet(t *testing.T) {
	hub := &progressHub{
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
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
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
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

func TestPrepareStreamPayloadSnapshotsWhenFileSetShrinks(t *testing.T) {
	hub := &progressHub{
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
	}
	first := sampleCurrent(10, streamDoingItem{
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
	hub.prepareStreamPayload(7, &first, true)

	next := sampleCurrent(80)
	next.Duration = 5
	hub.prepareStreamPayload(7, &next, false)
	if next.DoingTask == nil {
		t.Fatalf("file-set shrink must keep doingTask snapshot")
	}
	if next.DoingPatch != nil {
		t.Fatalf("file-set shrink must not emit doingPatch")
	}
}

func TestPrepareStreamPayloadEmitsEmptyPatchWhenProgressIsUnchanged(t *testing.T) {
	hub := &progressHub{
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
	}
	first := sampleCurrent(10)
	hub.prepareStreamPayload(7, &first, true)

	next := sampleCurrent(10)
	hub.prepareStreamPayload(7, &next, false)
	if next.DoingTask != nil {
		t.Fatalf("unchanged file set must omit doingTask")
	}
	if next.DoingPatch == nil {
		t.Fatalf("unchanged file set must emit an empty doingPatch")
	}
	if len(next.DoingPatch) != 0 {
		t.Fatalf("doingPatch len = %d, want 0", len(next.DoingPatch))
	}
}

// Issue 4: publish on a deleted/missing job must never panic and emits null to active subscribers.
func TestProgressHubPublishOnDeletedJobDoesNotPanic(t *testing.T) {
	resetJobClientsForTest()
	defer resetJobClientsForTest()

	hub := &progressHub{
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
	}
	ch := make(chan []byte, 1)
	hub.subscribe(99999, ch)

	// Must not panic even though job 99999 is not in registry or DB.
	hub.publish(99999)

	select {
	case payload := <-ch:
		if !strings.Contains(string(payload), `"data":null`) {
			t.Fatalf("payload for deleted job = %s, want null data", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive terminal null payload for deleted job")
	}
}

// Issue 12: staggered subscribers keep existing baseline intact; slow consumers receive full snapshot.
func TestProgressHubStaggeredAndSlowSubscribersGetFullSnapshots(t *testing.T) {
	resetJobClientsForTest()
	defer resetJobClientsForTest()

	jt := &JobTask{
		TaskID:     77,
		CreateTime: float64(time.Now().Unix()),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()
	item := newCopyItem(jt, nil, "/src/", "/dst/", "big.bin", int64(1000), taskItemTypeCopy)
	item.DoingKey = 1
	item.setTaskID("t-1")
	item.setProgress(taskStatusRunning, 10, nil)
	jt.DoingMu.Lock()
	jt.Doing[item.DoingKey] = item
	jt.DoingMu.Unlock()

	client := &JobClient{JobID: 77, Job: map[string]interface{}{"id": int64(77), "enable": 1, "isCron": 2}}
	client.setCurrentTask(jt)
	jobClientListMu.Lock()
	jobClientList[77] = client
	jobClientListMu.Unlock()

	// Subscriber 1 joins and receives initial full snapshot.
	sub1, init1, err := SubscribeJobProgressWithSnapshot(77)
	if err != nil {
		t.Fatalf("SubscribeJobProgressWithSnapshot(sub1) error: %v", err)
	}
	defer UnsubscribeJobProgress(77, sub1)
	if !strings.Contains(string(init1), `"doingTask"`) {
		t.Fatalf("sub1 initial = %s, want full doingTask", init1)
	}

	// Subscriber 2 joins later; must NOT clobber sub1's baseline.
	sub2, init2, err := SubscribeJobProgressWithSnapshot(77)
	if err != nil {
		t.Fatalf("SubscribeJobProgressWithSnapshot(sub2) error: %v", err)
	}
	defer UnsubscribeJobProgress(77, sub2)
	if !strings.Contains(string(init2), `"doingTask"`) {
		t.Fatalf("sub2 initial = %s, want full doingTask", init2)
	}

	// Advance progress and publish: sub1 gets patch, sub2 gets full frame to sync onto baseline.
	item.setProgress(taskStatusRunning, 30, nil)
	jobProgressHub.publish(77)

	msg1 := string(<-sub1)
	msg2 := string(<-sub2)
	if !strings.Contains(msg1, `"doingPatch"`) {
		t.Fatalf("sub1 update = %s, want doingPatch", msg1)
	}
	if !strings.Contains(msg2, `"doingTask"`) {
		t.Fatalf("sub2 first update = %s, want full doingTask", msg2)
	}

	// Now make sub1 a slow consumer: do NOT drain sub1 while two updates are published.
	item.setProgress(taskStatusRunning, 50, nil)
	jobProgressHub.publish(77) // fills sub1 buffer with patch
	item.setProgress(taskStatusRunning, 80, nil)
	jobProgressHub.publish(77) // drops previous frame and replaces with FULL snapshot!

	slowMsg := string(<-sub1)
	if !strings.Contains(slowMsg, `"doingTask"`) {
		t.Fatalf("slow consumer received = %s, want full doingTask snapshot after overflow", slowMsg)
	}
	// Drain sub2 too
	_ = <-sub2
}

// Issue 12: concurrent publish & subscribe under -race.
func TestProgressHubConcurrentPublishAndSubscribeRace(t *testing.T) {
	resetJobClientsForTest()
	defer resetJobClientsForTest()

	jt := &JobTask{
		TaskID:     88,
		CreateTime: float64(time.Now().Unix()),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()
	item := newCopyItem(jt, nil, "/src/", "/dst/", "race.bin", int64(1000), taskItemTypeCopy)
	item.DoingKey = 1
	item.setTaskID("t-race")
	jt.DoingMu.Lock()
	jt.Doing[item.DoingKey] = item
	jt.DoingMu.Unlock()

	client := &JobClient{JobID: 88, Job: map[string]interface{}{"id": int64(88), "enable": 1, "isCron": 2}}
	client.setCurrentTask(jt)
	jobClientListMu.Lock()
	jobClientList[88] = client
	jobClientListMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				ch, _, err := SubscribeJobProgressWithSnapshot(88)
				if err == nil {
					select {
					case <-ch:
					default:
					}
					UnsubscribeJobProgress(88, ch)
				}
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				item.setProgress(taskStatusRunning, float64((idx*30+j)%100), nil)
				jobProgressHub.publish(88)
			}
		}(i)
	}
	wg.Wait()
}

// Issue 13: sub-second ticks preserve previous speed and progress values are clamped to [0, 100].
func TestTransferMeterPreservesSpeedOnSubSecondTickAndClampsProgress(t *testing.T) {
	if got := clampProgress(-5); got != 0 {
		t.Fatalf("clampProgress(-5) = %v, want 0", got)
	}
	if got := clampProgress(140); got != 100 {
		t.Fatalf("clampProgress(140) = %v, want 100", got)
	}
	if got := clampProgress(math.NaN()); got != 0 {
		t.Fatalf("clampProgress(NaN) = %v, want 0", got)
	}
	if got := clampProgress(math.Inf(1)); got != 0 {
		t.Fatalf("clampProgress(+Inf) = %v, want 0", got)
	}

	item := streamDoingItem{FileSize: 1000, Progress: 150}
	if got := item.transferredSize(); got != 1000 {
		t.Fatalf("transferredSize(150%%) = %d, want 1000", got)
	}

	jt := &JobTask{
		TaskID:     5,
		CreateTime: float64(time.Now().Unix()),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()

	// Pre-seed transferMeter with a recent (< 1s) measurement; sub-second refresh must preserve speed.
	jt.CurrentMu.Lock()
	jt.lastMeter = transferMeter{
		initialized: true,
		sampleAt:    time.Now().Add(-200 * time.Millisecond),
		doneSize:    1000,
		speed:       1234.5,
	}
	jt.CurrentMu.Unlock()

	payloadSub := jt.getCurrentPayload()
	if payloadSub.Speed != 1234.5 {
		t.Fatalf("sub-second tick speed = %v, want preserved 1234.5", payloadSub.Speed)
	}

	// Now age sampleAt past 1s and add finished bytes; next refresh must recompute positive speed.
	jt.FinishMu.Lock()
	jt.FinishedSizes[taskStatusSuccess] = 3000
	jt.FinishMu.Unlock()
	jt.CurrentMu.Lock()
	jt.lastMeter.sampleAt = time.Now().Add(-2 * time.Second)
	jt.CurrentMu.Unlock()

	payloadFull := jt.getCurrentPayload()
	if payloadFull.Speed <= 0 {
		t.Fatalf("post-1s tick speed = %v, want > 0", payloadFull.Speed)
	}
}

func benchmarkProgressPayload(itemCount int) jobCurrentPayload {
	doing := make([]streamDoingItem, itemCount)
	for i := range doing {
		doing[i] = streamDoingItem{
			AlistTaskID: fmt.Sprintf("copy-%d", i+1),
			FileName:    "file.bin",
			SrcPath:     "/src",
			DstPath:     "/dst",
			FileSize:    1024,
			Type:        0,
			Status:      1,
			Progress:    float64(i % 100),
			CreateTime:  int64(i),
		}
	}
	return jobCurrentPayload{
		TaskID:     9,
		CreateTime: 100,
		Duration:   4,
		DoingTask:  doing,
	}
}

func BenchmarkPrepareStreamPayloadProgressPatch1000(b *testing.B) {
	hub := &progressHub{
		subscribers:  make(map[int64]map[chan []byte]struct{}),
		subNeedsFull: make(map[chan []byte]bool),
		pending:      make(map[int64]struct{}),
		frames:       make(map[int64]progressFrame),
	}
	first := benchmarkProgressPayload(1000)
	hub.prepareStreamPayload(7, &first, true)
	base := benchmarkProgressPayload(1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		current := base
		current.Duration++
		current.DoingTask = append([]streamDoingItem(nil), base.DoingTask...)
		current.DoingTask[i%len(current.DoingTask)].Progress++
		hub.prepareStreamPayload(7, &current, false)
	}
}
