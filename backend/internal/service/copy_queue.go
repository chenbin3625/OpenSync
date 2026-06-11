package service

import (
	"context"
	"sync"
)

type copyQueue struct {
	mu       sync.Mutex
	items    []*CopyItem
	head     int
	closed   bool
	capacity int
	notify   chan struct{}
	space    chan struct{}
}

func newCopyQueue() *copyQueue {
	return newCopyQueueWithCapacity(maxQueuedCopyItems)
}

func newCopyQueueWithCapacity(capacity int) *copyQueue {
	return &copyQueue{
		items:    make([]*CopyItem, 0),
		capacity: capacity,
		notify:   make(chan struct{}, 1),
		space:    make(chan struct{}, 1),
	}
}

func (q *copyQueue) push(item *CopyItem) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed || !q.hasCapacityLocked() {
		return false
	}
	q.items = append(q.items, item)
	q.signal()
	return true
}

func (q *copyQueue) pushWait(ctx context.Context, item *CopyItem) bool {
	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return false
		}
		if q.hasCapacityLocked() {
			q.items = append(q.items, item)
			q.signal()
			q.mu.Unlock()
			return true
		}
		space := q.space
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return false
		case <-space:
		}
	}
}

func (q *copyQueue) pop() (*CopyItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.head >= len(q.items) {
		q.compactLocked()
		return nil, false
	}

	item := q.items[q.head]
	q.items[q.head] = nil
	q.head++
	q.compactLocked()
	q.signalSpace()
	return item, true
}

func (q *copyQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) - q.head
}

func (q *copyQueue) snapshot() []*CopyItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]*CopyItem(nil), q.items[q.head:]...)
}

func (q *copyQueue) closeAndDrain() []*CopyItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	items := append([]*CopyItem(nil), q.items[q.head:]...)
	for i := q.head; i < len(q.items); i++ {
		q.items[i] = nil
	}
	q.items = q.items[:0]
	q.head = 0
	q.signal()
	q.signalSpace()
	return items
}

func (q *copyQueue) waitCh() <-chan struct{} {
	return q.notify
}

func (q *copyQueue) compactLocked() {
	if q.head == 0 {
		return
	}
	if q.head == len(q.items) {
		q.items = q.items[:0]
		q.head = 0
		return
	}
	if q.head >= 1024 || q.head > len(q.items)/4 {
		q.items = append([]*CopyItem(nil), q.items[q.head:]...)
		q.head = 0
	}
}

func (q *copyQueue) hasCapacityLocked() bool {
	return q.capacity <= 0 || len(q.items)-q.head < q.capacity
}

func (q *copyQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *copyQueue) signalSpace() {
	select {
	case q.space <- struct{}{}:
	default:
	}
}
