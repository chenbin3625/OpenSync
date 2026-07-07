package mapper

import (
	_ "modernc.org/sqlite"
	"opensync/internal/config"
	"path/filepath"
	"testing"
)

// Simulate a fresh DB created by InitSQL, then restarted before any user exists.
func TestMigrateDBTxSkipsAlreadyAppliedFreshSchemaColumns(t *testing.T) {
	resetGlobalDBForTest(t, &config.Config{
		DB:     config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db")},
		Server: config.ServerConfig{PasswdStr: "x"},
	})
	InitSQL()

	err := migrateDBTx(GetDB(), 0)
	if err != nil {
		t.Fatalf("migrateDBTx(0) on fresh-empty DB error = %v, want nil", err)
	}
}
