package service

import (
	"bytes"
	"encoding/json"
	"log"
	"opensync/internal/model"
	"strconv"
	"sync"
	"time"
)

const progressNotifyDebounce = 400 * time.Millisecond
const maxProgressSubscribersPerJob = 8

type progressItemState struct {
	progress float64
	status   int
}

type progressFrame struct {
	taskID     int64
	createTime int
	keys       map[string]progressItemState
}

type progressHub struct {
	mu          sync.Mutex
	subscribers map[int64]map[chan []byte]struct{}
	pending     map[int64]struct{}
	frames      map[int64]progressFrame
}

var jobProgressHub = &progressHub{
	subscribers: make(map[int64]map[chan []byte]struct{}),
	pending:     make(map[int64]struct{}),
	frames:      make(map[int64]progressFrame),
}

func SubscribeJobProgress(jobID int64) <-chan []byte {
	ch := make(chan []byte, 1)
	if !jobProgressHub.subscribe(jobID, ch) {
		return nil
	}
	return ch
}

func UnsubscribeJobProgress(jobID int64, ch <-chan []byte) {
	jobProgressHub.unsubscribe(jobID, ch)
}

func (h *progressHub) subscribe(jobID int64, ch chan []byte) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers[jobID] == nil {
		h.subscribers[jobID] = make(map[chan []byte]struct{})
	}
	if len(h.subscribers[jobID]) >= maxProgressSubscribersPerJob {
		return false
	}
	h.subscribers[jobID][ch] = struct{}{}
	return true
}

func (h *progressHub) unsubscribe(jobID int64, ch <-chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.subscribers[jobID]
	if subs == nil {
		return
	}
	for candidate := range subs {
		if candidate == ch {
			delete(subs, candidate)
			break
		}
	}
	if len(subs) == 0 {
		delete(h.subscribers, jobID)
	}
}

func (jt *JobTask) notifyProgressChange() {
	if jt == nil || jt.JobClient == nil {
		return
	}
	jobProgressHub.schedule(jt.JobClient.JobID)
}

func (jt *JobTask) notifyProgressNow() {
	if jt == nil || jt.JobClient == nil {
		return
	}
	jobProgressHub.publish(jt.JobClient.JobID)
}

func (h *progressHub) schedule(jobID int64) {
	h.mu.Lock()
	if len(h.subscribers[jobID]) == 0 {
		h.mu.Unlock()
		return
	}
	if _, pending := h.pending[jobID]; pending {
		h.mu.Unlock()
		return
	}
	h.pending[jobID] = struct{}{}
	h.mu.Unlock()

	time.AfterFunc(progressNotifyDebounce, func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in progressHub.schedule publish for job %d: %v", jobID, r)
			}
		}()

		h.mu.Lock()
		delete(h.pending, jobID)
		h.mu.Unlock()

		h.publish(jobID)
	})
}

func (h *progressHub) publish(jobID int64) {
	subs := h.subscriberSnapshot(jobID)
	if len(subs) == 0 {
		return
	}

	payload, err := marshalJobProgress(jobID, false)
	if err != nil {
		log.Printf("Failed to marshal job %d progress stream payload: %v", jobID, err)
		return
	}
	for _, ch := range subs {
		sendProgressPayload(ch, payload)
	}
}

func (h *progressHub) subscriberSnapshot(jobID int64) []chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	subs := h.subscribers[jobID]
	if len(subs) == 0 {
		return nil
	}
	channels := make([]chan []byte, 0, len(subs))
	for ch := range subs {
		channels = append(channels, ch)
	}
	return channels
}

func sendProgressPayload(ch chan []byte, payload []byte) {
	select {
	case ch <- payload:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- payload:
		default:
		}
	}
}

func marshalJobProgress(jobID int64, snapshot bool) ([]byte, error) {
	data := GetJobCurrent(jobID, map[string]interface{}{})
	if data == nil {
		jobProgressHub.clearFrame(jobID)
		return marshalProgressJSON(model.Success(nil))
	}
	current, ok := data.(jobCurrentPayload)
	if !ok {
		return marshalProgressJSON(model.Success(data))
	}
	jobProgressHub.prepareStreamPayload(jobID, &current, snapshot)
	return marshalProgressJSON(model.Success(current))
}

var progressJSONPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

func marshalProgressJSON(v any) ([]byte, error) {
	buf := progressJSONPool.Get().(*bytes.Buffer)
	buf.Reset()
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		progressJSONPool.Put(buf)
		return nil, err
	}
	data := buf.Bytes()
	if n := len(data); n > 0 && data[n-1] == '\n' {
		data = data[:n-1]
	}
	out := make([]byte, len(data))
	copy(out, data)
	progressJSONPool.Put(buf)
	return out, nil
}

func BuildJobProgressStreamPayload(jobID int64) ([]byte, error) {
	return marshalJobProgress(jobID, true)
}

func (item streamDoingItem) streamKey() string {
	if item.ID != 0 {
		return "id:" + strconv.FormatInt(item.ID, 10)
	}
	if item.AlistTaskID != "" {
		return "alist:" + item.AlistTaskID
	}
	return "path:" + item.FileName + "|" + item.SrcPath + "|" + item.DstPath
}

func (item streamDoingItem) streamState() progressItemState {
	return progressItemState{
		progress: item.Progress,
		status:   item.Status,
	}
}

func (item streamDoingItem) toPatch(includePaths bool) streamDoingPatch {
	patch := streamDoingPatch{
		ID:          item.ID,
		AlistTaskID: item.AlistTaskID,
		Status:      item.Status,
		Progress:    item.Progress,
	}
	if includePaths {
		patch.FileName = item.FileName
		patch.SrcPath = item.SrcPath
		patch.DstPath = item.DstPath
	}
	return patch
}

func (h *progressHub) prepareStreamPayload(jobID int64, payload *jobCurrentPayload, snapshot bool) {
	doing := payload.DoingTask
	next := progressFrame{
		taskID:     payload.TaskID,
		createTime: payload.CreateTime,
		keys:       make(map[string]progressItemState, len(doing)),
	}
	for _, item := range doing {
		next.keys[item.streamKey()] = item.streamState()
	}

	h.mu.Lock()
	if h.frames == nil {
		h.frames = make(map[int64]progressFrame)
	}
	prev, hasPrev := h.frames[jobID]
	h.frames[jobID] = next
	h.mu.Unlock()

	if snapshot || !hasPrev || prev.taskID != next.taskID || prev.createTime != next.createTime {
		return
	}
	if !sameProgressKeySet(prev.keys, next.keys) {
		return
	}

	patch := make([]streamDoingPatch, 0, len(doing))
	for _, item := range doing {
		key := item.streamKey()
		state := next.keys[key]
		if prev.keys[key] == state {
			continue
		}
		patch = append(patch, item.toPatch(item.ID == 0 && item.AlistTaskID == ""))
	}
	payload.DoingTask = nil
	payload.DoingPatch = patch
}

func sameProgressKeySet(left, right map[string]progressItemState) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func (h *progressHub) clearFrame(jobID int64) {
	h.mu.Lock()
	delete(h.frames, jobID)
	h.mu.Unlock()
}
