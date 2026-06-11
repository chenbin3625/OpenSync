package mapper

import (
	"database/sql"
	"errors"
	"fmt"
	"opensync/internal/i18n"
	"opensync/pkg/util"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	jobTaskItemListColumns    = "id, taskId, srcPath, dstPath, isPath, fileName, fileSize, type, alistTaskId, status, progress, errMsg, createTime"
	jobTaskItemRuntimeColumns = "id, taskId, srcPath, dstPath, isPath, fileName, fileSize, type, alistTaskId, status, errMsg, createTime"
)

// GetJobList gets paginated job list
func GetJobList(params map[string]interface{}) (map[string]interface{}, error) {
	return FetchAllToPage("SELECT * FROM job ORDER BY createTime DESC", params)
}

// GetJobListAll gets all jobs
func GetJobListAll() ([]map[string]interface{}, error) {
	return FetchAllToTable("SELECT * FROM job ORDER BY createTime DESC")
}

// GetEnableJobList gets all enabled jobs
func GetEnableJobList() ([]map[string]interface{}, error) {
	return FetchAllToTable("SELECT * FROM job WHERE enable=1")
}

// GetJobByID gets job by ID
func GetJobByID(jobID int64) (map[string]interface{}, error) {
	rst, err := FetchAllToTable("SELECT * FROM job WHERE id=?", jobID)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, errors.New(i18n.G("job_not_found"))
	}
	return rst[0], nil
}

// GetJobByTaskID gets job by task ID
func GetJobByTaskID(taskID int64) (map[string]interface{}, error) {
	rst, err := FetchAllToTable(
		`SELECT j.* FROM job AS j
		 INNER JOIN job_task AS jt ON jt.jobId=j.id
		 WHERE jt.id=?`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, errors.New(i18n.G("job_not_found"))
	}
	return rst[0], nil
}

// AddJob inserts a new job
func AddJob(job map[string]interface{}) (int64, error) {
	return ExecuteInsert(
		`INSERT INTO job (enable, remark, srcPath, dstPath, alistId, useCacheT, scanIntervalT, useCacheS, scanIntervalS,
		 method, interval, isCron, month, day, day_of_week, hour, minute, second, exclude,
		 minFileSize, maxFileSize)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job["enable"], job["remark"], job["srcPath"], job["dstPath"], job["alistId"],
		job["useCacheT"], job["scanIntervalT"], job["useCacheS"], job["scanIntervalS"],
		job["method"], job["interval"], job["isCron"],
		job["month"], job["day"], job["day_of_week"],
		job["hour"], job["minute"], job["second"], job["exclude"],
		job["minFileSize"], job["maxFileSize"],
	)
}

// UpdateJob updates a job
func UpdateJob(job map[string]interface{}) error {
	return ExecuteUpdate(
		`UPDATE job SET enable=?, remark=?, srcPath=?, dstPath=?, alistId=?, useCacheT=?, scanIntervalT=?,
		 useCacheS=?, scanIntervalS=?, method=?, interval=?, isCron=?, month=?, day=?,
		 day_of_week=?, hour=?, minute=?, second=?, exclude=?, minFileSize=?, maxFileSize=? WHERE id=?`,
		job["enable"], job["remark"], job["srcPath"], job["dstPath"], job["alistId"],
		job["useCacheT"], job["scanIntervalT"], job["useCacheS"], job["scanIntervalS"],
		job["method"], job["interval"], job["isCron"],
		job["month"], job["day"], job["day_of_week"],
		job["hour"], job["minute"], job["second"], job["exclude"],
		job["minFileSize"], job["maxFileSize"],
		job["id"],
	)
}

// UpdateJobEnable updates job enable status
func UpdateJobEnable(jobID int64, enable int) error {
	return ExecuteUpdate("UPDATE job SET enable=? WHERE id=?", enable, jobID)
}

// DeleteJob deletes a job and its tasks
func DeleteJob(jobID int64) error {
	return withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM job_task_item WHERE taskId IN (SELECT id FROM job_task WHERE jobId=?)", jobID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM job_task WHERE jobId=?", jobID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM job WHERE id=?", jobID); err != nil {
			return err
		}
		return nil
	})
}

// --- Job Task ---

// GetJobTaskList gets paginated task list for a job
func GetJobTaskList(params map[string]interface{}) (map[string]interface{}, error) {
	jobID := params["id"]
	where := "WHERE jobId=?"
	args := []interface{}{jobID}

	if status, ok := params["status"]; ok {
		where += " AND status=?"
		args = append(args, util.ToInt(status))
	} else if statuses := parseStatusList(params["statusIn"]); len(statuses) > 0 {
		clause, statusArgs := statusInClause(statuses)
		where += fmt.Sprintf(" AND status IN (%s)", clause)
		args = append(args, statusArgs...)
	}
	if startTime, ok := params["startTime"]; ok {
		start := util.ToInt(startTime)
		if start > 0 {
			where += " AND COALESCE(NULLIF(runTime, 0), createTime) >= ?"
			args = append(args, start)
		}
	}
	if endTime, ok := params["endTime"]; ok {
		end := util.ToInt(endTime)
		if end > 0 {
			where += " AND COALESCE(NULLIF(runTime, 0), createTime) <= ?"
			args = append(args, end)
		}
	}
	if keyword, ok := params["keyword"]; ok {
		kw := strings.TrimSpace(fmt.Sprintf("%v", keyword))
		if kw != "" {
			where += " AND CAST(id AS TEXT) LIKE ? ESCAPE '\\'"
			args = append(args, "%"+escapeLike(kw)+"%")
		}
	}

	baseSQL := fmt.Sprintf("SELECT * FROM job_task %s ORDER BY createTime DESC", where)
	return FetchAllToPage(baseSQL, params, args...)
}

func parseStatusList(value interface{}) []int {
	switch v := value.(type) {
	case nil:
		return nil
	case []int:
		return v
	case []string:
		statuses := make([]int, 0, len(v))
		for _, item := range v {
			if strings.TrimSpace(item) == "" {
				continue
			}
			statuses = append(statuses, util.ToInt(item))
		}
		return statuses
	case []interface{}:
		statuses := make([]int, 0, len(v))
		for _, item := range v {
			statuses = append(statuses, util.ToInt(item))
		}
		return statuses
	case string:
		parts := strings.Split(v, ",")
		statuses := make([]int, 0, len(parts))
		for _, item := range parts {
			if strings.TrimSpace(item) == "" {
				continue
			}
			statuses = append(statuses, util.ToInt(item))
		}
		return statuses
	default:
		return []int{util.ToInt(v)}
	}
}

// GetJobTaskByID gets task by ID
func GetJobTaskByID(taskID int64) (map[string]interface{}, error) {
	rst, err := FetchAllToTable("SELECT * FROM job_task WHERE id=?", taskID)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, errors.New(i18n.G("task_not_found"))
	}
	return rst[0], nil
}

// AddJobTask inserts a new job task
func AddJobTask(jobID int64, runTime int64) (int64, error) {
	return ExecuteInsert("INSERT INTO job_task (jobId, runTime) VALUES (?, ?)", jobID, runTime)
}

// UpdateJobTaskStatus updates task status
func UpdateJobTaskStatus(taskID int64, status int, errMsg *string) error {
	return ExecuteUpdate("UPDATE job_task SET status=?, errMsg=? WHERE id=?", status, errMsg, taskID)
}

// UpdateJobTaskStatusByStatus updates incomplete tasks to aborted (for restart)
func UpdateJobTaskStatusByStatus() error {
	return ExecuteUpdate("UPDATE job_task SET status=4 WHERE status IN (0, 1)")
}

// UpdateJobTaskStatusByStatusAndJobID updates incomplete tasks to aborted for a job
func UpdateJobTaskStatusByStatusAndJobID(jobID int64) error {
	return ExecuteUpdate("UPDATE job_task SET status=4 WHERE status IN (0, 1) AND jobId=?", jobID)
}

// DeleteJobTaskByTaskID deletes a task and its items
func DeleteJobTaskByTaskID(taskID int64) error {
	return withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM job_task_item WHERE taskId=?", taskID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM job_task WHERE id=?", taskID); err != nil {
			return err
		}
		return nil
	})
}

// DeleteJobTaskByRunTime deletes old tasks
func DeleteJobTaskByRunTime(runTime int64) error {
	return withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM job_task_item WHERE taskId IN (SELECT id FROM job_task WHERE runTime < ?)", runTime); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM job_task WHERE runTime < ?", runTime); err != nil {
			return err
		}
		return nil
	})
}

func withTx(fn func(*sql.Tx) error) error {
	tx, err := GetDB().Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// UpdateJobTaskNumMany batch updates task result counts
func UpdateJobTaskNumMany(taskNums []map[string]interface{}) error {
	if len(taskNums) == 0 {
		return nil
	}
	argsList := make([][]interface{}, 0, len(taskNums))
	for _, tn := range taskNums {
		argsList = append(argsList, []interface{}{tn["taskNum"], tn["taskId"]})
	}
	return ExecuteMany("UPDATE job_task SET taskNum=? WHERE id=?", argsList)
}

// UpdateJobTaskStatusAndNum updates final status and cached counts in one write.
func UpdateJobTaskStatusAndNum(taskID int64, status int, errMsg *string, taskNum string) error {
	return ExecuteUpdate("UPDATE job_task SET status=?, errMsg=?, taskNum=? WHERE id=?", status, errMsg, taskNum, taskID)
}

// --- Job Task Item ---

// AddJobTaskItemMany batch inserts task items
func AddJobTaskItemMany(items []map[string]interface{}) error {
	if len(items) == 0 {
		return nil
	}
	argsList := make([][]interface{}, 0, len(items))
	for _, item := range items {
		createTime := item["createTime"]
		if createTime == nil {
			createTime = time.Now().Unix()
		}
		argsList = append(argsList, []interface{}{
			item["taskId"], item["srcPath"], item["dstPath"], item["isPath"], item["fileName"],
			item["fileSize"], item["type"], item["alistTaskId"], item["status"], item["errMsg"],
			createTime,
		})
	}
	return ExecuteMany(
		`INSERT INTO job_task_item (taskId, srcPath, dstPath, isPath, fileName, fileSize, type, alistTaskId, status, errMsg, createTime)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		argsList,
	)
}

// GetJobTaskItemList gets paginated task item list
func GetJobTaskItemList(params map[string]interface{}) (map[string]interface{}, error) {
	taskID := params["taskId"]
	where := "WHERE taskId=?"
	args := []interface{}{taskID}

	if status, ok := params["status"]; ok {
		where += " AND status=?"
		args = append(args, status)
	}
	if typ, ok := params["type"]; ok {
		where += " AND type=?"
		args = append(args, typ)
	}
	if isPath, ok := params["isPath"]; ok {
		where += " AND isPath=?"
		args = append(args, isPath)
	}
	if hasError, ok := params["hasError"]; ok {
		if util.ToInt(hasError) == 1 {
			where += " AND errMsg IS NOT NULL AND errMsg<>''"
		} else {
			where += " AND (errMsg IS NULL OR errMsg='')"
		}
	}
	if keyword, ok := params["keyword"]; ok {
		kw := strings.TrimSpace(fmt.Sprintf("%v", keyword))
		if kw != "" {
			filterSQL, filterArgs := taskItemKeywordFilter(kw)
			where += filterSQL
			args = append(args, filterArgs...)
		}
	}

	baseSQL := fmt.Sprintf("SELECT %s FROM job_task_item %s ORDER BY createTime DESC", jobTaskItemListColumns, where)

	// Manual pagination
	ps, pn, paginated, err := parsePageParams(params)
	if err != nil {
		return nil, err
	}
	if !paginated {
		dataList, err := FetchAllToTable(baseSQL, args...)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"dataList": dataList, "count": len(dataList)}, nil
	}

	offset := (pn - 1) * ps

	dataQuery := baseSQL + fmt.Sprintf(" LIMIT %d OFFSET %d", ps, offset)
	dataList, err := FetchAllToTable(dataQuery, args...)
	if err != nil {
		return nil, err
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM job_task_item %s", where)
	count, err := FetchFirstVal(countQuery, args...)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"dataList": dataList, "count": util.ToInt64(count)}, nil
}

// GetFailedJobTaskItems returns failed task items that can be retried.
func GetFailedJobTaskItems(taskID int64) ([]map[string]interface{}, error) {
	return FetchAllToTable(
		fmt.Sprintf(`SELECT %s FROM job_task_item
		 WHERE taskId=? AND status=7
		 ORDER BY createTime ASC, id ASC`, jobTaskItemRuntimeColumns),
		taskID,
	)
}

// CountJobTaskItemsByStatuses counts task items matching any of the given statuses.
func CountJobTaskItemsByStatuses(taskID int64, statuses []int) (int64, error) {
	if len(statuses) == 0 {
		return 0, nil
	}
	clause, args := statusInClause(statuses)
	query := fmt.Sprintf("SELECT COUNT(id) FROM job_task_item WHERE taskId=? AND status IN (%s)", clause)
	queryArgs := append([]interface{}{taskID}, args...)
	count, err := FetchFirstVal(query, queryArgs...)
	if err != nil {
		return 0, err
	}
	return util.ToInt64(count), nil
}

// ForEachJobTaskItemsByStatuses reads task items in bounded batches.
func ForEachJobTaskItemsByStatuses(taskID int64, statuses []int, batchSize int, fn func([]map[string]interface{}) error) error {
	if len(statuses) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	clause, statusArgs := statusInClause(statuses)
	lastCreateTime := int64(-1)
	lastID := int64(0)

	for {
		query := fmt.Sprintf(
			`SELECT %s FROM job_task_item
			 WHERE taskId=? AND status IN (%s)
			   AND (createTime > ? OR (createTime = ? AND id > ?))
			 ORDER BY createTime ASC, id ASC
			 LIMIT %d`,
			jobTaskItemRuntimeColumns,
			clause,
			batchSize,
		)
		args := append([]interface{}{taskID}, statusArgs...)
		args = append(args, lastCreateTime, lastCreateTime, lastID)
		items, err := FetchAllToTable(query, args...)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		if err := fn(items); err != nil {
			return err
		}
		last := items[len(items)-1]
		lastCreateTime = util.ToInt64(last["createTime"])
		lastID = util.ToInt64(last["id"])
	}
}

func statusInClause(statuses []int) (string, []interface{}) {
	placeholders := make([]string, 0, len(statuses))
	args := make([]interface{}, 0, len(statuses))
	for _, status := range statuses {
		placeholders = append(placeholders, "?")
		args = append(args, status)
	}
	return strings.Join(placeholders, ","), args
}

// GetResumableJobTaskItems returns interrupted task items that can continue
// without scanning the job again.
func GetResumableJobTaskItems(taskID int64) ([]map[string]interface{}, error) {
	return FetchAllToTable(
		fmt.Sprintf(`SELECT %s FROM job_task_item
		 WHERE taskId=? AND status IN (0, 1, 4)
		 ORDER BY createTime ASC, id ASC`, jobTaskItemRuntimeColumns),
		taskID,
	)
}

// GetJobTaskCounts returns all task item status counters in one query.
func GetJobTaskCounts(taskID int64) map[string]interface{} {
	rows, err := FetchAllToTable(
		`SELECT
			COUNT(id) AS allNum,
			COALESCE(SUM(CASE WHEN status=0 THEN 1 ELSE 0 END), 0) AS waitNum,
			COALESCE(SUM(CASE WHEN status=1 THEN 1 ELSE 0 END), 0) AS runningNum,
				COALESCE(SUM(CASE WHEN status=2 THEN 1 ELSE 0 END), 0) AS successNum,
				COALESCE(SUM(CASE WHEN status=7 THEN 1 ELSE 0 END), 0) AS failNum,
				COALESCE(SUM(CASE WHEN status NOT IN (0,1,2,7) THEN 1 ELSE 0 END), 0) AS otherNum,
				COALESCE(SUM(CASE WHEN status=2 AND type<>1 AND fileSize IS NOT NULL THEN fileSize ELSE 0 END), 0) AS sumSize
			FROM job_task_item WHERE taskId=?`,
		taskID,
	)
	if err != nil || len(rows) == 0 {
		return emptyJobTaskCounts()
	}
	return rows[0]
}

// GetJobTaskCountsByTaskIDs returns task item counters for many tasks in one query.
func GetJobTaskCountsByTaskIDs(taskIDs []int64) map[int64]map[string]interface{} {
	results := make(map[int64]map[string]interface{}, len(taskIDs))
	uniqueIDs := make([]int64, 0, len(taskIDs))
	seen := make(map[int64]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if taskID <= 0 {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		uniqueIDs = append(uniqueIDs, taskID)
		results[taskID] = emptyJobTaskCounts()
	}
	if len(uniqueIDs) == 0 {
		return results
	}

	clause, args := int64InClause(uniqueIDs)
	rows, err := FetchAllToTable(
		fmt.Sprintf(`SELECT
				taskId,
				COUNT(id) AS allNum,
				COALESCE(SUM(CASE WHEN status=0 THEN 1 ELSE 0 END), 0) AS waitNum,
				COALESCE(SUM(CASE WHEN status=1 THEN 1 ELSE 0 END), 0) AS runningNum,
				COALESCE(SUM(CASE WHEN status=2 THEN 1 ELSE 0 END), 0) AS successNum,
				COALESCE(SUM(CASE WHEN status=7 THEN 1 ELSE 0 END), 0) AS failNum,
				COALESCE(SUM(CASE WHEN status NOT IN (0,1,2,7) THEN 1 ELSE 0 END), 0) AS otherNum,
				COALESCE(SUM(CASE WHEN status=2 AND type<>1 AND fileSize IS NOT NULL THEN fileSize ELSE 0 END), 0) AS sumSize
			FROM job_task_item
			WHERE taskId IN (%s)
			GROUP BY taskId`, clause),
		args...,
	)
	if err != nil {
		return results
	}
	for _, row := range rows {
		taskID := util.ToInt64(row["taskId"])
		delete(row, "taskId")
		results[taskID] = row
	}
	return results
}

func emptyJobTaskCounts() map[string]interface{} {
	return map[string]interface{}{
		"waitNum":    int64(0),
		"runningNum": int64(0),
		"successNum": int64(0),
		"failNum":    int64(0),
		"otherNum":   int64(0),
		"allNum":     int64(0),
		"sumSize":    int64(0),
	}
}

func int64InClause(values []int64) (string, []interface{}) {
	placeholders := make([]string, 0, len(values))
	args := make([]interface{}, 0, len(values))
	for _, value := range values {
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}
	return strings.Join(placeholders, ","), args
}

func taskItemKeywordFilter(keyword string) (string, []interface{}) {
	if taskItemFTSAvailable() && utf8.RuneCountInString(keyword) >= 3 {
		return " AND id IN (SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?)", []interface{}{fts5Phrase(keyword)}
	}
	like := "%" + escapeLike(keyword) + "%"
	return " AND (fileName LIKE ? ESCAPE '\\' OR srcPath LIKE ? ESCAPE '\\' OR dstPath LIKE ? ESCAPE '\\' OR alistTaskId LIKE ? ESCAPE '\\' OR errMsg LIKE ? ESCAPE '\\')",
		[]interface{}{like, like, like, like, like}
}

func taskItemFTSAvailable() bool {
	var name string
	err := GetDB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='job_task_item_fts'").Scan(&name)
	return err == nil && name == "job_task_item_fts"
}

func fts5Phrase(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
