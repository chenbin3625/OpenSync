package service

import "testing"

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
