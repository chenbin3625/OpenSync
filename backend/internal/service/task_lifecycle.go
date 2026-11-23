package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"opensync/internal/mapper"
	"opensync/pkg/util"
	"time"
)

func (jt *JobTask) finishSubmittedTask(persistErr error) {
	if fatalErr := jt.fatalError(); fatalErr != nil {
		jt.finishFailedTask(*fatalErr)
		return
	}
	if persistErr != nil {
		jt.finishFailedTask(taskPersistenceErrorMessage(persistErr))
		return
	}
	jt.finishSuccessfulTask()
}

func safeSendTaskNotification(taskID int64, status int, taskNum map[string]interface{}, duration int, createTime float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Task %d notification panic: %v", taskID, r)
		}
	}()
	SendTaskNotification(taskID, status, taskNum, duration, createTime)
}

func (jt *JobTask) finishSuccessfulTask() {
	taskNum, status, duration, err := jt.persistTaskStatus()
	if err != nil {
		jt.finishFailedTask(taskStatusUpdateErrorMessage(err))
		return
	}
	// Release the task context timer, push the terminal SSE update before
	// freeing the JobClient slot (preventing a fast next run from interleaving),
	// then send webhook notifications outside any job lock.
	jt.cancelTaskContext()
	jt.finished.Store(true)
	jt.notifyProgressNow()
	if jt.JobClient != nil {
		jt.JobClient.releaseTask(jt)
	}
	safeSendTaskNotification(jt.TaskID, status.Int(), taskNum, duration, jt.CreateTime)
}

func taskPersistenceErrorMessage(err error) string {
	return fmt.Sprintf("failed to save task items: %v", err)
}

func taskStatusUpdateErrorMessage(err error) string {
	return fmt.Sprintf("failed to update task status: %v", err)
}

func (jt *JobTask) finishFailedTask(errMsg string) {
	jobID := int64(0)
	if jt.JobClient != nil {
		jobID = jt.JobClient.idSnapshot()
	}
	log.Printf("Task %d failed (job %d): %s", jt.TaskID, jobID, errMsg)
	duration := taskDuration(jt.CreateTime)
	taskNum, err := mapper.GetJobTaskCounts(jt.TaskID)
	if err != nil {
		log.Printf("Failed to load counts for failed task %d: %v", jt.TaskID, err)
		taskNum = mapper.EmptyJobTaskCounts()
	}
	taskNum["waitNum"] = 0
	taskNum["runningNum"] = 0
	taskNum["duration"] = duration
	taskNum["scanFinish"] = jt.ScanFinish.Load()
	taskNum["scan"] = jt.scanProgress()
	taskNumJSON, _ := json.Marshal(taskNum)
	if err := mapper.UpdateJobTaskStatusAndNum(jt.TaskID, taskStatusSystemFailed.Int(), &errMsg, string(taskNumJSON)); err != nil {
		log.Printf("Failed to mark task %d as failed: %v", jt.TaskID, err)
	}
	jt.cancelTaskContext()
	jt.finished.Store(true)
	jt.notifyProgressNow()
	if jt.JobClient != nil {
		jt.JobClient.releaseTask(jt)
	}
	safeSendTaskNotification(jt.TaskID, taskStatusSystemFailed.Int(), taskNum, duration, jt.CreateTime)
}

func (jt *JobTask) updateTaskStatus() error {
	_, _, _, err := jt.persistTaskStatus()
	return err
}

func (jt *JobTask) persistTaskStatus() (map[string]interface{}, taskStatus, int, error) {
	taskNum, err := mapper.GetJobTaskCounts(jt.TaskID)
	if err != nil {
		return nil, taskStatusSystemFailed, 0, err
	}
	failOrOtherNum := util.ToInt(taskNum["failNum"]) + util.ToInt(taskNum["otherNum"])
	allNum := util.ToInt(taskNum["allNum"])
	status := finalTaskStatus(jt.BreakFlag.Load(), jt.context().Err(), allNum, failOrOtherNum)
	duration := taskDuration(jt.CreateTime)
	taskNum["waitNum"] = 0
	taskNum["runningNum"] = 0
	taskNum["duration"] = duration
	taskNum["scanFinish"] = jt.ScanFinish.Load()
	taskNum["scan"] = jt.scanProgress()

	taskNumJSON, err := json.Marshal(taskNum)
	if err != nil {
		return nil, status, duration, err
	}
	if err := mapper.UpdateJobTaskStatusAndNum(jt.TaskID, status.Int(), nil, string(taskNumJSON)); err != nil {
		return nil, status, duration, err
	}
	return taskNum, status, duration, nil
}

func finalTaskStatus(isBreak bool, ctxErr error, allNum, failOrOtherNum int) taskStatus {
	if isBreak {
		return taskStatusStopped
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return taskStatusTimeout
	}
	if failOrOtherNum > 0 {
		return taskStatusPartialFail
	}
	if allNum == 0 {
		return taskStatusNoSync
	}
	return taskStatusSuccess
}

func finishJobTaskStatus(taskID int64, status taskStatus, errMsg *string, taskNum map[string]interface{}, duration int, createTime float64) error {
	taskNumJSON, _ := json.Marshal(taskNum)
	if err := mapper.UpdateJobTaskStatusAndNum(taskID, status.Int(), errMsg, string(taskNumJSON)); err != nil {
		return err
	}

	// Send notifications
	safeSendTaskNotification(taskID, status.Int(), taskNum, duration, createTime)
	return nil
}

func taskDuration(createTime float64) int {
	if createTime <= 0 {
		return 0
	}
	return int(float64(time.Now().Unix()) - createTime)
}

// UpdateJobTaskStatusSimple updates task status with error message
func UpdateJobTaskStatusSimple(taskID int64, status taskStatus, errMsg *string) error {
	taskNum, err := mapper.GetJobTaskCounts(taskID)
	if err != nil {
		return err
	}
	taskNumJSON, _ := json.Marshal(taskNum)
	return mapper.UpdateJobTaskStatusAndNum(taskID, status.Int(), errMsg, string(taskNumJSON))
}

// GetCuTaskNum gets current task counts from DB
func GetCuTaskNum(taskID int64) map[string]interface{} {
	counts, err := mapper.GetJobTaskCounts(taskID)
	if err != nil {
		panic(err.Error())
	}
	return counts
}
