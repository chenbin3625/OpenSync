package mapper

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"opensync/internal/config"
	"opensync/internal/i18n"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	once sync.Once
)

const maxPageSize = 500

// InitDB initializes the database connection
func InitDB() *sql.DB {
	once.Do(func() {
		cfg := config.GetConfig()
		var err error
		db, err = sql.Open("sqlite", cfg.DB.DBName)
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		db.SetMaxOpenConns(1) // SQLite single writer
		// Enable WAL mode
		db.Exec("PRAGMA journal_mode=WAL")
		db.Exec("PRAGMA busy_timeout=5000")
	})
	return db
}

// GetDB returns the database connection
func GetDB() *sql.DB {
	if db == nil {
		return InitDB()
	}
	return db
}

// FetchAllToTable executes a query and returns results as []map[string]interface{}
func FetchAllToTable(query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := GetDB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// FetchFirstVal executes a query and returns the first column of first row
func FetchFirstVal(query string, args ...interface{}) (interface{}, error) {
	var result interface{}
	err := GetDB().QueryRow(query, args...).Scan(&result)
	return result, err
}

// ExecuteInsert executes an insert and returns last insert id
func ExecuteInsert(query string, args ...interface{}) (int64, error) {
	result, err := GetDB().Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// ExecuteUpdate executes an update/delete query
func ExecuteUpdate(query string, args ...interface{}) error {
	_, err := GetDB().Exec(query, args...)
	return err
}

// ExecuteMany executes batch operations
func ExecuteMany(query string, argsList [][]interface{}) error {
	tx, err := GetDB().Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(query)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, args := range argsList {
		_, err := stmt.Exec(args...)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// FetchAllToPage executes a paginated query
func FetchAllToPage(baseSQL string, params map[string]interface{}, sqlArgs ...interface{}) (map[string]interface{}, error) {
	ps, pn, paginated, err := parsePageParams(params)
	if err != nil {
		return nil, err
	}
	if !paginated {
		dataList, err := FetchAllToTable(baseSQL, sqlArgs...)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"dataList": dataList,
			"count":    len(dataList),
		}, nil
	}

	offset := (pn - 1) * ps

	dataQuery := baseSQL + fmt.Sprintf(" LIMIT %d OFFSET %d", ps, offset)
	dataList, err := FetchAllToTable(dataQuery, sqlArgs...)
	if err != nil {
		return nil, err
	}

	countQuery := "SELECT COUNT(*) FROM (" + stripOrderBy(baseSQL) + ")"
	count, err := FetchFirstVal(countQuery, sqlArgs...)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"dataList": dataList,
		"count":    toInt64(count),
	}, nil
}

func parsePageParams(params map[string]interface{}) (pageSize, pageNum int, paginated bool, err error) {
	pageSizeVal, hasPageSize := params["pageSize"]
	pageNumVal, hasPageNum := params["pageNum"]
	if !hasPageSize && !hasPageNum {
		return 0, 0, false, nil
	}
	if !hasPageSize || !hasPageNum {
		return 0, 0, false, errors.New(i18n.G("lost_part"))
	}

	pageSize, err = positiveInt(pageSizeVal)
	if err != nil {
		return 0, 0, false, errors.New(i18n.G("lost_part"))
	}
	pageNum, err = positiveInt(pageNumVal)
	if err != nil {
		return 0, 0, false, errors.New(i18n.G("lost_part"))
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return pageSize, pageNum, true, nil
}

func positiveInt(v interface{}) (int, error) {
	var n int64
	switch val := v.(type) {
	case int:
		n = int64(val)
	case int64:
		n = val
	case float64:
		if math.Trunc(val) != val {
			return 0, errors.New(i18n.G("lost_part"))
		}
		n = int64(val)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return 0, err
		}
		n = parsed
	default:
		return 0, errors.New(i18n.G("lost_part"))
	}
	if n <= 0 || n > int64(math.MaxInt) {
		return 0, errors.New(i18n.G("lost_part"))
	}
	return int(n), nil
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	default:
		return 0
	}
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	default:
		return 0
	}
}

// CheckAndAddSQL builds dynamic update SQL from params
func CheckAndAddSQL(baseSQL string, params []string, data map[string]interface{}) (string, []interface{}, error) {
	var setClauses []string
	var args []interface{}
	flag := 0
	for _, item := range params {
		if v, ok := data[item]; ok {
			setClauses = append(setClauses, fmt.Sprintf("%s=?", item))
			args = append(args, v)
			flag++
		}
	}
	if flag == 0 {
		return "", nil, errors.New(i18n.G("lost_part"))
	}
	if _, ok := data["id"]; !ok {
		return "", nil, errors.New(i18n.G("lost_part"))
	}
	sql := baseSQL + " " + joinStrings(setClauses, ", ") + " WHERE id=?"
	args = append(args, data["id"])
	return sql, args, nil
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

func stripOrderBy(sql string) string {
	upperSQL := strings.ToUpper(sql)
	idx := strings.LastIndex(upperSQL, " ORDER BY ")
	if idx == -1 {
		return sql
	}
	return sql[:idx]
}
