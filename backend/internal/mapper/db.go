package mapper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"opensync/internal/config"
	"opensync/internal/msg"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	db           *sql.DB
	dbMu         sync.RWMutex
	batchWriteMu sync.Mutex
)

const maxPageSize = 500
const defaultUnpagedLimit = 500
const sqliteMaxOpenConns = 12
const maxBusyRetries = 3

// maxFloat64BeforeInt64Overflow is 2^63, the exact float64 boundary where
// conversion to int64 overflows (and saturates to math.MaxInt64 on ARM64).
const maxFloat64BeforeInt64Overflow = float64(uint64(1) << 63)

// InitDB initializes the database connection.
func InitDB() *sql.DB {
	dbMu.RLock()
	if db != nil {
		handle := db
		dbMu.RUnlock()
		return handle
	}
	dbMu.RUnlock()

	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		return db
	}

	cfg := config.GetConfig()
	newDB, err := openConfiguredDB(cfg.DB.DBName)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	db = newDB
	resetFTSAvailability()
	return db
}

func openConfiguredDB(dbName string) (*sql.DB, error) {
	ensureSQLiteFileMode(dbName)
	newDB, err := sql.Open("sqlite", sqliteDSN(dbName))
	if err != nil {
		return nil, err
	}

	if isIsolatedMemoryDB(dbName) {
		// Isolated in-memory databases create a separate empty database per
		// connection, so keep the pool capped to a single connection.
		newDB.SetMaxOpenConns(1)
		newDB.SetMaxIdleConns(1)
	} else {
		newDB.SetMaxOpenConns(sqliteMaxOpenConns)
		newDB.SetMaxIdleConns(sqliteMaxOpenConns)
	}

	// Keep explicit PRAGMAs as a startup sanity pass on the local handle before
	// publishing it; sqliteDSN also applies them to every pooled connection.
	// Set auto_vacuum=INCREMENTAL before journal_mode=WAL so brand-new empty DBs
	// record incremental vacuum mode before page 1 is initialized.
	pragmas := []string{
		"PRAGMA auto_vacuum=INCREMENTAL",
		"PRAGMA busy_timeout=5000",
	}
	if !isMemoryDB(dbName) {
		pragmas = append(pragmas,
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=NORMAL",
		)
	}
	pragmas = append(pragmas,
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA cache_size=-16384",
		"PRAGMA mmap_size=67108864",
	)
	for _, pragma := range pragmas {
		if _, err := newDB.Exec(pragma); err != nil {
			log.Printf("Failed to set sqlite %s: %v", pragma, err)
		}
	}

	tightenSQLiteFilePermissions(dbName)
	return newDB, nil
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

func tightenSQLiteFilePermissions(dbName string) {
	path, ok := sqliteDBPath(dbName)
	if !ok {
		return
	}
	for _, target := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Chmod(target, 0600); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to tighten sqlite file permissions for %s: %v", target, err)
		}
	}
}

func isMemoryDB(dbName string) bool {
	if dbName == "" || dbName == ":memory:" {
		return true
	}
	if !strings.HasPrefix(dbName, "file:") {
		return false
	}
	u, err := url.Parse(dbName)
	if err != nil {
		return false
	}
	if u.Query().Get("mode") == "memory" {
		return true
	}
	return u.Path == ":memory:" || u.Opaque == ":memory:" || strings.HasPrefix(u.Opaque, ":memory:")
}

func isIsolatedMemoryDB(dbName string) bool {
	if !isMemoryDB(dbName) {
		return false
	}
	if !strings.HasPrefix(dbName, "file:") {
		return true
	}
	u, err := url.Parse(dbName)
	if err != nil {
		return true
	}
	return u.Query().Get("cache") != "shared"
}

func sqliteDBPath(dbName string) (string, bool) {
	if isMemoryDB(dbName) {
		return "", false
	}
	if strings.HasPrefix(dbName, "file:") {
		u, err := url.Parse(dbName)
		if err != nil {
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
	pragmas.Add("_pragma", "auto_vacuum(INCREMENTAL)")
	pragmas.Add("_pragma", "busy_timeout(5000)")
	if !isMemoryDB(dbName) {
		pragmas.Add("_pragma", "journal_mode(WAL)")
		pragmas.Add("_pragma", "synchronous(NORMAL)")
	}
	// Apply per connection, not just the first pooled connection: SQLite keeps
	// page cache, temp store and mmap size as connection-local settings.
	pragmas.Add("_pragma", "foreign_keys(ON)")
	pragmas.Add("_pragma", "temp_store(MEMORY)")
	pragmas.Add("_pragma", "cache_size(-16384)")
	pragmas.Add("_pragma", "mmap_size(67108864)")
	query := pragmas.Encode()
	if strings.HasPrefix(dbName, "file:") {
		sep := "?"
		if strings.Contains(dbName, "?") {
			sep = "&"
		}
		return dbName + sep + query
	}
	return "file:" + encodeSQLiteURIPath(dbName) + "?" + query
}

// encodeSQLiteURIPath percent-encodes characters that have special meaning in
// SQLite URI filenames ('%', '?', '#') so paths like "backup#1/openSync.db"
// open the intended database instead of truncating at '#' or '?'.
func encodeSQLiteURIPath(p string) string {
	p = filepath.ToSlash(p)
	if vol := filepath.VolumeName(p); vol != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.NewReplacer(
		"%", "%25",
		"?", "%3F",
		"#", "%23",
	).Replace(p)
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
	handle := db
	db = nil
	resetFTSAvailability()
	dbMu.Unlock()
	if handle == nil {
		return nil
	}
	return handle.Close()
}

// SetDBForTest swaps the package database handle and returns a restore function.
func SetDBForTest(testDB *sql.DB) func() {
	dbMu.Lock()
	oldDB := db
	db = testDB
	resetFTSAvailability()
	dbMu.Unlock()
	return func() {
		dbMu.Lock()
		db = oldDB
		resetFTSAvailability()
		dbMu.Unlock()
	}
}

// FetchAllToTable executes a query and returns results as []map[string]interface{}
func FetchAllToTable(query string, args ...interface{}) ([]map[string]interface{}, error) {
	return fetchAllToTableContext(context.Background(), query, args...)
}

func fetchAllToTableContext(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRowsToTable(rows)
}

func scanRowsToTable(rows *sql.Rows) ([]map[string]interface{}, error) {
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

func executeUpdateAffected(notFoundErr error, query string, args ...interface{}) error {
	result, err := GetDB().Exec(query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return notFoundErr
	}
	return nil
}

func isSQLiteBusyOrLocked(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	primary := sqliteErr.Code() & 0xff
	return primary == sqlite3.SQLITE_BUSY || primary == sqlite3.SQLITE_LOCKED
}

func retryOnBusy(ctx context.Context, fn func() error) error {
	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if attempt >= maxBusyRetries || !isSQLiteBusyOrLocked(err) {
			return err
		}
		delay := time.Duration(1<<attempt) * 100 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// runImmediateTx runs fn inside a BEGIN IMMEDIATE transaction on a dedicated
// connection so WAL write transactions acquire the write lock up front instead
// of upgrading from a read snapshot (which would hit SQLITE_BUSY_SNAPSHOT).
func runImmediateTx(ctx context.Context, handle *sql.DB, fn func(ctx context.Context, conn *sql.Conn) error) error {
	conn, err := handle.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if err := fn(ctx, conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

// withImmediateWriteTx serializes multi-statement write transactions across
// goroutines, runs BEGIN IMMEDIATE, and retries bounded times on busy/locked.
func withImmediateWriteTx(ctx context.Context, fn func(ctx context.Context, conn *sql.Conn) error) error {
	batchWriteMu.Lock()
	defer batchWriteMu.Unlock()
	return retryOnBusy(ctx, func() error {
		return runImmediateTx(ctx, GetDB(), fn)
	})
}

// ExecuteMany executes batch operations
func ExecuteMany(query string, argsList [][]interface{}) error {
	if len(argsList) == 0 {
		return nil
	}
	err := withImmediateWriteTx(context.Background(), func(ctx context.Context, conn *sql.Conn) error {
		stmt, err := conn.PrepareContext(ctx, query)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, args := range argsList {
			if _, err := stmt.ExecContext(ctx, args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Database batch execute failed (size %d): %v", len(argsList), err)
	}
	return err
}

// FetchAllToPage executes a paginated query.
//
// When the page does not reveal the total, a separate COUNT query is used rather than a
// COUNT(*) OVER() window column: the window forces SQLite to materialize the
// whole result set and sort it in a temp B-tree before applying LIMIT, which
// costs 5-40x more than the page query alone on large tables. Both queries run
// inside a single read-only transaction so rows and total share one snapshot.
func FetchAllToPage(baseSQL string, params map[string]interface{}, sqlArgs ...interface{}) (map[string]interface{}, error) {
	return fetchAllToPageContext(context.Background(), baseSQL, params, sqlArgs...)
}

func fetchAllToPageContext(ctx context.Context, baseSQL string, params map[string]interface{}, sqlArgs ...interface{}) (map[string]interface{}, error) {
	ps, pn, paginated, err := parsePageParams(params)
	if err != nil {
		return nil, err
	}
	if !paginated {
		return fetchPage(ctx, baseSQL, defaultUnpagedLimit, 0, false, sqlArgs)
	}
	offset, err := pageOffset(ps, pn)
	if err != nil {
		return nil, err
	}
	return fetchPage(ctx, baseSQL, ps, offset, true, sqlArgs)
}

func fetchPage(ctx context.Context, baseSQL string, limit int, offset int64, paginated bool, sqlArgs []interface{}) (map[string]interface{}, error) {
	tx, err := GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return fetchPageTx(ctx, tx, baseSQL, limit, offset, paginated, sqlArgs)
}

func fetchPageTx(ctx context.Context, tx *sql.Tx, baseSQL string, limit int, offset int64, paginated bool, sqlArgs []interface{}) (map[string]interface{}, error) {
	query, args := pageQuery(baseSQL, limit, offset, paginated, sqlArgs)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	dataList, scanErr := scanRowsToTable(rows)
	closeErr := rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr != nil {
		return nil, closeErr
	}

	var total int64
	if rowCount := len(dataList); rowCount < limit && (rowCount > 0 || offset == 0) {
		total = offset + int64(rowCount)
	} else {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+stripOrderBy(baseSQL)+")", sqlArgs...).Scan(&total); err != nil {
			return nil, err
		}
	}
	result := map[string]interface{}{"dataList": dataList, "count": total}
	if !paginated && total > int64(len(dataList)) {
		result["truncated"] = true
	}
	return result, nil
}

func pageQuery(baseSQL string, limit int, offset int64, paginated bool, sqlArgs []interface{}) (string, []interface{}) {
	if paginated {
		return baseSQL + " LIMIT ? OFFSET ?", appendSQLArgs(sqlArgs, limit, offset)
	}
	return baseSQL + " LIMIT ?", appendSQLArgs(sqlArgs, limit)
}

func appendSQLArgs(args []interface{}, extra ...interface{}) []interface{} {
	result := make([]interface{}, 0, len(args)+len(extra))
	result = append(result, args...)
	return append(result, extra...)
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
		if math.IsNaN(val) || math.IsInf(val, 0) || math.Trunc(val) != val {
			return 0, errors.New(msg.LostPart)
		}
		if val <= 0 || val >= maxFloat64BeforeInt64Overflow {
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
