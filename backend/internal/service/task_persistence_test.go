package service

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPersistRemainingTaskItemsWaitsForInFlightBatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"success", nil}, {"failure", errors.New("write failed")}} {
		t.Run(tc.name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			oldPersist := persistJobTaskItems
			var firstWrite sync.Once
			persistJobTaskItems = func([]map[string]interface{}) error {
				firstWrite.Do(func() {
					close(entered)
					<-release
				})
				return tc.err
			}
			defer func() { persistJobTaskItems = oldPersist }()
			jt := &JobTask{persistBuffer: []JobTaskItem{{FileName: "one"}}}
			first := make(chan error, 1)
			go func() { first <- jt.flushPersistBuffer() }()
			<-entered
			final := make(chan error, 1)
			go func() { final <- jt.persistRemainingTaskItems() }()
			select {
			case err := <-final:
				close(release)
				<-first
				t.Fatalf("final flush returned before the in-flight write: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			if err := <-first; !errors.Is(err, tc.err) {
				t.Fatalf("first flush = %v, want %v", err, tc.err)
			}
			if err := <-final; !errors.Is(err, tc.err) {
				t.Fatalf("final flush = %v, want %v", err, tc.err)
			}
		})
	}
}
