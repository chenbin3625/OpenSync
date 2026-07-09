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

func (jt *JobTask) finishSuccessfulTask() {
	if err := jt.updateTaskStatus(); err != nil {
		jt.finishFailedTask(taskStatusUpdateErrorMessage(err))
		return
	}
	jt.JobClient.markDone()
	jt.JobClient.clearCurrentTask(jt)
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
		jobID = jt.JobClient.JobID
	}
	log.Printf("Task %d failed (job %d): %s", jt.TaskID, jobID, errMsg)
	if err := UpdateJobTaskStatusSimple(jt.TaskID, taskStatusSystemFailed, &errMsg); err != nil {
		log.Printf("Failed to mark task %d as failed: %v", jt.TaskID, err)
	}
	if jt.JobClient != nil {
		jt.JobClient.markDone()
		jt.JobClient.clearCurrentTask(jt)
	}
}

func (jt *JobTask) updateTaskStatus() error {
	taskNum := GetCuTaskNum(jt.TaskID)
	failOrOtherNum := util.ToInt(taskNum["failNum"]) + util.ToInt(taskNum["otherNum"])
	allNum := util.ToInt(taskNum["allNum"])
	status := finalTaskStatus(jt.isBreak(), jt.context().Err(), allNum, failOrOtherNum)
	duration := taskDuration(jt.CreateTime)
	taskNum["duration"] = duration
	taskNum["scanFinish"] = jt.ScanFinish.Load()
	taskNum["scan"] = jt.scanProgress()

	return finishJobTaskStatus(jt.TaskID, status, nil, taskNum, duration, jt.CreateTime)
}

func finalTaskStatus(isBreak bool, ctxErr error, allNum, failOrOtherNum int) taskStatus {
	if isBreak {
		return taskStatusFailed
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
	SendTaskNotification(taskID, status.Int(), taskNum, duration, createTime)
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
	taskNum := GetCuTaskNum(taskID)
	taskNumJSON, _ := json.Marshal(taskNum)
	return mapper.UpdateJobTaskStatusAndNum(taskID, status.Int(), errMsg, string(taskNumJSON))
}

// GetCuTaskNum gets current task counts from DB
func GetCuTaskNum(taskID int64) map[string]interface{} {
	return mapper.GetJobTaskCounts(taskID)
}
