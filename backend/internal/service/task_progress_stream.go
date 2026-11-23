package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"opensync/internal/model"
	"sync"
	"time"
)

const progressNotifyDebounce = 400 * time.Millisecond
const maxProgressSubscribersPerJob = 8

// ErrTooManyProgressStreams is returned when a job already has the maximum
// allowed concurrent SSE progress subscribers.
var ErrTooManyProgressStreams = errors.New("too many progress streams")

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
	mu           sync.Mutex
	framesMu     sync.Mutex
	publishLocks [64]sync.Mutex
	subscribers  map[int64]map[chan []byte]struct{}
	subNeedsFull map[chan []byte]bool
	pending      map[int64]struct{}
	frames       map[int64]progressFrame
}

var jobProgressHub = &progressHub{
	subscribers:  make(map[int64]map[chan []byte]struct{}),
	subNeedsFull: make(map[chan []byte]bool),
	pending:      make(map[int64]struct{}),
	frames:       make(map[int64]progressFrame),
}

func (h *progressHub) jobPublishLock(jobID int64) *sync.Mutex {
	idx := uint64(jobID) % uint64(len(h.publishLocks))
	return &h.publishLocks[idx]
}

func SubscribeJobProgress(jobID int64) <-chan []byte {
	ch := make(chan []byte, 1)
	if !jobProgressHub.subscribe(jobID, ch) {
		return nil
	}
	return ch
}

// SubscribeJobProgressWithSnapshot validates that the job exists, subscribes to
// its progress stream, and builds the initial full snapshot atomically under
// the job's publish lock so no terminal "null" event can slip between snapshot
// capture and subscription registration.
func SubscribeJobProgressWithSnapshot(jobID int64) (<-chan []byte, []byte, error) {
	// Validate the job exists before acquiring the publish lock so an unknown
	// job still surfaces the normal public JobNotFound error.
	_ = GetJobClientByID(jobID)

	publishMu := jobProgressHub.jobPublishLock(jobID)
	publishMu.Lock()
	defer publishMu.Unlock()

	ch := make(chan []byte, 1)
	ok, hadOthers := jobProgressHub.subscribeTracked(jobID, ch)
	if !ok {
		return nil, nil, ErrTooManyProgressStreams
	}

	// If other subscribers already exist, build a standalone full snapshot
	// without overwriting their shared diff baseline, and mark this new
	// subscriber to receive a full frame on the next publish so it syncs onto
	// the shared baseline cleanly.
	payload, err := jobProgressHub.buildInitialSnapshotLocked(jobID, !hadOthers)
	if err != nil {
		jobProgressHub.unsubscribe(jobID, ch)
		return nil, nil, err
	}
	return ch, payload, nil
}

func UnsubscribeJobProgress(jobID int64, ch <-chan []byte) {
	jobProgressHub.unsubscribe(jobID, ch)
}

func (h *progressHub) subscribe(jobID int64, ch chan []byte) bool {
	ok, _ := h.subscribeTracked(jobID, ch)
	return ok
}

func (h *progressHub) subscribeTracked(jobID int64, ch chan []byte) (bool, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers == nil {
		h.subscribers = make(map[int64]map[chan []byte]struct{})
	}
	if h.subNeedsFull == nil {
		h.subNeedsFull = make(map[chan []byte]bool)
	}
	if h.subscribers[jobID] == nil {
		h.subscribers[jobID] = make(map[chan []byte]struct{})
	}
	existing := len(h.subscribers[jobID])
	if existing >= maxProgressSubscribersPerJob {
		return false, false
	}
	h.subscribers[jobID][ch] = struct{}{}
	hadOthers := existing > 0
	if hadOthers {
		h.subNeedsFull[ch] = true
	}
	return true, hadOthers
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
			delete(h.subNeedsFull, candidate)
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
		h.clearFrame(jobID)
	}
}

func (jt *JobTask) notifyProgressChange() {
	if jt == nil || jt.JobClient == nil {
		return
	}
	jobProgressHub.schedule(jt.JobClient.idSnapshot())
}

func (jt *JobTask) notifyProgressNow() {
	if jt == nil || jt.JobClient == nil {
		return
	}
	jobProgressHub.publish(jt.JobClient.idSnapshot())
}

func (h *progressHub) schedule(jobID int64) {
	h.mu.Lock()
	if len(h.subscribers[jobID]) == 0 {
		h.mu.Unlock()
		return
	}
	if h.pending == nil {
		h.pending = make(map[int64]struct{})
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
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in progressHub.publish for job %d: %v", jobID, r)
		}
	}()

	publishMu := h.jobPublishLock(jobID)
	publishMu.Lock()
	defer publishMu.Unlock()

	subs := h.subscriberSnapshot(jobID)
	if len(subs) == 0 {
		return
	}

	payload, ok := currentJobProgressPayloadSafe(jobID)
	if !ok {
		h.clearFrame(jobID)
		nullBytes, err := marshalProgressJSON(model.Success(nil))
		if err != nil {
			log.Printf("Failed to marshal idle job %d progress stream payload: %v", jobID, err)
			return
		}
		h.broadcastLocked(subs, nullBytes, func() ([]byte, error) { return nullBytes, nil }, false)
		return
	}

	fullCopy := payload
	h.prepareStreamPayload(jobID, &payload, false)
	isPatch := payload.DoingTask == nil
	frameBytes, err := marshalProgressJSON(model.Success(payload))
	if err != nil {
		log.Printf("Failed to marshal job %d progress stream payload: %v", jobID, err)
		return
	}

	var fullBytes []byte
	var fullOnce sync.Once
	var fullErr error
	getFullBytes := func() ([]byte, error) {
		if !isPatch {
			return frameBytes, nil
		}
		fullOnce.Do(func() {
			fullBytes, fullErr = marshalProgressJSON(model.Success(fullCopy))
		})
		return fullBytes, fullErr
	}

	h.broadcastLocked(subs, frameBytes, getFullBytes, isPatch)
}

func (h *progressHub) broadcastLocked(subs []chan []byte, frameBytes []byte, getFullBytes func() ([]byte, error), isPatch bool) {
	for _, ch := range subs {
		needsFull := h.takeNeedsFull(ch)
		if needsFull && isPatch {
			fullBytes, err := getFullBytes()
			if err != nil {
				log.Printf("Failed to marshal full progress snapshot: %v", err)
				continue
			}
			sendProgressPayload(ch, fullBytes)
			continue
		}

		// Fast path: subscriber drained its buffer and is synchronized with the
		// shared frame baseline.
		select {
		case ch <- frameBytes:
			continue
		default:
		}

		// Slow consumer: an older frame was still sitting in ch. Evicting it and
		// pushing a delta patch would leave the client with an orphaned patch, so
		// replace the evicted frame with a full snapshot instead.
		payloadToSend := frameBytes
		if isPatch {
			fullBytes, err := getFullBytes()
			if err == nil {
				payloadToSend = fullBytes
			}
		}
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- payloadToSend:
		default:
			if isPatch {
				h.markNeedsFull(ch)
			}
		}
	}
}

func (h *progressHub) takeNeedsFull(ch chan []byte) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subNeedsFull == nil || !h.subNeedsFull[ch] {
		return false
	}
	delete(h.subNeedsFull, ch)
	return true
}

func (h *progressHub) markNeedsFull(ch chan []byte) {
	h.mu.Lock()
	if h.subNeedsFull == nil {
		h.subNeedsFull = make(map[chan []byte]bool)
	}
	h.subNeedsFull[ch] = true
	h.mu.Unlock()
}

func (h *progressHub) buildInitialSnapshotLocked(jobID int64, seedFrame bool) ([]byte, error) {
	payload, ok := currentJobProgressPayloadSafe(jobID)
	if !ok {
		if seedFrame {
			h.clearFrame(jobID)
		}
		return marshalProgressJSON(model.Success(nil))
	}
	if seedFrame {
		h.prepareStreamPayload(jobID, &payload, true)
	}
	return marshalProgressJSON(model.Success(payload))
}

// currentJobProgressPayloadSafe retrieves the active task's current progress
// snapshot without hitting the DB or panicking if the job was deleted.
func currentJobProgressPayloadSafe(jobID int64) (jobCurrentPayload, bool) {
	client, ok := getJobClientIfExists(jobID)
	if !ok {
		return jobCurrentPayload{}, false
	}
	task := client.currentTask()
	if task == nil || task.finished.Load() {
		return jobCurrentPayload{}, false
	}
	return task.getCurrentPayload(), true
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
	publishMu := jobProgressHub.jobPublishLock(jobID)
	publishMu.Lock()
	defer publishMu.Unlock()
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
