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
			var firstWrite sync.Once
			restorePersist := replacePersistJobTaskItems(func([]map[string]interface{}) error {
				firstWrite.Do(func() {
					close(entered)
					<-release
				})
				return tc.err
			})
			defer restorePersist()
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

func TestFinishCopyItemReleasesDoingAndCopySemBeforeDBFlush(t *testing.T) {
	flushEntered := make(chan struct{})
	flushRelease := make(chan struct{})
	var once sync.Once
	restorePersist := replacePersistJobTaskItems(func([]map[string]interface{}) error {
		once.Do(func() {
			close(flushEntered)
			<-flushRelease
		})
		return nil
	})
	defer restorePersist()

	jt := &JobTask{TaskID: 99}
	jt.initRuntime()
	// Pre-fill persistBuffer to threshold-1 so finishCopyItem triggers a synchronous DB flush.
	jt.persistBufMu.Lock()
	for i := 0; i < persistBatchSize-1; i++ {
		jt.persistBuffer = append(jt.persistBuffer, JobTaskItem{TaskID: 99, FileName: "prev"})
	}
	jt.persistBufMu.Unlock()

	jt.copySem = make(chan struct{}, 1)
	jt.copySem <- struct{}{} // occupy the sole concurrency slot

	item := newCopyItem(jt, nil, "/src/", "/dst/", "cur.txt", int64(10), taskItemTypeCopy)
	item.DoingKey = 1
	item.copySlotHeld = true
	item.setProgress(taskStatusSuccess, 100, nil)
	jt.DoingMu.Lock()
	jt.Doing[item.DoingKey] = item
	jt.DoingMu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		jt.finishCopyItem(item)
	}()

	<-flushEntered

	// While DB flush is blocked, Doing must already be empty and copySem slot must be free!
	if got := jt.doingLen(); got != 0 {
		close(flushRelease)
		<-done
		t.Fatalf("doingLen() = %d while DB flush blocked, want 0", got)
	}
	select {
	case jt.copySem <- struct{}{}:
		<-jt.copySem
	default:
		close(flushRelease)
		<-done
		t.Fatal("copySem slot was still held during synchronous DB flush")
	}

	close(flushRelease)
	<-done
}
