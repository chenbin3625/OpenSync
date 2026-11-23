package service

import (
	"database/sql"
	"fmt"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/pkg/crypto"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func resetPwdTrackingForTest() {
	errPwdMu.Lock()
	errPwd = nil
	errPwdByIP = nil
	inFlightPwd = nil
	inFlightPwdByIP = nil
	errPwdMu.Unlock()
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

func TestInitializeUserAllowsOnlyOneConcurrentFirstUser(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()
	testDB.SetMaxOpenConns(1)

	createUserTableForTest(t, testDB)
	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	const workers = 8
	var successes atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			defer func() {
				_ = recover()
			}()
			InitializeUser(fmt.Sprintf("admin-%d", i), "secret-password")
			successes.Add(1)
		}()
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("successful initializations = %d, want 1", got)
	}
	var count int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM user_list").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("user count = %d, want 1", count)
	}
}

func TestResetPasswdWithRecoveryKeyRotatesRecoveryKey(t *testing.T) {
	resetPwdTrackingForTest()
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
	resetPwdTrackingForTest()
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
	resetPwdTrackingForTest()

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

func TestLockedScopeNotEvictedByFloodAndIPLockoutTriggers(t *testing.T) {
	resetPwdTrackingForTest()
	t.Cleanup(resetPwdTrackingForTest)

	lockedScope := passwordErrorScope(0, "admin", "198.51.100.1")
	for i := 0; i < pwdErrorLockThreshold; i++ {
		AddPwdErrorForScope(lockedScope)
	}

	// Flood more than maxPwdErrorScopes unlocked scopes; lockedScope must NEVER be evicted.
	for i := 0; i < maxPwdErrorScopes+100; i++ {
		AddPwdErrorForScope(fmt.Sprintf("random-%d|203.0.113.%d", i, i%250))
	}

	var panicVal interface{}
	func() {
		defer func() { panicVal = recover() }()
		CheckPwdTimeForScope(lockedScope)
	}()
	if pub, ok := panicVal.(model.PublicError); !ok || string(pub) != msg.PasswdWrongMaxTime {
		t.Fatalf("lockedScope after flood panic = %#v, want PublicError(%q)", panicVal, msg.PasswdWrongMaxTime)
	}

	// Flooding nonexistent usernames from a single IP must lock out that IP after ipPwdErrorLockThreshold failures.
	origGetByName := getUserByName
	getUserByName = func(name string) (map[string]interface{}, error) {
		return nil, mapper.ErrUserNotFound
	}
	t.Cleanup(func() { getUserByName = origGetByName })

	floodIP := "198.51.100.99"
	for i := 0; i < ipPwdErrorLockThreshold; i++ {
		var p interface{}
		func() {
			defer func() { p = recover() }()
			CheckPwdScoped(0, "wrong-pass", fmt.Sprintf("nouser-%d", i), floodIP)
		}()
		if pub, ok := p.(model.PublicError); !ok || string(pub) != msg.PasswdWrong {
			t.Fatalf("attempt %d panic = %#v, want PasswdWrong", i+1, p)
		}
	}

	var lockedPanic interface{}
	func() {
		defer func() { lockedPanic = recover() }()
		CheckPwdScoped(0, "wrong-pass", "brand-new-user", floodIP)
	}()
	if pub, ok := lockedPanic.(model.PublicError); !ok || string(pub) != msg.PasswdWrongMaxTime {
		t.Fatalf("IP lockout panic = %#v, want PasswdWrongMaxTime", lockedPanic)
	}
}

func TestCheckPwdScopedPreventsConcurrentTOCTOUBurst(t *testing.T) {
	resetPwdTrackingForTest()
	t.Cleanup(resetPwdTrackingForTest)

	hash, err := crypto.HashPassword("real-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	var lookups atomic.Int64
	origGetByName := getUserByName
	getUserByName = func(name string) (map[string]interface{}, error) {
		lookups.Add(1)
		time.Sleep(30 * time.Millisecond)
		return map[string]interface{}{
			"id":       int64(1),
			"userName": name,
			"passwd":   hash,
		}, nil
	}
	t.Cleanup(func() { getUserByName = origGetByName })

	const workers = 20
	start := make(chan struct{})
	var lockedCount atomic.Int64
	var wrongCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			defer func() {
				if r := recover(); r != nil {
					if pub, ok := r.(model.PublicError); ok {
						switch string(pub) {
						case msg.PasswdWrongMaxTime:
							lockedCount.Add(1)
						case msg.PasswdWrong:
							wrongCount.Add(1)
						}
					}
				}
			}()
			CheckPwdScoped(0, "bad-password", "admin", "198.51.100.77")
		}()
	}
	close(start)
	wg.Wait()

	if got := lookups.Load(); got != pwdErrorLockThreshold {
		t.Fatalf("concurrent bcrypt lookups = %d, want at most %d", got, pwdErrorLockThreshold)
	}
	if got := wrongCount.Load(); got != pwdErrorLockThreshold {
		t.Fatalf("wrong password responses = %d, want %d", got, pwdErrorLockThreshold)
	}
	if got := lockedCount.Load(); got != workers-pwdErrorLockThreshold {
		t.Fatalf("locked responses = %d, want %d", got, workers-pwdErrorLockThreshold)
	}
}
