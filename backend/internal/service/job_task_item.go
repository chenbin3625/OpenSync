package service

import "opensync/pkg/util"

type taskStatus int
type taskItemType int
type taskItemObject int

const (
	taskStatusOther        taskStatus = -1
	taskStatusWaiting      taskStatus = 0
	taskStatusRunning      taskStatus = 1
	taskStatusSuccess      taskStatus = 2
	taskStatusPartialFail  taskStatus = 3
	taskStatusStopped      taskStatus = 4
	taskStatusTimeout      taskStatus = 5
	taskStatusSystemFailed taskStatus = 6
	taskStatusFailed       taskStatus = 7
	taskStatusRetrying     taskStatus = 8

	taskItemTypeCopy   taskItemType = 0
	taskItemTypeDelete taskItemType = 1
	taskItemTypeMove   taskItemType = 2

	taskItemFile taskItemObject = 0
	taskItemPath taskItemObject = 1
)

func (status taskStatus) Int() int {
	return int(status)
}

func taskStatusFromValue(value interface{}) taskStatus {
	return taskStatus(util.ToInt(value))
}

func taskStatusValues(statuses ...taskStatus) []int {
	values := make([]int, len(statuses))
	for i, status := range statuses {
		values[i] = status.Int()
	}
	return values
}

func (itemType taskItemType) Int() int {
	return int(itemType)
}

func taskItemTypeFromValue(value interface{}) taskItemType {
	return taskItemType(util.ToInt(value))
}

func (object taskItemObject) Int() int {
	return int(object)
}

func taskItemObjectFromValue(value interface{}) taskItemObject {
	return taskItemObject(util.ToInt(value))
}

func boolToTaskItemObject(value bool) taskItemObject {
	if value {
		return taskItemPath
	}
	return taskItemFile
}

type JobTaskItem struct {
	ID          int64
	TaskID      int64
	SrcPath     string
	DstPath     string
	IsPath      taskItemObject
	FileName    string
	FileSize    interface{}
	Type        taskItemType
	AlistTaskID string
	Status      taskStatus
	ErrMsg      *string
	CreateTime  int64
}

func NewCopyJobTaskItem(taskID int64, srcPath, dstPath, fileName string, fileSize interface{},
	alistTaskID string, status taskStatus, errMsg *string, isPath taskItemObject, copyType taskItemType, createTime int64) JobTaskItem {
	return JobTaskItem{
		TaskID:      taskID,
		SrcPath:     srcPath,
		DstPath:     dstPath,
		IsPath:      isPath,
		FileName:    fileName,
		FileSize:    fileSize,
		Type:        copyType,
		AlistTaskID: alistTaskID,
		Status:      status,
		ErrMsg:      errMsg,
		CreateTime:  createTime,
	}
}

func NewDeleteJobTaskItem(taskID int64, dstPath, fileName string, fileSize interface{},
	status taskStatus, errMsg *string, isPath taskItemObject, createTime int64) JobTaskItem {
	return JobTaskItem{
		TaskID:     taskID,
		DstPath:    dstPath,
		IsPath:     isPath,
		FileName:   fileName,
		FileSize:   fileSize,
		Type:       taskItemTypeDelete,
		Status:     status,
		ErrMsg:     errMsg,
		CreateTime: createTime,
	}
}

func (item JobTaskItem) ToMap() map[string]interface{} {
	var srcPath interface{} = item.SrcPath
	var alistTaskID interface{} = item.AlistTaskID
	if item.Type == taskItemTypeDelete {
		srcPath = nil
		alistTaskID = nil
	}

	result := map[string]interface{}{
		"taskId":      item.TaskID,
		"srcPath":     srcPath,
		"dstPath":     item.DstPath,
		"isPath":      item.IsPath.Int(),
		"fileName":    item.FileName,
		"fileSize":    item.FileSize,
		"type":        item.Type.Int(),
		"alistTaskId": alistTaskID,
		"status":      item.Status.Int(),
		"errMsg":      item.ErrMsg,
		"createTime":  item.CreateTime,
	}
	if item.ID != 0 {
		result["id"] = item.ID
	}
	return result
}

func jobTaskItemsToMaps(items []JobTaskItem) []map[string]interface{} {
	maps := make([]map[string]interface{}, len(items))
	for i, item := range items {
		maps[i] = item.ToMap()
	}
	return maps
}

func isOtherTaskStatus(status taskStatus) bool {
	return status != taskStatusWaiting &&
		status != taskStatusRunning &&
		status != taskStatusSuccess &&
		status != taskStatusFailed
}

func (item JobTaskItem) CountableFileSize() int64 {
	if item.FileSize == nil || item.Type == taskItemTypeDelete {
		return 0
	}
	return util.ToInt64(item.FileSize)
}
