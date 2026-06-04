package mapper

import (
	"database/sql"
	"errors"
	"fmt"
	"opensync/internal/i18n"
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
	rst, err := FetchAllToTable("SELECT * FROM job WHERE id IN (SELECT jobId FROM job_task WHERE id=?)", taskID)
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
		 method, interval, isCron, year, month, day, week, day_of_week, hour, minute, second, start_date, end_date, exclude)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job["enable"], job["remark"], job["srcPath"], job["dstPath"], job["alistId"],
		job["useCacheT"], job["scanIntervalT"], job["useCacheS"], job["scanIntervalS"],
		job["method"], job["interval"], job["isCron"],
		job["year"], job["month"], job["day"], job["week"], job["day_of_week"],
		job["hour"], job["minute"], job["second"], job["start_date"], job["end_date"], job["exclude"],
	)
}

// UpdateJob updates a job
func UpdateJob(job map[string]interface{}) error {
	return ExecuteUpdate(
		`UPDATE job SET enable=?, remark=?, srcPath=?, dstPath=?, alistId=?, useCacheT=?, scanIntervalT=?,
		 useCacheS=?, scanIntervalS=?, method=?, interval=?, isCron=?, year=?, month=?, day=?, week=?,
		 day_of_week=?, hour=?, minute=?, second=?, start_date=?, end_date=?, exclude=? WHERE id=?`,
		job["enable"], job["remark"], job["srcPath"], job["dstPath"], job["alistId"],
		job["useCacheT"], job["scanIntervalT"], job["useCacheS"], job["scanIntervalS"],
		job["method"], job["interval"], job["isCron"],
		job["year"], job["month"], job["day"], job["week"], job["day_of_week"],
		job["hour"], job["minute"], job["second"], job["start_date"], job["end_date"], job["exclude"],
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
	return FetchAllToPage("SELECT * FROM job_task WHERE jobId=? ORDER BY createTime DESC", params, jobID)
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

// --- Job Task Item ---

// AddJobTaskItemMany batch inserts task items
func AddJobTaskItemMany(items []map[string]interface{}) error {
	if len(items) == 0 {
		return nil
	}
	argsList := make([][]interface{}, 0, len(items))
	for _, item := range items {
		argsList = append(argsList, []interface{}{
			item["taskId"], item["srcPath"], item["dstPath"], item["isPath"], item["fileName"],
			item["fileSize"], item["type"], item["alistTaskId"], item["status"], item["errMsg"],
		})
	}
	return ExecuteMany(
		`INSERT INTO job_task_item (taskId, srcPath, dstPath, isPath, fileName, fileSize, type, alistTaskId, status, errMsg)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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

	baseSQL := fmt.Sprintf("SELECT * FROM job_task_item %s ORDER BY createTime DESC", where)

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

	return map[string]interface{}{"dataList": dataList, "count": toInt64(count)}, nil
}

// GetJobTaskCountByStatus counts task items by status
func GetJobTaskCountByStatus(taskID int64, status int) int64 {
	val, err := FetchFirstVal("SELECT COUNT(id) FROM job_task_item WHERE status=? AND taskId=?", status, taskID)
	if err != nil {
		return 0
	}
	return toInt64(val)
}

// GetJobTaskCountByOther counts task items with other status
func GetJobTaskCountByOther(taskID int64) int64 {
	val, err := FetchFirstVal("SELECT COUNT(id) FROM job_task_item WHERE status NOT IN (0,1,2,7) AND taskId=?", taskID)
	if err != nil {
		return 0
	}
	return toInt64(val)
}

// GetJobTaskCountByAll counts all task items for a task
func GetJobTaskCountByAll(taskID int64) int64 {
	val, err := FetchFirstVal("SELECT COUNT(id) FROM job_task_item WHERE taskId=?", taskID)
	if err != nil {
		return 0
	}
	return toInt64(val)
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
	return rows[0]
}
