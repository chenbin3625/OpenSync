package service

import (
	"database/sql"
	"fmt"
	"testing"

	"opensync/internal/mapper"
	"opensync/pkg/crypto"

	_ "modernc.org/sqlite"
)

func createUserTableForTest(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key autoincrement,
		userName text,
		passwd text,
		recoveryKey text,
		sqlVersion integer,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
}

func requirePublicPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("expected panic")
		}
	}()
	fn()
}

func TestInitializeUserCreatesCustomFirstUser(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	createUserTableForTest(t, testDB)

	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	if IsInitialized() {
		t.Fatalf("IsInitialized() = true, want false before first user is created")
	}

	user, recoveryKey := InitializeUser(" owner ", "secret-password")
	if got := user["userName"]; got != "owner" {
		t.Fatalf("created userName = %v, want trimmed owner", got)
	}
	if len(recoveryKey) != 24 {
		t.Fatalf("recovery key length = %d, want 24", len(recoveryKey))
	}
	if !IsInitialized() {
		t.Fatalf("IsInitialized() = false, want true after first user is created")
	}

	var storedHash, storedRecoveryHash string
	if err := testDB.QueryRow("SELECT passwd, recoveryKey FROM user_list WHERE userName='owner'").Scan(&storedHash, &storedRecoveryHash); err != nil {
		t.Fatalf("read stored hashes: %v", err)
	}
	if storedHash == "secret-password" {
		t.Fatalf("stored password was not hashed")
	}
	if !crypto.CheckPassword("secret-password", storedHash) {
		t.Fatalf("stored password hash does not verify custom password")
	}
	if storedRecoveryHash == "" || storedRecoveryHash == recoveryKey {
		t.Fatalf("stored recovery key hash was not protected")
	}
	if !crypto.CheckPassword(recoveryKey, storedRecoveryHash) {
		t.Fatalf("stored recovery key hash does not verify generated recovery key")
	}
}

func TestInitializeUserRejectsExistingUser(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	createUserTableForTest(t, testDB)
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, recoveryKey) VALUES ('admin', 'hash', 'recovery-hash')"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("InitializeUser() did not panic for an already initialized system")
		}
		var count int
		if err := testDB.QueryRow("SELECT COUNT(*) FROM user_list").Scan(&count); err != nil {
			t.Fatalf("count users: %v", err)
		}
		if count != 1 {
			t.Fatalf("user count = %d, want unchanged 1", count)
		}
	}()

	InitializeUser("other", "other-password")
}

func TestResetPasswdWithRecoveryKeyRotatesRecoveryKey(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	createUserTableForTest(t, testDB)
	passwordHash, err := crypto.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword(old-password) error: %v", err)
	}
	recoveryHash, err := crypto.HashPassword("original-recovery-key")
	if err != nil {
		t.Fatalf("HashPassword(original-recovery-key) error: %v", err)
	}
	if _, err := testDB.Exec(
		"INSERT INTO user_list(id, userName, passwd, recoveryKey) VALUES (1, 'admin', ?, ?)",
		passwordHash,
		recoveryHash,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	newRecoveryKey := ResetPasswd("admin", "original-recovery-key", "new-password")
	if len(newRecoveryKey) != 24 {
		t.Fatalf("new recovery key length = %d, want 24", len(newRecoveryKey))
	}
	if newRecoveryKey == "original-recovery-key" {
		t.Fatalf("new recovery key reused old key")
	}

	var storedPasswordHash, storedRecoveryHash string
	if err := testDB.QueryRow("SELECT passwd, recoveryKey FROM user_list WHERE id=1").Scan(&storedPasswordHash, &storedRecoveryHash); err != nil {
		t.Fatalf("read updated hashes: %v", err)
	}
	if !crypto.CheckPassword("new-password", storedPasswordHash) {
		t.Fatalf("updated password hash does not verify new password")
	}
	if crypto.CheckPassword("old-password", storedPasswordHash) {
		t.Fatalf("updated password hash still verifies old password")
	}
	if !crypto.CheckPassword(newRecoveryKey, storedRecoveryHash) {
		t.Fatalf("updated recovery hash does not verify new recovery key")
	}
	if crypto.CheckPassword("original-recovery-key", storedRecoveryHash) {
		t.Fatalf("updated recovery hash still verifies old recovery key")
	}
}

func TestResetPasswdRejectsWrongRecoveryKey(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	createUserTableForTest(t, testDB)
	passwordHash, err := crypto.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword(old-password) error: %v", err)
	}
	recoveryHash, err := crypto.HashPassword("right-recovery-key")
	if err != nil {
		t.Fatalf("HashPassword(right-recovery-key) error: %v", err)
	}
	if _, err := testDB.Exec(
		"INSERT INTO user_list(id, userName, passwd, recoveryKey) VALUES (1, 'admin', ?, ?)",
		passwordHash,
		recoveryHash,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	requirePublicPanic(t, func() {
		ResetPasswd("admin", "wrong-recovery-key", "new-password")
	})

	var storedPasswordHash, storedRecoveryHash string
	if err := testDB.QueryRow("SELECT passwd, recoveryKey FROM user_list WHERE id=1").Scan(&storedPasswordHash, &storedRecoveryHash); err != nil {
		t.Fatalf("read hashes: %v", err)
	}
	if !crypto.CheckPassword("old-password", storedPasswordHash) {
		t.Fatalf("password hash changed after rejected recovery key")
	}
	if !crypto.CheckPassword("right-recovery-key", storedRecoveryHash) {
		t.Fatalf("recovery hash changed after rejected recovery key")
	}
}

func TestResetPasswdForCLIGeneratesNewPasswordAndRecoveryKey(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	createUserTableForTest(t, testDB)
	passwordHash, err := crypto.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword(old-password) error: %v", err)
	}
	recoveryHash, err := crypto.HashPassword("old-recovery-key")
	if err != nil {
		t.Fatalf("HashPassword(old-recovery-key) error: %v", err)
	}
	if _, err := testDB.Exec(
		"INSERT INTO user_list(id, userName, passwd, recoveryKey) VALUES (1, 'admin', ?, ?)",
		passwordHash,
		recoveryHash,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	newPassword, newRecoveryKey := ResetPasswdForCLI("admin")
	if newPassword == "" {
		t.Fatalf("ResetPasswdForCLI returned empty password")
	}
	if len(newRecoveryKey) != 24 {
		t.Fatalf("new recovery key length = %d, want 24", len(newRecoveryKey))
	}

	var storedPasswordHash, storedRecoveryHash string
	if err := testDB.QueryRow("SELECT passwd, recoveryKey FROM user_list WHERE id=1").Scan(&storedPasswordHash, &storedRecoveryHash); err != nil {
		t.Fatalf("read updated hashes: %v", err)
	}
	if !crypto.CheckPassword(newPassword, storedPasswordHash) {
		t.Fatalf("stored password hash does not verify CLI-generated password")
	}
	if !crypto.CheckPassword(newRecoveryKey, storedRecoveryHash) {
		t.Fatalf("stored recovery hash does not verify CLI-generated recovery key")
	}
	if crypto.CheckPassword("old-recovery-key", storedRecoveryHash) {
		t.Fatalf("stored recovery hash still verifies old recovery key")
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
