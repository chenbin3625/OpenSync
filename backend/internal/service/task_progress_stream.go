package service

import (
	"bytes"
	"encoding/json"
	"log"
	"opensync/internal/model"
	"sync"
	"time"
)

const progressNotifyDebounce = 400 * time.Millisecond
const maxProgressSubscribersPerJob = 8

type progressItemState struct {
	progress   float64
	status     int
	generation uint64
}

// progressItemKey is deliberately a comparable value instead of a formatted
// string. Progress frames are rebuilt on every debounced SSE update; building
// "id:"/"alist:"/"path:" strings for every active item creates avoidable
// temporary allocations and CPU work on the hottest realtime path.
type progressItemKey struct {
	kind                       uint8
	id                         int64
	text                       string
	fileName, srcPath, dstPath string
}

type progressFrame struct {
	taskID     int64
	createTime int
	generation uint64
	keys       map[progressItemKey]progressItemState
}

type progressHub struct {
	mu          sync.Mutex
	framesMu    sync.Mutex
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
	clearFrame := false
	h.mu.Lock()
	subs := h.subscribers[jobID]
	if subs == nil {
		h.mu.Unlock()
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
		clearFrame = true
	}
	h.mu.Unlock()
	// A disconnected viewer no longer needs the last progress diff frame.
	// Release it immediately so a job with many completed/aborted viewers does
	// not retain all active-item keys until the next task snapshot.
	if clearFrame {
		h.framesMu.Lock()
		delete(h.frames, jobID)
		h.framesMu.Unlock()
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

func (item streamDoingItem) streamKey() progressItemKey {
	if item.ID != 0 {
		return progressItemKey{kind: 1, id: item.ID}
	}
	if item.AlistTaskID != "" {
		return progressItemKey{kind: 2, text: item.AlistTaskID}
	}
	return progressItemKey{
		kind: 3,
		// Path-keyed items are only used before AList returns a remote task id.
		// Keep the common ID forms allocation-free and retain the fallback as
		// separate comparable fields so it also avoids composite-string builds.
		fileName: item.FileName,
		srcPath:  item.SrcPath,
		dstPath:  item.DstPath,
	}
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
	patch := make([]streamDoingPatch, 0, len(doing))

	h.framesMu.Lock()
	if h.frames == nil {
		h.frames = make(map[int64]progressFrame)
	}
	frame, hasPrev := h.frames[jobID]
	previousTaskID := frame.taskID
	previousCreateTime := frame.createTime
	sameTask := hasPrev && previousTaskID == payload.TaskID && previousCreateTime == payload.CreateTime
	if frame.keys == nil {
		frame.keys = make(map[progressItemKey]progressItemState, len(doing))
	}
	frame.taskID = payload.TaskID
	frame.createTime = payload.CreateTime
	frame.generation++
	if frame.generation == 0 {
		// A running task will never realistically reach uint64 wraparound, but
		// resetting the map keeps the generation invariant explicit.
		clear(frame.keys)
		frame.generation = 1
	}
	generation := frame.generation
	fileSetChanged := !sameTask
	for _, item := range doing {
		key := item.streamKey()
		state := item.streamState()
		previous, exists := frame.keys[key]
		if sameTask {
			if !exists {
				fileSetChanged = true
			} else if previous.progress != state.progress || previous.status != state.status {
				patch = append(patch, item.toPatch(item.ID == 0 && item.AlistTaskID == ""))
			}
		}
		state.generation = generation
		frame.keys[key] = state
	}
	for key, state := range frame.keys {
		if state.generation != generation {
			delete(frame.keys, key)
			if sameTask {
				fileSetChanged = true
			}
		}
	}
	h.frames[jobID] = frame
	h.framesMu.Unlock()

	if snapshot || !hasPrev || !sameTask || fileSetChanged {
		return
	}
	payload.DoingTask = nil
	payload.DoingPatch = patch
}

func (h *progressHub) clearFrame(jobID int64) {
	h.framesMu.Lock()
	delete(h.frames, jobID)
	h.framesMu.Unlock()
}
