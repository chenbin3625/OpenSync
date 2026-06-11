package service

import (
	"opensync/internal/i18n"
	"opensync/pkg/util"
	"testing"
	"time"
)

func TestJobClientDoingStateIsSerialized(t *testing.T) {
	client := &JobClient{}

	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() on idle client = false, want true")
	}
	if client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() on running client = true, want false")
	}
	if !client.isDoing() {
		t.Fatalf("isDoing() = false, want true")
	}

	client.markDone()
	if client.isDoing() {
		t.Fatalf("isDoing() after markDone = true, want false")
	}
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() after markDone = false, want true")
	}
}

func TestJobClientCurrentTaskIsProtected(t *testing.T) {
	client := &JobClient{}
	task := &JobTask{TaskID: 99}

	client.setCurrentTask(task)
	if got := client.currentTask(); got != task {
		t.Fatalf("currentTask() = %#v, want %#v", got, task)
	}

	client.clearCurrentTask(task)
	if got := client.currentTask(); got != nil {
		t.Fatalf("currentTask() after clear = %#v, want nil", got)
	}
}

func TestStopJobKeepsClientBusyUntilTaskFinishes(t *testing.T) {
	client := &JobClient{
		Job:       map[string]interface{}{"enable": 1, "isCron": 2},
		Scheduler: NewScheduler(),
	}
	defer client.Scheduler.Stop()

	task := &JobTask{}
	task.initRuntime()
	client.setCurrentTask(task)
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}

	client.StopJob(true)

	if !task.isBreak() {
		t.Fatalf("StopJob() did not request task break")
	}
	if !client.isDoing() {
		t.Fatalf("StopJob() marked client idle before task cleanup finished")
	}
}

func TestDoScheduledSkipsWhenJobAlreadyRunning(t *testing.T) {
	client := &JobClient{
		Job: map[string]interface{}{"enable": 1, "isCron": 2},
	}
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}

	if client.DoScheduled() {
		t.Fatalf("DoScheduled() = true while job is running, want false")
	}
	if !client.isDoing() {
		t.Fatalf("DoScheduled() changed running state, want still running")
	}
}

func TestWaitUntilIdleWaitsForTaskCleanup(t *testing.T) {
	client := &JobClient{}
	task := &JobTask{TaskID: 99}
	client.setCurrentTask(task)
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		client.markDone()
		client.clearCurrentTask(task)
	}()

	if !client.waitUntilIdle(time.Second) {
		t.Fatalf("waitUntilIdle() = false, want true after task cleanup")
	}
}

func TestWaitUntilIdleTimesOutWhileTaskStillRunning(t *testing.T) {
	client := &JobClient{}
	task := &JobTask{TaskID: 99}
	client.setCurrentTask(task)
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}
	defer func() {
		client.markDone()
		client.clearCurrentTask(task)
	}()

	if client.waitUntilIdle(20 * time.Millisecond) {
		t.Fatalf("waitUntilIdle() = true, want false while task is still running")
	}
}

func TestRemoveJobClientRejectsRunningJobWithoutStoppingIt(t *testing.T) {
	client := &JobClient{
		JobID:     99,
		Job:       map[string]interface{}{"id": int64(99), "enable": 1, "isCron": 2},
		Scheduler: NewScheduler(),
	}
	defer client.Scheduler.Stop()

	task := &JobTask{TaskID: 100}
	task.initRuntime()
	client.setCurrentTask(task)
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}

	jobClientListMu.Lock()
	previousClients := jobClientList
	jobClientList = map[int64]*JobClient{client.JobID: client}
	jobClientListMu.Unlock()
	defer func() {
		jobClientListMu.Lock()
		jobClientList = previousClients
		jobClientListMu.Unlock()
	}()

	panicCh := make(chan interface{}, 1)
	go func() {
		defer func() {
			panicCh <- recover()
		}()
		RemoveJobClient(client.JobID)
	}()

	select {
	case recovered := <-panicCh:
		err, ok := recovered.(interface{ Error() string })
		if !ok || err.Error() != i18n.G("job_running_cannot_delete") {
			t.Fatalf("RemoveJobClient() panic = %#v, want %q", recovered, i18n.G("job_running_cannot_delete"))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("RemoveJobClient() did not reject running job immediately")
	}

	if task.isBreak() {
		t.Fatalf("RemoveJobClient() requested task break while rejecting delete")
	}
	if got := util.ToInt(client.Job["enable"]); got != 1 {
		t.Fatalf("job enable after rejected delete = %d, want 1", got)
	}
}
