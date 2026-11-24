package mapper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	jobTaskItemListColumns     = "id, taskId, srcPath, dstPath, isPath, fileName, fileSize, type, alistTaskId, status, progress, errMsg, createTime"
	jobTaskItemRuntimeColumns  = "id, taskId, srcPath, dstPath, isPath, fileName, fileSize, type, alistTaskId, status, errMsg, createTime"
	expiredTaskDeleteBatchSize = 500
	sqlInClauseChunkSize       = 500
	maxStatusListLen           = 64
)

// taskItemDeleteChunkSize bounds job_task_item rows deleted per write transaction
// so removing a large task does not hold SQLite's write lock indefinitely.
var taskItemDeleteChunkSize = 5000

// GetJobList gets paginated job list. When locateJobId is provided alongside
// pagination parameters, it resolves the page containing that job inside the
// same read-only snapshot and returns pageNum and locatedJobFound.
func GetJobList(params map[string]interface{}) (map[string]interface{}, error) {
	const baseSQL = "SELECT * FROM job ORDER BY createTime DESC, id DESC"
	locateJobID, hasLocate := parsePositiveInt64Param(params["locateJobId"])
	if !hasLocate {
		return FetchAllToPage(baseSQL, params)
	}

	pageSize, pageNum, paginated, err := parsePageParams(params)
	if err != nil {
		return nil, err
	}
	if !paginated {
		return FetchAllToPage(baseSQL, params)
	}

	ctx := context.Background()
	tx, err := GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	resolvedPage := pageNum
	found := false
	var ct sql.NullInt64
	err = tx.QueryRowContext(ctx, "SELECT createTime FROM job WHERE id=?", locateJobID).Scan(&ct)
	if err == nil {
		found = true
		var beforeCount int64
		if ct.Valid {
			err = tx.QueryRowContext(
				ctx,
				"SELECT COUNT(*) FROM job WHERE createTime > ? OR (createTime = ? AND id > ?)",
				ct.Int64, ct.Int64, locateJobID,
			).Scan(&beforeCount)
		} else {
			err = tx.QueryRowContext(
				ctx,
				"SELECT COUNT(*) FROM job WHERE createTime IS NOT NULL OR id > ?",
				locateJobID,
			).Scan(&beforeCount)
		}
		if err != nil {
			return nil, err
		}
		resolvedPage = int(beforeCount/int64(pageSize)) + 1
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	offset, err := pageOffset(pageSize, resolvedPage)
	if err != nil {
		return nil, err
	}
	result, err := fetchPageTx(ctx, tx, baseSQL, pageSize, offset, true, nil)
	if err != nil {
		return nil, err
	}
	result["pageNum"] = resolvedPage
	result["locatedJobFound"] = found
	return result, nil
}

func parsePositiveInt64Param(value interface{}) (int64, bool) {
	if value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return int64(v), v > 0
	case int64:
		return v, v > 0
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v <= 0 || v >= maxFloat64BeforeInt64Overflow {
			return 0, false
		}
		return int64(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		return n, err == nil && n > 0
	default:
		return 0, false
	}
}

// GetJobListAll gets all jobs
func GetJobListAll() ([]map[string]interface{}, error) {
	return FetchAllToTable("SELECT * FROM job ORDER BY createTime DESC, id DESC")
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
		return nil, errors.New(msg.JobNotFound)
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
		return nil, errors.New(msg.JobNotFound)
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
	return executeUpdateAffected(
		errors.New(msg.JobNotFound),
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

// DeleteJob deletes a job and its tasks. Large item sets are removed in bounded
// chunks before deleting the parent job/job_task rows so an interruption never
// leaves orphaned job_task_item rows unreachable by retries or retention cleanup.
func DeleteJob(jobID int64) error {
	chunkSize := effectiveItemDeleteChunkSize()
	for {
		done := false
		err := withImmediateWriteTx(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
			var probeCount int64
			if err := conn.QueryRowContext(
				ctx,
				"SELECT COUNT(*) FROM (SELECT 1 FROM job_task_item WHERE taskId IN (SELECT id FROM job_task WHERE jobId=?) LIMIT ?)",
				jobID, chunkSize+1,
			).Scan(&probeCount); err != nil {
				return err
			}
			if probeCount > int64(chunkSize) {
				if _, err := conn.ExecContext(
					ctx,
					"DELETE FROM job_task_item WHERE id IN (SELECT id FROM job_task_item WHERE taskId IN (SELECT id FROM job_task WHERE jobId=?) LIMIT ?)",
					jobID, chunkSize,
				); err != nil {
					return err
				}
				return nil
			}
			if _, err := conn.ExecContext(
				ctx,
				"DELETE FROM job_task_item WHERE taskId IN (SELECT id FROM job_task WHERE jobId=?)",
				jobID,
			); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, "DELETE FROM job_task WHERE jobId=?", jobID); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, "DELETE FROM job WHERE id=?", jobID); err != nil {
				return err
			}
			done = true
			return nil
		})
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// --- Job Task ---

// GetJobTaskList gets paginated task list for a job
func GetJobTaskList(params map[string]interface{}) (map[string]interface{}, error) {
	jobID := params["id"]
	where := "WHERE jobId=?"
	args := []interface{}{jobID}

	if status, ok := params["status"]; ok && status != nil {
		where += " AND status=?"
		args = append(args, util.ToInt(status))
	} else if statuses := parseStatusList(params["statusIn"]); len(statuses) > 0 {
		clause, statusArgs := statusInClause(statuses)
		where += fmt.Sprintf(" AND status IN (%s)", clause)
		args = append(args, statusArgs...)
	}
	if startTime, ok := params["startTime"]; ok && startTime != nil {
		start := util.ToInt64(startTime)
		if start > 0 {
			where += " AND COALESCE(NULLIF(runTime, 0), createTime) >= ?"
			args = append(args, start)
		}
	}
	if endTime, ok := params["endTime"]; ok && endTime != nil {
		end := util.ToInt64(endTime)
		if end > 0 {
			where += " AND COALESCE(NULLIF(runTime, 0), createTime) <= ?"
			args = append(args, end)
		}
	}
	if keyword, ok := params["keyword"]; ok && keyword != nil {
		kw := strings.TrimSpace(util.StringValue(keyword))
		if kw != "" {
			where += " AND CAST(id AS TEXT) LIKE ? ESCAPE '\\'"
			args = append(args, "%"+escapeLike(kw)+"%")
		}
	}

	baseSQL := fmt.Sprintf("SELECT * FROM job_task %s ORDER BY createTime DESC, id DESC", where)
	return FetchAllToPage(baseSQL, params, args...)
}

func parseStatusInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		if v < math.MinInt || v > math.MaxInt {
			return 0, false
		}
		return int(v), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
			return 0, false
		}
		if v < float64(math.MinInt) || v >= maxFloat64BeforeInt64Overflow {
			return 0, false
		}
		n := int64(v)
		if n < int64(math.MinInt) || n > int64(math.MaxInt) {
			return 0, false
		}
		return int(n), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func dedupeStatuses(raw []int) []int {
	if len(raw) == 0 {
		return nil
	}
	statuses := make([]int, 0, len(raw))
	seen := make(map[int]struct{}, len(raw))
	for _, s := range raw {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		statuses = append(statuses, s)
		if len(statuses) >= maxStatusListLen {
			break
		}
	}
	return statuses
}

func parseStatusList(value interface{}) []int {
	appendIfValid := func(statuses []int, seen map[int]struct{}, item interface{}) []int {
		if len(statuses) >= maxStatusListLen {
			return statuses
		}
		parsed, ok := parseStatusInt(item)
		if !ok {
			return statuses
		}
		if _, exists := seen[parsed]; exists {
			return statuses
		}
		seen[parsed] = struct{}{}
		return append(statuses, parsed)
	}

	switch v := value.(type) {
	case nil:
		return nil
	case []int:
		return dedupeStatuses(v)
	case []string:
		statuses := make([]int, 0, len(v))
		seen := make(map[int]struct{}, len(v))
		for _, item := range v {
			statuses = appendIfValid(statuses, seen, item)
		}
		return statuses
	case []interface{}:
		statuses := make([]int, 0, len(v))
		seen := make(map[int]struct{}, len(v))
		for _, item := range v {
			statuses = appendIfValid(statuses, seen, item)
		}
		return statuses
	case string:
		parts := strings.Split(v, ",")
		statuses := make([]int, 0, len(parts))
		seen := make(map[int]struct{}, len(parts))
		for _, item := range parts {
			statuses = appendIfValid(statuses, seen, item)
		}
		return statuses
	default:
		if parsed, ok := parseStatusInt(v); ok {
			return []int{parsed}
		}
		return nil
	}
}

// GetJobTaskByID gets task by ID
func GetJobTaskByID(taskID int64) (map[string]interface{}, error) {
	rst, err := FetchAllToTable("SELECT * FROM job_task WHERE id=?", taskID)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, errors.New(msg.TaskNotFound)
	}
	return rst[0], nil
}

// AddJobTask inserts a new job task, recording the job's current alistId when available.
func AddJobTask(jobID int64, runTime int64) (int64, error) {
	return ExecuteInsert(
		"INSERT INTO job_task (jobId, alistId, runTime) VALUES (?, COALESCE((SELECT alistId FROM job WHERE id=?), 0), ?)",
		jobID, jobID, runTime,
	)
}

// AddJobTaskWithAlist inserts a new job task with an explicit alistId snapshot.
func AddJobTaskWithAlist(jobID, alistID, runTime int64) (int64, error) {
	return ExecuteInsert("INSERT INTO job_task (jobId, alistId, runTime) VALUES (?, ?, ?)", jobID, alistID, runTime)
}

// BackfillJobTaskAlistID records the previous engine ID on pre-migration tasks
// before a job switches to a different engine.
func BackfillJobTaskAlistID(jobID, alistID int64) error {
	if jobID <= 0 || alistID <= 0 {
		return nil
	}
	return ExecuteUpdate(
		"UPDATE job_task SET alistId=? WHERE jobId=? AND (alistId IS NULL OR alistId=0)",
		alistID, jobID,
	)
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

func effectiveItemDeleteChunkSize() int {
	if taskItemDeleteChunkSize <= 0 {
		return 5000
	}
	return taskItemDeleteChunkSize
}

// DeleteJobTaskByTaskID deletes a task and its items. Excess items are removed
// in bounded chunks before the parent job_task row is deleted in the final
// transaction so an interruption leaves the parent retryable with no orphans.
func DeleteJobTaskByTaskID(taskID int64) error {
	chunkSize := effectiveItemDeleteChunkSize()
	for {
		done := false
		err := withImmediateWriteTx(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
			var status int
			if err := conn.QueryRowContext(ctx, "SELECT status FROM job_task WHERE id=?", taskID).Scan(&status); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return errors.New(msg.TaskNotFound)
				}
				return err
			}
			if status == 0 || status == 1 {
				return errors.New(msg.JobRunningCannotDelete)
			}

			var probeCount int64
			if err := conn.QueryRowContext(
				ctx,
				"SELECT COUNT(*) FROM (SELECT 1 FROM job_task_item WHERE taskId=? LIMIT ?)",
				taskID, chunkSize+1,
			).Scan(&probeCount); err != nil {
				return err
			}
			if probeCount > int64(chunkSize) {
				if _, err := conn.ExecContext(
					ctx,
					"DELETE FROM job_task_item WHERE id IN (SELECT id FROM job_task_item WHERE taskId=? LIMIT ?)",
					taskID, chunkSize,
				); err != nil {
					return err
				}
				return nil
			}

			result, err := conn.ExecContext(ctx, "DELETE FROM job_task WHERE id=? AND status NOT IN (0,1)", taskID)
			if err != nil {
				return err
			}
			if affected, err := result.RowsAffected(); err != nil {
				return err
			} else if affected == 0 {
				return errors.New(msg.TaskNotFound)
			}

			if _, err := conn.ExecContext(ctx, "DELETE FROM job_task_item WHERE taskId=?", taskID); err != nil {
				return err
			}
			done = true
			return nil
		})
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// DeleteJobTaskByRunTime deletes old tasks and sweeps any orphaned items.
func DeleteJobTaskByRunTime(runTime int64) error {
	for {
		taskIDs, err := expiredJobTaskIDs(runTime, expiredTaskDeleteBatchSize)
		if err != nil {
			return err
		}
		if len(taskIDs) == 0 {
			break
		}
		deletedCount, err := deleteJobTasksByIDs(taskIDs)
		if err != nil {
			return err
		}
		if deletedCount == 0 {
			break
		}
	}
	if err := deleteOrphanedTaskItems(effectiveItemDeleteChunkSize()); err != nil {
		return err
	}
	runRetentionMaintenance()
	return nil
}

func runRetentionMaintenance() {
	handle := GetDB()
	_, _ = handle.Exec("PRAGMA wal_checkpoint(PASSIVE)")
	_, _ = handle.Exec("PRAGMA optimize")
	var autoVacuum int
	// PRAGMA incremental_vacuum is a no-op when auto_vacuum is NONE (0) on
	// pre-existing DBs; only invoke it when INCREMENTAL (2) mode is active.
	if err := handle.QueryRow("PRAGMA auto_vacuum").Scan(&autoVacuum); err == nil && autoVacuum == 2 {
		_, _ = handle.Exec("PRAGMA incremental_vacuum(200)")
	}
}

func expiredJobTaskIDs(cutoff int64, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = expiredTaskDeleteBatchSize
	}
	rows, err := GetDB().Query(
		`SELECT id FROM job_task
		 WHERE COALESCE(NULLIF(runTime, 0), createTime) < ?
		   AND status NOT IN (0, 1)
		 ORDER BY COALESCE(NULLIF(runTime, 0), createTime), id
		 LIMIT ?`,
		cutoff,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var taskIDs []int64
	for rows.Next() {
		var taskID int64
		if err := rows.Scan(&taskID); err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

func queryEligibleTaskIDs(ctx context.Context, conn *sql.Conn, clause string, args []interface{}) ([]int64, error) {
	rows, err := conn.QueryContext(
		ctx,
		"SELECT id FROM job_task WHERE id IN ("+clause+") AND status NOT IN (0, 1)",
		args...,
	)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return ids, nil
}

func deleteJobTasksByIDs(taskIDs []int64) (int, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}
	chunkSize := effectiveItemDeleteChunkSize()
	totalDeleted := 0

	for start := 0; start < len(taskIDs); start += sqlInClauseChunkSize {
		end := start + sqlInClauseChunkSize
		if end > len(taskIDs) {
			end = len(taskIDs)
		}
		idBatch := taskIDs[start:end]
		clause, args := int64InClause(idBatch)

		for {
			done := false
			var batchDeleted int
			err := withImmediateWriteTx(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
				batchDeleted = 0

				eligibleIDs, err := queryEligibleTaskIDs(ctx, conn, clause, args)
				if err != nil {
					return err
				}
				if len(eligibleIDs) == 0 {
					done = true
					return nil
				}

				elClause, elArgs := int64InClause(eligibleIDs)
				var probeCount int64
				if err := conn.QueryRowContext(
					ctx,
					"SELECT COUNT(*) FROM (SELECT 1 FROM job_task_item WHERE taskId IN ("+elClause+") LIMIT ?)",
					appendSQLArgs(elArgs, chunkSize+1)...,
				).Scan(&probeCount); err != nil {
					return err
				}
				if probeCount > int64(chunkSize) {
					if _, err := conn.ExecContext(
						ctx,
						"DELETE FROM job_task_item WHERE id IN (SELECT id FROM job_task_item WHERE taskId IN ("+elClause+") LIMIT ?)",
						appendSQLArgs(elArgs, chunkSize)...,
					); err != nil {
						return err
					}
					return nil
				}

				// Re-verify status NOT IN (0, 1) atomically under BEGIN IMMEDIATE so
				// tasks that became active are never deleted, and remove the parent
				// rows in the same final transaction as their last chunk of items.
				rows, err := conn.QueryContext(
					ctx,
					"DELETE FROM job_task WHERE id IN ("+elClause+") AND status NOT IN (0, 1) RETURNING id",
					elArgs...,
				)
				if err != nil {
					return err
				}
				var deletedTaskIDs []int64
				for rows.Next() {
					var id int64
					if err := rows.Scan(&id); err != nil {
						_ = rows.Close()
						return err
					}
					deletedTaskIDs = append(deletedTaskIDs, id)
				}
				if err := rows.Err(); err != nil {
					_ = rows.Close()
					return err
				}
				if err := rows.Close(); err != nil {
					return err
				}

				if len(deletedTaskIDs) > 0 {
					delClause, delArgs := int64InClause(deletedTaskIDs)
					if _, err := conn.ExecContext(
						ctx,
						"DELETE FROM job_task_item WHERE taskId IN ("+delClause+")",
						delArgs...,
					); err != nil {
						return err
					}
				}
				batchDeleted = len(deletedTaskIDs)
				done = true
				return nil
			})
			if err != nil {
				return totalDeleted, err
			}
			totalDeleted += batchDeleted
			if done {
				break
			}
		}
	}

	return totalDeleted, nil
}

func deleteOrphanedTaskItems(chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = effectiveItemDeleteChunkSize()
	}
	for {
		var deleted int64
		err := withImmediateWriteTx(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
			res, err := conn.ExecContext(
				ctx,
				`DELETE FROM job_task_item
				 WHERE id IN (
				     SELECT jti.id FROM job_task_item AS jti
				     LEFT JOIN job_task AS jt ON jt.id = jti.taskId
				     WHERE jt.id IS NULL
				     LIMIT ?
				 )`,
				chunkSize,
			)
			if err != nil {
				return err
			}
			affected, err := res.RowsAffected()
			if err != nil {
				return err
			}
			deleted = affected
			return nil
		})
		if err != nil {
			return err
		}
		if deleted < int64(chunkSize) {
			return nil
		}
	}
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
	return GetJobTaskItemListContext(context.Background(), params)
}

func GetJobTaskItemListContext(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	taskID := params["taskId"]
	where := "WHERE taskId=?"
	args := []interface{}{taskID}

	if status, ok := params["status"]; ok && status != nil {
		if util.ToInt(status) == -1 {
			where += " AND status NOT IN (0,1,2,7)"
		} else {
			where += " AND status=?"
			args = append(args, status)
		}
	}
	if typ, ok := params["type"]; ok && typ != nil {
		where += " AND type=?"
		args = append(args, typ)
	}
	if isPath, ok := params["isPath"]; ok && isPath != nil {
		where += " AND isPath=?"
		args = append(args, isPath)
	}
	if hasError, ok := params["hasError"]; ok && hasError != nil {
		if util.ToInt(hasError) == 1 {
			where += " AND errMsg IS NOT NULL AND errMsg<>''"
		} else {
			where += " AND (errMsg IS NULL OR errMsg='')"
		}
	}
	if keyword, ok := params["keyword"]; ok && keyword != nil {
		kw := strings.TrimSpace(util.StringValue(keyword))
		if kw != "" {
			filterSQL, filterArgs, pureFTS := taskItemKeywordFilter(ctx, kw)
			if pureFTS {
				// Drive selective FTS5 searches from job_task_item_fts -> PRIMARY KEY
				// rather than scanning every row for taskId via idx_job_task_item_task_time.
				where = strings.Replace(where, "WHERE taskId=?", "WHERE +taskId=?", 1)
			}
			where += filterSQL
			args = append(args, filterArgs...)
		}
	}

	baseSQL := fmt.Sprintf("SELECT %s FROM job_task_item %s ORDER BY createTime DESC, id DESC", jobTaskItemListColumns, where)
	return fetchAllToPageContext(ctx, baseSQL, params, args...)
}

// CountJobTaskItemsByStatuses counts task items matching any of the given statuses.
func CountJobTaskItemsByStatuses(taskID int64, statuses []int) (int64, error) {
	statuses = dedupeStatuses(statuses)
	if len(statuses) == 0 {
		return 0, nil
	}
	clause, args := statusInClause(statuses)
	query := fmt.Sprintf("SELECT COUNT(id) FROM job_task_item WHERE taskId=? AND status IN (%s)", clause)
	queryArgs := make([]interface{}, 1, len(args)+1)
	queryArgs[0] = taskID
	queryArgs = append(queryArgs, args...)
	var count int64
	if err := GetDB().QueryRow(query, queryArgs...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ForEachJobTaskItemsByStatuses reads task items in bounded batches.
func ForEachJobTaskItemsByStatuses(taskID int64, statuses []int, batchSize int, fn func([]map[string]interface{}) error) error {
	statuses = dedupeStatuses(statuses)
	if len(statuses) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	clause, statusArgs := statusInClause(statuses)
	hasCursor := false
	lastCreateTimeNull := false
	var lastCreateTime int64
	var lastID int64

	for {
		var query string
		args := make([]interface{}, 0, len(statusArgs)+4)
		args = append(args, taskID)
		args = append(args, statusArgs...)

		switch {
		case !hasCursor:
			// Omit the cursor predicate on the first page so rows with
			// createTime IS NULL (which sort first in ASC order) are included.
			query = fmt.Sprintf(
				`SELECT %s FROM job_task_item
				 WHERE taskId=? AND +status IN (%s)
				 ORDER BY createTime ASC, id ASC
				 LIMIT ?`,
				jobTaskItemRuntimeColumns,
				clause,
			)
			args = append(args, batchSize)
		case lastCreateTimeNull:
			query = fmt.Sprintf(
				`SELECT %s FROM job_task_item
				 WHERE taskId=? AND +status IN (%s)
				   AND ((createTime IS NULL AND id > ?) OR createTime IS NOT NULL)
				 ORDER BY createTime ASC, id ASC
				 LIMIT ?`,
				jobTaskItemRuntimeColumns,
				clause,
			)
			args = append(args, lastID, batchSize)
		default:
			query = fmt.Sprintf(
				`SELECT %s FROM job_task_item
				 WHERE taskId=? AND +status IN (%s)
				   AND (createTime, id) > (?, ?)
				 ORDER BY createTime ASC, id ASC
				 LIMIT ?`,
				jobTaskItemRuntimeColumns,
				clause,
			)
			args = append(args, lastCreateTime, lastID, batchSize)
		}

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
		if len(items) < batchSize {
			return nil
		}
		last := items[len(items)-1]
		hasCursor = true
		if last["createTime"] == nil {
			lastCreateTimeNull = true
			lastCreateTime = 0
		} else {
			lastCreateTimeNull = false
			lastCreateTime = util.ToInt64(last["createTime"])
		}
		lastID = util.ToInt64(last["id"])
	}
}

const jobTaskCountSelect = `
			COUNT(id) AS allNum,
			COALESCE(SUM(CASE WHEN status=0 THEN 1 ELSE 0 END), 0) AS waitNum,
			COALESCE(SUM(CASE WHEN status=1 THEN 1 ELSE 0 END), 0) AS runningNum,
			COALESCE(SUM(CASE WHEN status=2 THEN 1 ELSE 0 END), 0) AS successNum,
			COALESCE(SUM(CASE WHEN status=7 THEN 1 ELSE 0 END), 0) AS failNum,
			COALESCE(SUM(CASE WHEN status NOT IN (0,1,2,7) THEN 1 ELSE 0 END), 0) AS otherNum,
			COALESCE(SUM(CASE WHEN status=2 AND type<>1 AND fileSize IS NOT NULL THEN fileSize ELSE 0 END), 0) AS sumSize`

func inClause(values []interface{}) (string, []interface{}) {
	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ","), values
}

func statusInClause(statuses []int) (string, []interface{}) {
	args := make([]interface{}, 0, len(statuses))
	for _, status := range statuses {
		args = append(args, status)
	}
	return inClause(args)
}

func int64InClause(values []int64) (string, []interface{}) {
	args := make([]interface{}, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return inClause(args)
}

// GetJobTaskCounts returns all task item status counters in one query.
func GetJobTaskCounts(taskID int64) (map[string]interface{}, error) {
	var counts jobTaskCounts
	if err := GetDB().QueryRow(
		fmt.Sprintf(`SELECT%s FROM job_task_item WHERE taskId=?`, jobTaskCountSelect),
		taskID,
	).Scan(
		&counts.allNum,
		&counts.waitNum,
		&counts.runningNum,
		&counts.successNum,
		&counts.failNum,
		&counts.otherNum,
		&counts.sumSize,
	); err != nil {
		return nil, err
	}
	return counts.toMap(), nil
}

// GetJobTaskCountsByTaskIDs returns task item counters for many tasks in one query.
func GetJobTaskCountsByTaskIDs(taskIDs []int64) map[int64]map[string]interface{} {
	results, _ := GetJobTaskCountsByTaskIDsWithError(taskIDs)
	return results
}

func GetJobTaskCountsByTaskIDsWithError(taskIDs []int64) (map[int64]map[string]interface{}, error) {
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
		results[taskID] = EmptyJobTaskCounts()
	}
	if len(uniqueIDs) == 0 {
		return results, nil
	}

	ctx := context.Background()
	tx, err := GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return results, err
	}
	defer tx.Rollback()

	for start := 0; start < len(uniqueIDs); start += sqlInClauseChunkSize {
		end := start + sqlInClauseChunkSize
		if end > len(uniqueIDs) {
			end = len(uniqueIDs)
		}
		clause, args := int64InClause(uniqueIDs[start:end])
		rows, err := tx.QueryContext(
			ctx,
			fmt.Sprintf(`SELECT
					taskId,%s
				FROM job_task_item
				WHERE taskId IN (%s)
				GROUP BY taskId`, jobTaskCountSelect, clause),
			args...,
		)
		if err != nil {
			return results, err
		}
		for rows.Next() {
			var taskID int64
			var counts jobTaskCounts
			if err := rows.Scan(
				&taskID,
				&counts.allNum,
				&counts.waitNum,
				&counts.runningNum,
				&counts.successNum,
				&counts.failNum,
				&counts.otherNum,
				&counts.sumSize,
			); err != nil {
				_ = rows.Close()
				return results, err
			}
			results[taskID] = counts.toMap()
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return results, err
		}
		if err := rows.Close(); err != nil {
			return results, err
		}
	}
	return results, nil
}

type jobTaskCounts struct {
	allNum     int64
	waitNum    int64
	runningNum int64
	successNum int64
	failNum    int64
	otherNum   int64
	sumSize    int64
}

func (c jobTaskCounts) toMap() map[string]interface{} {
	return map[string]interface{}{
		"waitNum":    c.waitNum,
		"runningNum": c.runningNum,
		"successNum": c.successNum,
		"failNum":    c.failNum,
		"otherNum":   c.otherNum,
		"allNum":     c.allNum,
		"sumSize":    c.sumSize,
	}
}

func EmptyJobTaskCounts() map[string]interface{} {
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

func taskItemKeywordFilter(ctx context.Context, keyword string) (string, []interface{}, bool) {
	like := "%" + escapeLike(keyword) + "%"
	sanitized := sanitizeFTS5Term(keyword)
	if sanitized == keyword && utf8.RuneCountInString(sanitized) >= 3 {
		available, coversAll := taskItemFTSCapabilities(ctx)
		if available {
			if coversAll {
				// All searchable columns (fileName, srcPath, dstPath, alistTaskId,
				// errMsg) are indexed in job_task_item_fts, avoiding an OR LIKE
				// fallback that would force SQLite to scan every task item.
				return ` AND id IN (SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?)`,
					[]interface{}{fts5Phrase(sanitized)}, true
			}
			return ` AND (
			id IN (SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?)
			OR alistTaskId LIKE ? ESCAPE '\'
			OR errMsg LIKE ? ESCAPE '\'
		)`, []interface{}{fts5Phrase(sanitized), like, like}, false
		}
	}
	return " AND (fileName LIKE ? ESCAPE '\\' OR srcPath LIKE ? ESCAPE '\\' OR dstPath LIKE ? ESCAPE '\\' OR alistTaskId LIKE ? ESCAPE '\\' OR errMsg LIKE ? ESCAPE '\\')",
		[]interface{}{like, like, like, like, like}, false
}

func taskItemFTSAvailable(ctx context.Context) bool {
	ok, _ := taskItemFTSCapabilities(ctx)
	return ok
}

func taskItemFTSCapabilities(ctx context.Context) (available bool, coversAll bool) {
	handle := GetDB()
	ftsAvailability.mu.Lock()
	if ftsAvailability.known && ftsAvailability.db == handle {
		ok := ftsAvailability.ok
		all := ftsAvailability.coversAll
		ftsAvailability.mu.Unlock()
		return ok, all
	}
	ftsAvailability.mu.Unlock()

	var name string
	err := handle.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='job_task_item_fts'").Scan(&name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, false
	}
	ok := err == nil && name == "job_task_item_fts"
	all := false
	if ok {
		hasAlistTaskID, errA := tableHasColumnContext(ctx, handle, "job_task_item_fts", "alistTaskId")
		hasErrMsg, errE := tableHasColumnContext(ctx, handle, "job_task_item_fts", "errMsg")
		if errA != nil || errE != nil {
			return false, false
		}
		all = hasAlistTaskID && hasErrMsg
	}

	ftsAvailability.mu.Lock()
	if GetDB() == handle {
		ftsAvailability.db = handle
		ftsAvailability.known = true
		ftsAvailability.ok = ok
		ftsAvailability.coversAll = all
	}
	ftsAvailability.mu.Unlock()
	return ok, all
}

func resetFTSAvailability() {
	ftsAvailability.mu.Lock()
	ftsAvailability.db = nil
	ftsAvailability.known = false
	ftsAvailability.ok = false
	ftsAvailability.coversAll = false
	ftsAvailability.mu.Unlock()
}

var ftsAvailability struct {
	mu        sync.Mutex
	db        *sql.DB
	known     bool
	ok        bool
	coversAll bool
}

func sanitizeFTS5Term(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0 || r < 0x20 || r == 0x7f || r == utf8.RuneError {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func fts5Phrase(s string) string {
	clean := sanitizeFTS5Term(s)
	return `"` + strings.ReplaceAll(clean, `"`, `""`) + `"`
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
