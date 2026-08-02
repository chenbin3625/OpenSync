package service

import (
	"encoding/json"
	"log"
	"opensync/internal/model"
	"sync"
	"time"
)

const progressNotifyDebounce = 400 * time.Millisecond

type progressHub struct {
	mu          sync.Mutex
	subscribers map[int64]map[chan []byte]struct{}
	pending     map[int64]struct{}
}

var jobProgressHub = &progressHub{
	subscribers: make(map[int64]map[chan []byte]struct{}),
	pending:     make(map[int64]struct{}),
}

func SubscribeJobProgress(jobID int64) <-chan []byte {
	ch := make(chan []byte, 1)
	jobProgressHub.subscribe(jobID, ch)
	return ch
}

func UnsubscribeJobProgress(jobID int64, ch <-chan []byte) {
	jobProgressHub.unsubscribe(jobID, ch)
}

func (h *progressHub) subscribe(jobID int64, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers[jobID] == nil {
		h.subscribers[jobID] = make(map[chan []byte]struct{})
	}
	h.subscribers[jobID][ch] = struct{}{}
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

	payload, err := marshalJobProgress(jobID)
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

func marshalJobProgress(jobID int64) ([]byte, error) {
	data := GetJobCurrent(jobID, map[string]interface{}{})
	envelope := model.Success(data)
	return json.Marshal(envelope)
}

func BuildJobProgressStreamPayload(jobID int64) ([]byte, error) {
	return marshalJobProgress(jobID)
}
