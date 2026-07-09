package service

import (
	"context"
	"fmt"
	"log"
	"opensync/internal/msg"
	"opensync/internal/mapper"
	"opensync/pkg/util"
	"sync"
	"time"
)

// JobClient manages a single job's lifecycle
type JobClient struct {
	JobID          int64
	Job            map[string]interface{}
	Scheduler      *Scheduler
	JobDoing       bool
	CurrentJobTask *JobTask
	mu             sync.Mutex
	stateCh        chan struct{}
}

func (jc *JobClient) tryMarkDoing() bool {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	if jc.JobDoing {
		return false
	}
	jc.JobDoing = true
	jc.signalStateChangeLocked()
	return true
}

func (jc *JobClient) isDoing() bool {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.JobDoing
}

func (jc *JobClient) isBusy() bool {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.JobDoing || jc.CurrentJobTask != nil
}

func (jc *JobClient) markDone() {
	jc.mu.Lock()
	jc.JobDoing = false
	jc.signalStateChangeLocked()
	jc.mu.Unlock()
}

func (jc *JobClient) setCurrentTask(task *JobTask) {
	jc.mu.Lock()
	jc.CurrentJobTask = task
	jc.signalStateChangeLocked()
	jc.mu.Unlock()
}

func (jc *JobClient) currentTask() *JobTask {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.CurrentJobTask
}

func (jc *JobClient) clearCurrentTask(task *JobTask) {
	jc.mu.Lock()
	if task == nil || jc.CurrentJobTask == task {
		jc.CurrentJobTask = nil
		jc.signalStateChangeLocked()
	}
	jc.mu.Unlock()
}

func (jc *JobClient) waitUntilIdle(timeout time.Duration) bool {
	if timeout <= 0 {
		jc.mu.Lock()
		idle := jc.isIdleLocked()
		jc.mu.Unlock()
		return idle
	}
	return jc.waitUntilIdleContext(context.Background(), timeout)
}

func (jc *JobClient) waitUntilIdleContext(ctx context.Context, timeout time.Duration) bool {
	var deadline <-chan time.Time
	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}

	for {
		jc.mu.Lock()
		if jc.isIdleLocked() {
			jc.mu.Unlock()
			return true
		}
		stateCh := jc.stateChangeChLocked()
		jc.mu.Unlock()

		select {
		case <-ctx.Done():
			jc.mu.Lock()
			idle := jc.isIdleLocked()
			jc.mu.Unlock()
			return idle
		case <-deadline:
			jc.mu.Lock()
			idle := jc.isIdleLocked()
			jc.mu.Unlock()
			return idle
		case <-stateCh:
		}
	}
}

func (jc *JobClient) isIdleLocked() bool {
	return !jc.JobDoing && jc.CurrentJobTask == nil
}

func (jc *JobClient) stateChangeChLocked() chan struct{} {
	if jc.stateCh == nil {
		jc.stateCh = make(chan struct{})
	}
	return jc.stateCh
}

func (jc *JobClient) signalStateChangeLocked() {
	if jc.stateCh == nil {
		jc.stateCh = make(chan struct{})
		return
	}
	close(jc.stateCh)
	jc.stateCh = make(chan struct{})
}

func (jc *JobClient) setEnable(enable int) {
	jc.mu.Lock()
	if jc.Job != nil {
		jc.Job["enable"] = enable
	}
	jc.mu.Unlock()
}

func (jc *JobClient) enabled() bool {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.Job != nil && util.ToInt(jc.Job["enable"]) == 1
}

func (jc *JobClient) jobSnapshot() map[string]interface{} {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return cloneJobConfig(jc.Job)
}

func (jc *JobClient) idSnapshot() int64 {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.JobID
}

func (jc *JobClient) configSnapshot() (int64, map[string]interface{}, *Scheduler) {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.JobID, cloneJobConfig(jc.Job), jc.Scheduler
}

func (jc *JobClient) schedulerSnapshot() *Scheduler {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.Scheduler
}

func (jc *JobClient) replaceJobConfig(job map[string]interface{}, scheduler *Scheduler) *Scheduler {
	jc.mu.Lock()
	oldScheduler := jc.Scheduler
	jc.JobID = util.ToInt64(job["id"])
	jc.Job = cloneJobConfig(job)
	jc.Scheduler = scheduler
	jc.mu.Unlock()
	return oldScheduler
}

func cloneJobConfig(job map[string]interface{}) map[string]interface{} {
	if job == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(job))
	for key, value := range job {
		cloned[key] = value
	}
	return cloned
}

func cloneTaskRows(rows []map[string]interface{}) []map[string]interface{} {
	cloned := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]interface{}, len(row))
		for key, value := range row {
			item[key] = value
		}
		cloned = append(cloned, item)
	}
	return cloned
}

func jobTaskHasStatus(taskID int64, statuses ...taskStatus) bool {
	task, err := mapper.GetJobTaskByID(taskID)
	if err != nil {
		return false
	}
	current := taskStatusFromValue(task["status"])
	for _, status := range statuses {
		if current == status {
			return true
		}
	}
	return false
}

// NewJobClient creates a new job client
func NewJobClient(job map[string]interface{}, isInit bool) *JobClient {
	jc := &JobClient{
		Job: cloneJobConfig(job),
	}

	addJobID := int64(0)
	if _, ok := job["enable"]; !ok {
		job["enable"] = 1
	}
	if _, ok := job["method"]; !ok {
		job["method"] = 0
	}

	if _, ok := job["id"]; !ok {
		id, err := mapper.AddJob(job)
		if err != nil {
			panic(err.Error())
		}
		addJobID = id
		var err2 error
		job, err2 = mapper.GetJobByID(id)
		if err2 != nil {
			panic(err2.Error())
		}
	}

	jc.JobID = util.ToInt64(job["id"])
	jc.Job = cloneJobConfig(job)

	sched := NewScheduler()
	jc.Scheduler = sched

	err := sched.AddJob(util.ToInt(job["isCron"]), job, func() {
		jc.DoScheduled()
	})
	if err != nil {
		sched.Stop()
		if addJobID != 0 {
			log.Printf("Error during job setup, deleting job: %v", job)
			mapper.DeleteJob(jc.JobID)
		} else if isInit {
			log.Printf("Error during job setup, disabling job %d: %v", jc.JobID, err)
			if updateErr := mapper.UpdateJobEnable(jc.JobID, 0); updateErr != nil {
				log.Printf("Failed to disable invalid job %d: %v", jc.JobID, updateErr)
			}
			jc.setEnable(0)
		}
		panicPublic(err.Error())
	}

	return jc
}

func (jc *JobClient) runMarkedJob() {
	jc.runMarkedJobConfig(0, nil)
}

// runMarkedJobConfig starts a task. With sourceTaskID > 0 it replays the source
// task's items whose status is in statuses (retry-failed); otherwise it runs a
// fresh full scan.
func (jc *JobClient) runMarkedJobConfig(sourceTaskID int64, statuses []taskStatus) {
	taskID := int64(0)
	defer func() {
		if r := recover(); r != nil {
			jc.markDone()
			jc.clearCurrentTask(nil)
			errMsg := fmt.Sprintf("%v", r)
			log.Printf("Job execution error: %s", errMsg)
			if taskID > 0 && !jobTaskHasStatus(taskID, taskStatusStopped) {
				if err := UpdateJobTaskStatusSimple(taskID, taskStatusSystemFailed, &errMsg); err != nil {
					log.Printf("Failed to mark task %d as failed after execution error: %v", taskID, err)
				}
			}
		}
	}()

	if !jc.enabled() {
		return
	}

	var err error
	taskID, err = mapper.AddJobTask(jc.idSnapshot(), time.Now().Unix())
	if err != nil {
		log.Printf("Failed to create task for job %d: %v", jc.JobID, err)
		panic(err.Error())
	}
	if !jc.enabled() {
		if err := UpdateJobTaskStatusSimple(taskID, taskStatusStopped, nil); err != nil {
			log.Printf("Failed to mark disabled task %d as stopped: %v", taskID, err)
		}
		return
	}
	task := newJobTask(taskID, jc)
	if sourceTaskID > 0 {
		task.RetrySourceTaskID = sourceTaskID
		task.RetryStatuses = append([]taskStatus(nil), statuses...)
	}
	jc.setCurrentTask(task)
	task.Start()
}

// DoJob executes the job, waiting until any current run has finished.
func (jc *JobClient) DoJob() {
	for !jc.tryMarkDoing() {
		if !jc.enabled() {
			return
		}
		jc.waitUntilIdle(10 * time.Second)
	}
	jc.runMarkedJob()
}

// DoScheduled executes a scheduled job once, skipping if the previous run is still active.
func (jc *JobClient) DoScheduled() bool {
	if !jc.tryMarkDoing() {
		log.Printf("Skipping job %d because previous run is still active", jc.JobID)
		return false
	}
	jc.runMarkedJob()
	return true
}

// DoManual triggers manual execution
func (jc *JobClient) DoManual() {
	if !jc.tryMarkDoing() {
		panicPublic(msg.JobRunning)
	}
	go jc.runMarkedJob()
}

// DoRetryFailedTaskItems triggers a manual execution that replays the non-success
// items of a historical task.
func (jc *JobClient) DoRetryFailedTaskItems(sourceTaskID int64) {
	if !jc.tryMarkDoing() {
		panicPublic(msg.JobRunning)
	}
	go jc.runMarkedJobConfig(sourceTaskID, retryableTaskStatuses)
}

// ResumeJob enables and resumes the job
func (jc *JobClient) ResumeJob() {
	jobID, job, scheduler := jc.configSnapshot()
	isCron := util.ToInt(job["isCron"])
	if isCron == 2 {
		// Manual only, just enable
		if err := mapper.UpdateJobEnable(jobID, 1); err != nil {
			panic(err.Error())
		}
		jc.setEnable(1)
		return
	}

	if scheduler == nil {
		panicPublic(msg.CannotResumeLostJob)
	}
	err := scheduler.Resume(isCron, job, func() {
		jc.DoScheduled()
	})
	if err != nil {
		panicPublic(err.Error())
	}
	if err := mapper.UpdateJobEnable(jobID, 1); err != nil {
		scheduler.Pause()
		panic(err.Error())
	}
	jc.setEnable(1)
}

// AbortJob aborts the current running task
func (jc *JobClient) AbortJob() {
	if task := jc.currentTask(); task != nil {
		task.requestBreak()
	}
}

// StopJob stops the job (for disable or delete)
func (jc *JobClient) StopJob(remove bool) {
	jobID := jc.idSnapshot()
	scheduler := jc.schedulerSnapshot()
	if remove {
		jc.setEnable(0)
		if task := jc.currentTask(); task != nil {
			task.requestBreak()
		}
		if scheduler != nil {
			scheduler.Stop()
		}
	} else {
		if err := mapper.UpdateJobEnable(jobID, 0); err != nil {
			panic(err.Error())
		}
		if err := mapper.UpdateJobTaskStatusByStatusAndJobID(jobID); err != nil {
			panic(err.Error())
		}
		jc.setEnable(0)
		if task := jc.currentTask(); task != nil {
			task.requestBreak()
		}
		if scheduler != nil {
			scheduler.Pause()
		}
	}
}
