package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"opensync/internal/mapper"
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
	jt.updateTaskStatus()
	jt.JobClient.markDone()
	jt.JobClient.clearCurrentTask(jt)
}

func taskPersistenceErrorMessage(err error) string {
	return fmt.Sprintf("failed to save task items: %v", err)
}

func (jt *JobTask) finishFailedTask(errMsg string) {
	UpdateJobTaskStatusSimple(jt.TaskID, taskStatusSystemFailed, &errMsg)
	if jt.JobClient != nil {
		jt.JobClient.markDone()
		jt.JobClient.clearCurrentTask(jt)
	}
}

func (jt *JobTask) updateTaskStatus() {
	jt.GetCurrent()
	taskNum := GetCuTaskNum(jt.TaskID)
	failOrOtherNum := toInt(taskNum["failNum"]) + toInt(taskNum["otherNum"])
	status := finalTaskStatus(jt.isBreak(), jt.context().Err(), failOrOtherNum)
	duration := taskDuration(jt.CreateTime)
	taskNum["duration"] = duration
	taskNum["scanFinish"] = jt.ScanFinish.Load()
	taskNum["scan"] = jt.scanProgress()

	finishJobTaskStatus(jt.TaskID, status, nil, taskNum, duration, jt.CreateTime)
}

func finalTaskStatus(isBreak bool, ctxErr error, failOrOtherNum int) taskStatus {
	if isBreak {
		return taskStatusFailed
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return taskStatusTimeout
	}
	if failOrOtherNum > 0 {
		return taskStatusPartialFail
	}
	return taskStatusSuccess
}

// UpdateJobTaskStatusFinal updates task status after completion with notification
func UpdateJobTaskStatusFinal(taskID int64, status taskStatus, currentTasks map[int][]map[string]interface{}, createTime float64) {
	duration := taskDuration(createTime)

	var taskNum map[string]interface{}
	var errMsg *string

	if currentTasks != nil {
		successNum := len(currentTasks[taskStatusSuccess.Int()])
		failNum := len(currentTasks[taskStatusFailed.Int()])
		otherNum := len(currentTasks[taskStatusOther.Int()])
		allNum := successNum + failNum + otherNum

		var sumSize int64
		if tasks, ok := currentTasks[taskStatusSuccess.Int()]; ok {
			for _, t := range tasks {
				if t["fileSize"] != nil && taskItemTypeFromValue(t["type"]) != taskItemTypeDelete {
					sumSize += toInt64Val(t["fileSize"])
				}
			}
		}

		taskNum = map[string]interface{}{
			"waitNum":    0,
			"runningNum": 0,
			"successNum": successNum,
			"failNum":    failNum,
			"otherNum":   otherNum,
			"allNum":     allNum,
			"duration":   duration,
			"sumSize":    sumSize,
		}
	} else {
		taskNum = GetCuTaskNum(taskID)
		taskNum["duration"] = duration
	}

	finishJobTaskStatus(taskID, status, errMsg, taskNum, duration, createTime)
}

func finishJobTaskStatus(taskID int64, status taskStatus, errMsg *string, taskNum map[string]interface{}, duration int, createTime float64) {
	taskNumJSON, _ := json.Marshal(taskNum)
	mapper.UpdateJobTaskStatusAndNum(taskID, status.Int(), errMsg, string(taskNumJSON))

	// Send notifications
	SendTaskNotification(taskID, status.Int(), taskNum, duration, createTime)
}

func taskDuration(createTime float64) int {
	if createTime <= 0 {
		return 0
	}
	return int(float64(time.Now().Unix()) - createTime)
}

// UpdateJobTaskStatusSimple updates task status with error message
func UpdateJobTaskStatusSimple(taskID int64, status taskStatus, errMsg *string) {
	taskNum := GetCuTaskNum(taskID)
	taskNumJSON, _ := json.Marshal(taskNum)
	mapper.UpdateJobTaskStatusAndNum(taskID, status.Int(), errMsg, string(taskNumJSON))
}

// GetCuTaskNum gets current task counts from DB
func GetCuTaskNum(taskID int64) map[string]interface{} {
	return mapper.GetJobTaskCounts(taskID)
}
