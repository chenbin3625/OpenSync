package service

import "testing"

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
