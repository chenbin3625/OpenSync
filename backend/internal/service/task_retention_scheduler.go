package service

import (
	"log"
	"opensync/internal/config"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

const taskRetentionCleanupCronSpec = "0 0 3 * * *"

var (
	taskRetentionSchedulerMu sync.Mutex
	taskRetentionCron        *cron.Cron
	taskRetentionRunning     atomic.Bool
	runRetentionCleanupFn    = CleanupExpiredTasks
)

// RunTaskRetentionCleanup deletes task history older than the configured retention window.
func RunTaskRetentionCleanup() {
	if !taskRetentionRunning.CompareAndSwap(false, true) {
		log.Printf("Skipping task retention cleanup because a previous run is still active")
		return
	}
	defer taskRetentionRunning.Store(false)
	runRetentionCleanupFn(log.Default(), config.GetConfig().Server.TaskSave, time.Now())
}

// StartTaskRetentionScheduler runs cleanup daily at 03:00 in the scheduler timezone.
// The returned stop function shuts down the scheduler and is safe to call multiple times.
func StartTaskRetentionScheduler() func() {
	taskRetentionSchedulerMu.Lock()
	defer taskRetentionSchedulerMu.Unlock()

	if taskRetentionCron != nil {
		return stopTaskRetentionScheduler
	}

	loc := schedulerLocation()
	c := cron.New(cron.WithSeconds(), cron.WithLocation(loc))
	if _, err := c.AddFunc(taskRetentionCleanupCronSpec, RunTaskRetentionCleanup); err != nil {
		log.Printf("Failed to schedule task retention cleanup: %v", err)
		return func() {}
	}
	c.Start()
	taskRetentionCron = c
	log.Printf("Task history cleanup scheduled daily at 03:00 (%s)", loc.String())

	return stopTaskRetentionScheduler
}

func stopTaskRetentionScheduler() {
	taskRetentionSchedulerMu.Lock()
	c := taskRetentionCron
	taskRetentionCron = nil
	taskRetentionSchedulerMu.Unlock()

	if c == nil {
		return
	}
	ctx := c.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(schedulerStopTimeout):
		log.Printf("task retention scheduler stop timed out after %s", schedulerStopTimeout)
	}
}
