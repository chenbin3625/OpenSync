package model

// User represents user_list table
type User struct {
	ID          int64  `json:"id" db:"id"`
	UserName    string `json:"userName" db:"userName"`
	Passwd      string `json:"passwd,omitempty" db:"passwd"`
	RecoveryKey string `json:"recoveryKey,omitempty" db:"recoveryKey"`
	SQLVersion  int64  `json:"sqlVersion,omitempty" db:"sqlVersion"`
	CreateTime  int64  `json:"createTime" db:"createTime"`
}

// Alist represents alist_list table
type Alist struct {
	ID         int64  `json:"id" db:"id"`
	Remark     string `json:"remark" db:"remark"`
	URL        string `json:"url" db:"url"`
	UserName   string `json:"userName" db:"userName"`
	Token      string `json:"token,omitempty" db:"token"`
	CreateTime int64  `json:"createTime" db:"createTime"`
}

// Job represents job table
type Job struct {
	ID            int64  `json:"id" db:"id"`
	Enable        int    `json:"enable" db:"enable"`
	Remark        string `json:"remark" db:"remark"`
	SrcPath       string `json:"srcPath" db:"srcPath"`
	DstPath       string `json:"dstPath" db:"dstPath"`
	AlistID       int64  `json:"alistId" db:"alistId"`
	UseCacheT     int    `json:"useCacheT" db:"useCacheT"`
	ScanIntervalT int    `json:"scanIntervalT" db:"scanIntervalT"`
	UseCacheS     int    `json:"useCacheS" db:"useCacheS"`
	ScanIntervalS int    `json:"scanIntervalS" db:"scanIntervalS"`
	Method        int    `json:"method" db:"method"`
	Interval      int    `json:"interval" db:"interval"`
	IsCron        int    `json:"isCron" db:"isCron"`
	Month         string `json:"month" db:"month"`
	Day           string `json:"day" db:"day"`
	DayOfWeek     string `json:"day_of_week" db:"day_of_week"`
	Hour          string `json:"hour" db:"hour"`
	Minute        string `json:"minute" db:"minute"`
	Second        string `json:"second" db:"second"`
	Exclude       string `json:"exclude" db:"exclude"`
	MinFileSize   int64  `json:"minFileSize" db:"minFileSize"`
	MaxFileSize   int64  `json:"maxFileSize" db:"maxFileSize"`
	CreateTime    int64  `json:"createTime" db:"createTime"`
}

// JobTask represents job_task table
type JobTask struct {
	ID         int64  `json:"id" db:"id"`
	JobID      int64  `json:"jobId" db:"jobId"`
	Status     int    `json:"status" db:"status"`
	ErrMsg     string `json:"errMsg" db:"errMsg"`
	RunTime    int64  `json:"runTime" db:"runTime"`
	TaskNum    string `json:"taskNum" db:"taskNum"`
	CreateTime int64  `json:"createTime" db:"createTime"`
}

// JobTaskItem represents job_task_item table
type JobTaskItem struct {
	ID          int64   `json:"id" db:"id"`
	TaskID      int64   `json:"taskId" db:"taskId"`
	SrcPath     string  `json:"srcPath" db:"srcPath"`
	DstPath     string  `json:"dstPath" db:"dstPath"`
	IsPath      int     `json:"isPath" db:"isPath"`
	FileName    string  `json:"fileName" db:"fileName"`
	FileSize    *int64  `json:"fileSize" db:"fileSize"`
	Type        int     `json:"type" db:"type"`
	AlistTaskID string  `json:"alistTaskId" db:"alistTaskId"`
	Status      int     `json:"status" db:"status"`
	Progress    float64 `json:"progress" db:"progress"`
	ErrMsg      string  `json:"errMsg" db:"errMsg"`
	CreateTime  int64   `json:"createTime" db:"createTime"`
}

// Notify represents notify table
type Notify struct {
	ID         int64  `json:"id" db:"id"`
	Enable     int    `json:"enable" db:"enable"`
	Method     int    `json:"method" db:"method"`
	Params     string `json:"params" db:"params"`
	CreateTime int64  `json:"createTime" db:"createTime"`
}

// Response is the unified API response format
type Response struct {
	Code int         `json:"code"`
	Data interface{} `json:"data"`
	Msg  string      `json:"msg"`
}

// PublicError is a panic payload that is safe to return to API clients.
type PublicError string

func (e PublicError) Error() string {
	return string(e)
}

// PageResult is the paginated result
type PageResult struct {
	DataList interface{} `json:"dataList"`
	Count    int64       `json:"count"`
}

// PageParam holds pagination parameters
type PageParam struct {
	PageSize int `json:"pageSize" form:"pageSize"`
	PageNum  int `json:"pageNum" form:"pageNum"`
}

// Success returns a success response
func Success(data interface{}) Response {
	if data == nil {
		return Response{Code: 200, Data: nil, Msg: "success"}
	}
	return Response{Code: 200, Data: data, Msg: "success"}
}

// Error returns an error response
func Error(msg string) Response {
	return Response{Code: 500, Data: nil, Msg: msg}
}

// Unauthorized returns a 401 response
func Unauthorized(msg string) Response {
	return Response{Code: 401, Data: nil, Msg: msg}
}
