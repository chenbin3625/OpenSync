package mapper

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"opensync/internal/config"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	once = &sync.Once{}
	dbMu sync.RWMutex
)

const maxPageSize = 500
const defaultUnpagedLimit = 500
const sqliteMaxOpenConns = 12

// InitDB initializes the database connection
func InitDB() *sql.DB {
	once.Do(func() {
		cfg := config.GetConfig()
		var err error
		ensureSQLiteFileMode(cfg.DB.DBName)
		db, err = sql.Open("sqlite", sqliteDSN(cfg.DB.DBName))
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		db.SetMaxOpenConns(sqliteMaxOpenConns)
		db.SetMaxIdleConns(sqliteMaxOpenConns)
		// Keep explicit PRAGMAs as a startup sanity pass; sqliteDSN applies
		// them to each new pooled connection.
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			log.Printf("Failed to set sqlite journal_mode: %v", err)
		}
		if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
			log.Printf("Failed to set sqlite busy_timeout: %v", err)
		}
		for _, pragma := range []string{"PRAGMA foreign_keys=ON", "PRAGMA temp_store=MEMORY", "PRAGMA cache_size=-16384", "PRAGMA mmap_size=67108864"} {
			if _, err := db.Exec(pragma); err != nil {
				log.Printf("Failed to set sqlite %s: %v", pragma, err)
			}
		}
	})
	return db
}

func ensureSQLiteFileMode(dbName string) {
	path, ok := sqliteDBPath(dbName)
	if !ok {
		return
	}
	if dir := filepath.Dir(path); dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		log.Printf("Failed to prepare sqlite database file permissions: %v", err)
		return
	}
	_ = file.Close()
	if err := os.Chmod(path, 0600); err != nil {
		log.Printf("Failed to set sqlite database file permissions: %v", err)
	}
}

func sqliteDBPath(dbName string) (string, bool) {
	if dbName == "" || dbName == ":memory:" {
		return "", false
	}
	if strings.HasPrefix(dbName, "file:") {
		u, err := url.Parse(dbName)
		if err != nil || strings.Contains(u.RawQuery, "mode=memory") {
			return "", false
		}
		if u.Path != "" {
			return u.Path, true
		}
		if u.Opaque != "" && !strings.HasPrefix(u.Opaque, ":memory:") {
			return u.Opaque, true
		}
		return "", false
	}
	return dbName, true
}

func sqliteDSN(dbName string) string {
	if dbName == ":memory:" {
		return dbName
	}
	pragmas := url.Values{}
	pragmas.Add("_pragma", "busy_timeout(5000)")
	pragmas.Add("_pragma", "journal_mode(WAL)")
	query := pragmas.Encode()
	if strings.HasPrefix(dbName, "file:") {
		sep := "?"
		if strings.Contains(dbName, "?") {
			sep = "&"
		}
		return dbName + sep + query
	}
	return "file:" + dbName + "?" + query
}

// GetDB returns the database connection
func GetDB() *sql.DB {
	dbMu.RLock()
	if db != nil {
		handle := db
		dbMu.RUnlock()
		return handle
	}
	dbMu.RUnlock()
	return InitDB()
}

// CloseDB closes the global database handle and allows later reinitialization.
func CloseDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	once = &sync.Once{}
	return err
}

// SetDBForTest swaps the package database handle and returns a restore function.
func SetDBForTest(testDB *sql.DB) func() {
	dbMu.Lock()
	oldDB := db
	db = testDB
	dbMu.Unlock()
	return func() {
		dbMu.Lock()
		db = oldDB
		dbMu.Unlock()
	}
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

	results := make([]map[string]interface{}, 0, 16)
	n := len(columns)
	values := make([]interface{}, n)
	valuePtrs := make([]interface{}, n)
	for i := range valuePtrs {
		valuePtrs[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, n)
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
	if len(argsList) == 0 {
		return nil
	}
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
			log.Printf("Database batch execute failed (size %d): %v", len(argsList), err)
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("Database batch commit failed (size %d): %v", len(argsList), err)
		return err
	}
	return nil
}

// FetchAllToPage executes a paginated query with a window count when SQLite
// can keep the list and total in one round-trip.
func FetchAllToPage(baseSQL string, params map[string]interface{}, sqlArgs ...interface{}) (map[string]interface{}, error) {
	ps, pn, paginated, err := parsePageParams(params)
	if err != nil {
		return nil, err
	}
	if !paginated {
		return fetchPage(baseSQL, defaultUnpagedLimit, 0, false, sqlArgs)
	}
	offset, err := pageOffset(ps, pn)
	if err != nil {
		return nil, err
	}
	return fetchPage(baseSQL, ps, offset, true, sqlArgs)
}

const pageTotalColumn = "__opensync_page_total"

func fetchPage(baseSQL string, limit int, offset int64, paginated bool, sqlArgs []interface{}) (map[string]interface{}, error) {
	query, args, window := pageQuery(baseSQL, limit, offset, paginated, sqlArgs)
	dataList, err := FetchAllToTable(query, args...)
	if err != nil {
		return nil, err
	}
	total, hasTotal := takePageTotal(dataList)
	if !hasTotal {
		if len(dataList) == 0 && offset == 0 {
			total = 0
		} else {
			count, err := FetchFirstVal("SELECT COUNT(*) FROM ("+stripOrderBy(baseSQL)+")", sqlArgs...)
			if err != nil {
				return nil, err
			}
			total = util.ToInt64(count)
		}
	}
	result := map[string]interface{}{"dataList": dataList, "count": total}
	if !paginated && window && total > int64(len(dataList)) {
		result["truncated"] = true
	}
	return result, nil
}

func pageQuery(baseSQL string, limit int, offset int64, paginated bool, sqlArgs []interface{}) (string, []interface{}, bool) {
	if !strings.Contains(strings.ToUpper(baseSQL), " UNION ") {
		query := withPageTotal(baseSQL)
		if paginated {
			return query + " LIMIT ? OFFSET ?", appendSQLArgs(sqlArgs, limit, offset), true
		}
		return query + " LIMIT ?", appendSQLArgs(sqlArgs, limit), true
	}
	if paginated {
		return baseSQL + " LIMIT ? OFFSET ?", appendSQLArgs(sqlArgs, limit, offset), false
	}
	return baseSQL + " LIMIT ?", appendSQLArgs(sqlArgs, limit), false
}

func appendSQLArgs(args []interface{}, extra ...interface{}) []interface{} {
	result := make([]interface{}, 0, len(args)+len(extra))
	result = append(result, args...)
	return append(result, extra...)
}

func withPageTotal(baseSQL string) string {
	trimmed := strings.TrimSpace(baseSQL)
	if len(trimmed) < 7 || !strings.EqualFold(trimmed[:6], "SELECT") || (trimmed[6] != ' ' && trimmed[6] != '\t' && trimmed[6] != '\n') {
		return trimmed
	}
	return "SELECT COUNT(*) OVER() AS " + pageTotalColumn + "," + trimmed[6:]
}

func takePageTotal(rows []map[string]interface{}) (int64, bool) {
	if len(rows) == 0 {
		return 0, false
	}
	var total int64
	found := false
	for _, row := range rows {
		if value, ok := row[pageTotalColumn]; ok {
			total = util.ToInt64(value)
			delete(row, pageTotalColumn)
			found = true
		}
	}
	return total, found
}

func pageOffset(pageSize, pageNum int) (int64, error) {
	if pageSize <= 0 || pageNum <= 0 {
		return 0, errors.New(msg.LostPart)
	}
	index := int64(pageNum) - 1
	size := int64(pageSize)
	maxInt64 := int64(^uint64(0) >> 1)
	if index > maxInt64/size {
		return 0, errors.New(msg.LostPart)
	}
	return index * size, nil
}

func withDefaultLimit(baseSQL string) string {
	return baseSQL + fmt.Sprintf(" LIMIT %d", defaultUnpagedLimit)
}

func parsePageParams(params map[string]interface{}) (pageSize, pageNum int, paginated bool, err error) {
	pageSizeVal, hasPageSize := params["pageSize"]
	pageNumVal, hasPageNum := params["pageNum"]
	if !hasPageSize && !hasPageNum {
		return 0, 0, false, nil
	}
	if !hasPageSize || !hasPageNum {
		return 0, 0, false, errors.New(msg.LostPart)
	}

	pageSize, err = positiveInt(pageSizeVal)
	if err != nil {
		return 0, 0, false, errors.New(msg.LostPart)
	}
	pageNum, err = positiveInt(pageNumVal)
	if err != nil {
		return 0, 0, false, errors.New(msg.LostPart)
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
			return 0, errors.New(msg.LostPart)
		}
		n = int64(val)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			return 0, err
		}
		n = parsed
	default:
		return 0, errors.New(msg.LostPart)
	}
	if n <= 0 || n > int64(math.MaxInt) {
		return 0, errors.New(msg.LostPart)
	}
	return int(n), nil
}

// CheckAndAddSQL builds dynamic update SQL from params
func CheckAndAddSQL(baseSQL string, params []string, data map[string]interface{}) (string, []interface{}, error) {
	var setClauses []string
	var args []interface{}
	flag := 0
	for _, item := range params {
		if !isSafeSQLIdentifier(item) {
			return "", nil, errors.New(msg.LostPart)
		}
		if v, ok := data[item]; ok {
			setClauses = append(setClauses, fmt.Sprintf("%s=?", item))
			args = append(args, v)
			flag++
		}
	}
	if flag == 0 {
		return "", nil, errors.New(msg.LostPart)
	}
	if _, ok := data["id"]; !ok {
		return "", nil, errors.New(msg.LostPart)
	}
	sql := baseSQL + " " + strings.Join(setClauses, ", ") + " WHERE id=?"
	args = append(args, data["id"])
	return sql, args, nil
}

func isSafeSQLIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func stripOrderBy(sql string) string {
	upperSQL := strings.ToUpper(sql)
	idx := strings.LastIndex(upperSQL, " ORDER BY ")
	if idx == -1 {
		return sql
	}
	return sql[:idx]
}
