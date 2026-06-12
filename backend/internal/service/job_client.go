package service

import (
	"fmt"
	"log"
	"opensync/internal/i18n"
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
		if timeout <= 0 {
			jc.mu.Unlock()
			return false
		}
		stateCh := jc.stateChangeChLocked()
		jc.mu.Unlock()

		select {
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
		if isInit || addJobID != 0 {
			log.Printf("Error during job setup, deleting job: %v", job)
			mapper.DeleteJob(jc.JobID)
		}
		panicPublic(err.Error())
	}

	return jc
}

func (jc *JobClient) runMarkedJob() {
	jc.runMarkedJobWithRetryItems(nil)
}

func (jc *JobClient) runMarkedJobWithRetryItems(retryItems []map[string]interface{}) {
	jc.runMarkedJobWithRetryConfig(retryItems, 0, nil)
}

func (jc *JobClient) runMarkedJobWithRetrySource(sourceTaskID int64, statuses []taskStatus) {
	jc.runMarkedJobWithRetryConfig(nil, sourceTaskID, statuses)
}

func (jc *JobClient) runMarkedJobWithRetryConfig(retryItems []map[string]interface{}, sourceTaskID int64, statuses []taskStatus) {
	taskID := int64(0)
	defer func() {
		if r := recover(); r != nil {
			jc.markDone()
			jc.clearCurrentTask(nil)
			errMsg := fmt.Sprintf("%v", r)
			log.Printf("Job execution error: %s", errMsg)
			if taskID > 0 {
				UpdateJobTaskStatusSimple(taskID, taskStatusSystemFailed, &errMsg)
			}
		}
	}()

	var err error
	taskID, err = mapper.AddJobTask(jc.idSnapshot(), time.Now().Unix())
	if err != nil {
		panic(err.Error())
	}
	if !jc.enabled() {
		panicPublic(i18n.G("disabled_job_cannot_run"))
	}
	task := newJobTask(taskID, jc)
	if len(retryItems) > 0 {
		task.RetryItems = cloneTaskRows(retryItems)
	}
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
		panicPublic(i18n.G("job_running"))
	}
	go jc.runMarkedJob()
}

// DoRetryItems triggers a manual execution that only retries selected failed items.
func (jc *JobClient) DoRetryItems(items []map[string]interface{}) {
	if len(items) == 0 {
		panicPublic(i18n.G("no_failed_task_items"))
	}
	if !jc.tryMarkDoing() {
		panicPublic(i18n.G("job_running"))
	}
	go jc.runMarkedJobWithRetryItems(items)
}

func (jc *JobClient) DoRetryTaskItems(sourceTaskID int64) {
	if !jc.tryMarkDoing() {
		panicPublic(i18n.G("job_running"))
	}
	go jc.runMarkedJobWithRetrySource(sourceTaskID, []taskStatus{taskStatusFailed})
}

// DoResumeItems triggers a manual execution for interrupted items from a stopped task.
func (jc *JobClient) DoResumeItems(items []map[string]interface{}) {
	if len(items) == 0 {
		panicPublic(i18n.G("no_resumable_task_items"))
	}
	if !jc.tryMarkDoing() {
		panicPublic(i18n.G("job_running"))
	}
	go jc.runMarkedJobWithRetryItems(items)
}

func (jc *JobClient) DoResumeTaskItems(sourceTaskID int64) {
	if !jc.tryMarkDoing() {
		panicPublic(i18n.G("job_running"))
	}
	go jc.runMarkedJobWithRetrySource(sourceTaskID, []taskStatus{taskStatusWaiting, taskStatusRunning, taskStatusStopped})
}

// ResumeJob enables and resumes the job
func (jc *JobClient) ResumeJob() {
	jobID, job, scheduler := jc.configSnapshot()
	isCron := util.ToInt(job["isCron"])
	if isCron == 2 {
		// Manual only, just enable
		mapper.UpdateJobEnable(jobID, 1)
		jc.setEnable(1)
		return
	}

	err := scheduler.Resume(isCron, job, func() {
		jc.DoScheduled()
	})
	if err != nil {
		panic(err.Error())
	}
	mapper.UpdateJobEnable(jobID, 1)
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
	jc.setEnable(0)
	if task := jc.currentTask(); task != nil {
		task.requestBreak()
	}
	jobID := jc.idSnapshot()
	scheduler := jc.schedulerSnapshot()
	if remove {
		if scheduler != nil {
			scheduler.Stop()
		}
	} else {
		if scheduler != nil {
			scheduler.Pause()
		}
		mapper.UpdateJobEnable(jobID, 0)
		mapper.UpdateJobTaskStatusByStatusAndJobID(jobID)
	}
}
