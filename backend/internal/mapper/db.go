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

// pageTotalColumn is injected via COUNT(*) OVER() so list + total share one
// SQLite round-trip. The name is reserved for mapper pagination and stripped
// before rows leave FetchAllToPage.
const pageTotalColumn = "__opensync_page_total"

func pageOffset(pageSize, pageNum int) (int64, error) {
	if pageSize <= 0 || pageNum <= 0 {
		return 0, errors.New(msg.LostPart)
	}
	// Keep the multiplication in int64 and reject values that cannot be
	// represented by SQLite's signed integer offset instead of allowing an int
	// overflow to turn a large page number into a negative OFFSET.
	pageIndex := int64(pageNum) - 1
	size := int64(pageSize)
	maxInt64 := int64(^uint64(0) >> 1)
	if pageIndex > maxInt64/size {
		return 0, errors.New(msg.LostPart)
	}
	return pageIndex * size, nil
}

// InitDB initializes the database connection
func InitDB() *sql.DB {
	once.Do(func() {
		cfg := config.GetConfig()
		var err error
		ensureSQLiteFileMode(cfg.DB.DBName)
		db, err = sql.Open("sqlite", sqliteDSN(cfg.DB))
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
		if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
			log.Printf("Failed to enable sqlite foreign keys: %v", err)
		}
		if _, err := db.Exec("PRAGMA temp_store=MEMORY"); err != nil {
			log.Printf("Failed to set sqlite temp_store: %v", err)
		}
		if _, err := db.Exec("PRAGMA cache_size=-16384"); err != nil {
			log.Printf("Failed to set sqlite cache_size: %v", err)
		}
		if _, err := db.Exec("PRAGMA mmap_size=67108864"); err != nil {
			log.Printf("Failed to set sqlite mmap_size: %v", err)
		}
		if mode := normalizeSqliteSync(cfg.DB.SqliteSync); mode != "" {
			if _, err := db.Exec("PRAGMA synchronous=" + mode); err != nil {
				log.Printf("Failed to set sqlite synchronous mode: %v", err)
			}
		}
	})
	dbMu.RLock()
	defer dbMu.RUnlock()
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

// normalizeSqliteSync keeps the sqlite synchronous mode within the three valid
// values, returning empty for anything unrecognized so the DSN omits it (the
// driver default is then used).
func normalizeSqliteSync(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "full", "normal", "off":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return ""
	}
}

func sqliteDSN(cfg config.DBConfig) string {
	dbName := cfg.DBName
	if dbName == ":memory:" {
		return dbName
	}
	pragmas := url.Values{}
	pragmas.Add("_pragma", "busy_timeout(5000)")
	pragmas.Add("_pragma", "journal_mode(WAL)")
	pragmas.Add("_pragma", "foreign_keys(ON)")
	pragmas.Add("_pragma", "temp_store(MEMORY)")
	pragmas.Add("_pragma", "cache_size(-16384)")  // 16 MiB page cache
	pragmas.Add("_pragma", "mmap_size(67108864)") // 64 MiB mmap; bounds RSS on small NAS boxes
	// WAL + NORMAL is the default (see config.defaultSqliteSync). FULL fsyncs
	// every commit and is the dominant I/O cost on NAS disks; OFF is opt-in.
	// Defensively normalize even though GetConfig already canonicalizes, so a
	// test or future caller passing a raw value cannot emit a bad PRAGMA.
	if mode := normalizeSqliteSync(cfg.SqliteSync); mode != "" {
		pragmas.Add("_pragma", "synchronous("+mode+")")
	}
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

	n := len(columns)
	values := make([]interface{}, n)
	valuePtrs := make([]interface{}, n)
	for i := range valuePtrs {
		valuePtrs[i] = &values[i]
	}

	results := make([]map[string]interface{}, 0, 16)
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

// FetchAllToPage executes a paginated query in a single SQLite round-trip
// (COUNT(*) OVER() + bound LIMIT/OFFSET). A separate COUNT is only used when
// the page is empty past the first page, or the SQL cannot take a window
// (UNION), so list and total stay consistent under WAL writers.
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

func fetchPage(baseSQL string, limit int, offset int64, paginated bool, sqlArgs []interface{}) (map[string]interface{}, error) {
	query, args, _ := pageQuery(baseSQL, limit, offset, paginated, sqlArgs)
	dataList, err := FetchAllToTable(query, args...)
	if err != nil {
		return nil, err
	}

	total, hasTotal := takePageTotal(dataList)
	if !hasTotal {
		if len(dataList) == 0 && offset == 0 {
			total = 0
		} else {
			count, err := FetchFirstVal(countQueryFromSelect(baseSQL), sqlArgs...)
			if err != nil {
				return nil, err
			}
			total = util.ToInt64(count)
		}
	}

	result := map[string]interface{}{
		"dataList": dataList,
		"count":    total,
	}
	if !paginated && total > int64(len(dataList)) {
		result["truncated"] = true
	}
	return result, nil
}

func pageQuery(baseSQL string, limit int, offset int64, paginated bool, sqlArgs []interface{}) (string, []interface{}, bool) {
	if canUseWindowCount(baseSQL) {
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

func appendSQLArgs(sqlArgs []interface{}, extra ...interface{}) []interface{} {
	args := make([]interface{}, 0, len(sqlArgs)+len(extra))
	args = append(args, sqlArgs...)
	return append(args, extra...)
}

func canUseWindowCount(baseSQL string) bool {
	return !strings.Contains(strings.ToUpper(baseSQL), " UNION ")
}

func withPageTotal(baseSQL string) string {
	sql := strings.TrimSpace(baseSQL)
	if len(sql) < 7 || !strings.EqualFold(sql[:6], "SELECT") || sql[6] != ' ' && sql[6] != '\t' && sql[6] != '\n' {
		return sql
	}
	return "SELECT COUNT(*) OVER() AS " + pageTotalColumn + "," + sql[6:]
}

func takePageTotal(rows []map[string]interface{}) (int64, bool) {
	if len(rows) == 0 {
		return 0, false
	}
	var total int64
	ok := false
	for _, row := range rows {
		if v, exists := row[pageTotalColumn]; exists {
			total = util.ToInt64(v)
			delete(row, pageTotalColumn)
			ok = true
		}
	}
	return total, ok
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
	// The offset is emitted as a SQLite integer. Validate the full page
	// calculation once here so all mapper callers get the same behavior.
	if _, err := pageOffset(pageSize, pageNum); err != nil {
		return 0, 0, false, err
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

// countQueryFromSelect rewrites `SELECT cols FROM t WHERE ... ORDER BY ...`
// into `SELECT COUNT(*) FROM t WHERE ...` so SQLite can satisfy the count
// from covering indexes instead of materializing the select-list subquery.
func countQueryFromSelect(baseSQL string) string {
	sql := strings.TrimSpace(stripOrderBy(baseSQL))
	upper := strings.ToUpper(sql)
	if strings.Contains(upper, " DISTINCT ") ||
		strings.Contains(upper, " GROUP BY ") ||
		strings.Contains(upper, " UNION ") {
		return "SELECT COUNT(*) FROM (" + sql + ")"
	}
	fromIdx := strings.Index(upper, " FROM ")
	if fromIdx == -1 {
		return "SELECT COUNT(*) FROM (" + sql + ")"
	}
	return "SELECT COUNT(*)" + sql[fromIdx:]
}
