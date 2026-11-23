package service

import (
	"log"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestTaskRetentionCleanupCronSpecRunsDailyAtThreeAM(t *testing.T) {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(taskRetentionCleanupCronSpec)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", taskRetentionCleanupCronSpec, err)
	}

	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	start := time.Date(2026, 7, 9, 10, 0, 0, 0, loc)
	next := schedule.Next(start)
	want := time.Date(2026, 7, 10, 3, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestStartTaskRetentionSchedulerStopIsIdempotent(t *testing.T) {
	stop := StartTaskRetentionScheduler()
	stop()
	stop()
}

func TestStartTaskRetentionSchedulerReturnsWorkingStopWhenAlreadyStarted(t *testing.T) {
	stopFirst := StartTaskRetentionScheduler()
	stopSecond := StartTaskRetentionScheduler()
	stopFirst()
	stopSecond()
}

func TestRunTaskRetentionCleanupSkipsOverlappingRun(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	oldFn := runRetentionCleanupFn
	runRetentionCleanupFn = func(*log.Logger, int, time.Time) {
		calls.Add(1)
		close(entered)
		<-release
	}
	defer func() { runRetentionCleanupFn = oldFn }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		RunTaskRetentionCleanup()
	}()

	<-entered
	// Concurrent second call must immediately skip via atomic re-entry guard.
	RunTaskRetentionCleanup()

	close(release)
	<-done

	if got := calls.Load(); got != 1 {
		t.Fatalf("overlapping cleanup calls = %d, want 1", got)
	}
}
