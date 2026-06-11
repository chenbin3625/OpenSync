package service

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/pkg/crypto"

	_ "modernc.org/sqlite"
)

func TestCheckPwdUpgradesLegacyMD5PasswordHash(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE user_list(
		id integer primary key autoincrement,
		userName text,
		passwd text,
		sqlVersion integer,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	legacyHash := crypto.PasswordToMD5("old-password", "test-secret")
	if _, err := testDB.Exec(
		"INSERT INTO user_list(id, userName, passwd) VALUES (1, 'admin', ?)",
		legacyHash,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{PasswdStr: "test-secret"},
	})
	defer config.SetConfigForTest(oldConfig)
	errPwdMu.Lock()
	errPwd = nil
	errPwdMu.Unlock()

	CheckPwd(0, "old-password", "admin")

	var upgradedHash string
	if err := testDB.QueryRow("SELECT passwd FROM user_list WHERE id=1").Scan(&upgradedHash); err != nil {
		t.Fatalf("read passwd: %v", err)
	}
	if upgradedHash == legacyHash {
		t.Fatalf("password hash was not upgraded")
	}
	if !strings.HasPrefix(upgradedHash, "$2") {
		t.Fatalf("upgraded hash = %q, want bcrypt hash", upgradedHash)
	}
	if !crypto.CheckPassword("old-password", upgradedHash, "test-secret") {
		t.Fatalf("upgraded hash does not verify original password")
	}
}

func TestPasswordErrorScopesAreBounded(t *testing.T) {
	errPwdMu.Lock()
	errPwd = nil
	errPwdMu.Unlock()

	for i := 0; i < maxPwdErrorScopes+50; i++ {
		AddPwdErrorForScope(fmt.Sprintf("scope-%d", i))
	}

	errPwdMu.Lock()
	got := len(errPwd)
	errPwdMu.Unlock()

	if got > maxPwdErrorScopes {
		t.Fatalf("tracked password error scopes = %d, want <= %d", got, maxPwdErrorScopes)
	}
}
