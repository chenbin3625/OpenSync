package service

import (
	"errors"
	"fmt"
	"opensync/internal/mapper"
	"opensync/internal/msg"
	"opensync/pkg/crypto"
	"opensync/pkg/util"
	"strings"
	"sync"
	"time"
)

var (
	errPwd          map[string][]int64
	errPwdByIP      map[string][]int64
	inFlightPwd     map[string]int
	inFlightPwdByIP map[string]int
	errPwdMu        sync.Mutex
	getUserByID     = mapper.GetUserByID
	getUserByName   = mapper.GetUserByName
)

const (
	pwdErrorWindowSeconds      = int64(300)
	maxPwdErrorScopes          = 1024
	pwdErrorLockThreshold      = 4
	ipPwdErrorLockThreshold    = 10
	cliGeneratedPasswordLength = 16
	minPasswordLength          = 8
	maxPasswordBytes           = 72
	dummyPasswordHash          = "$2a$10$bxCodrDhN/EbuPfzKC5zmeZFtzgi1h1Yr.AOgTUKVCQEWuuLJUNaq"
)

func initPwdTrackingLocked() {
	if errPwd == nil {
		errPwd = make(map[string][]int64)
	}
	if errPwdByIP == nil {
		errPwdByIP = make(map[string][]int64)
	}
	if inFlightPwd == nil {
		inFlightPwd = make(map[string]int)
	}
	if inFlightPwdByIP == nil {
		inFlightPwdByIP = make(map[string]int)
	}
}

// CheckPwdTime checks if too many password errors in 5 minutes
func CheckPwdTime() {
	CheckPwdTimeForScope(defaultPwdErrorScope())
}

func CheckPwdTimeForScope(scope string) {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	now := time.Now().Unix()
	scope = normalizePwdErrorScope(scope)
	initPwdTrackingLocked()
	prunePwdErrorMapLocked(errPwd, now)

	if len(errPwd[scope])+inFlightPwd[scope] >= pwdErrorLockThreshold {
		panicPublic(msg.PasswdWrongMaxTime)
	}
}

// AddPwdError records a password error
func AddPwdError() {
	AddPwdErrorForScope(defaultPwdErrorScope())
}

func AddPwdErrorForScope(scope string) {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	scope = normalizePwdErrorScope(scope)
	initPwdTrackingLocked()
	now := time.Now().Unix()
	prunePwdErrorMapLocked(errPwd, now)
	errPwd[scope] = append(errPwd[scope], now)
	enforcePwdErrorMapLimitLocked(errPwd, pwdErrorLockThreshold)
}

// ClearPwdErrorForScope clears any recorded failures for a scope upon successful auth
func ClearPwdErrorForScope(scope string) {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	scope = normalizePwdErrorScope(scope)
	if errPwd != nil {
		delete(errPwd, scope)
	}
}

func clientIPScope(clientScope string) string {
	clientScope = strings.TrimSpace(clientScope)
	if clientScope == "" {
		return ""
	}
	return "ip:" + clientScope
}

// beginPwdAttempt atomically checks both per-account+IP and IP-level lockout
// thresholds (including concurrent in-flight attempts) and reserves an in-flight
// slot so concurrent burst requests cannot bypass the lockout threshold.
func beginPwdAttempt(scope string, ipScope string) {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	scope = normalizePwdErrorScope(scope)
	initPwdTrackingLocked()
	now := time.Now().Unix()
	prunePwdErrorMapLocked(errPwd, now)
	if ipScope != "" {
		prunePwdErrorMapLocked(errPwdByIP, now)
	}
	if len(errPwd[scope])+inFlightPwd[scope] >= pwdErrorLockThreshold {
		panicPublic(msg.PasswdWrongMaxTime)
	}
	if ipScope != "" && len(errPwdByIP[ipScope])+inFlightPwdByIP[ipScope] >= ipPwdErrorLockThreshold {
		panicPublic(msg.PasswdWrongMaxTime)
	}
	inFlightPwd[scope]++
	if ipScope != "" {
		inFlightPwdByIP[ipScope]++
	}
}

// finishPwdAttempt releases the in-flight reservation and atomically records a
// failure or clears prior failures under the same lock so there is no TOCTOU gap.
func finishPwdAttempt(scope string, ipScope string, failed bool, succeeded bool) {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	scope = normalizePwdErrorScope(scope)
	initPwdTrackingLocked()
	now := time.Now().Unix()

	if failed {
		prunePwdErrorMapLocked(errPwd, now)
		errPwd[scope] = append(errPwd[scope], now)
		enforcePwdErrorMapLimitLocked(errPwd, pwdErrorLockThreshold)
		if ipScope != "" {
			prunePwdErrorMapLocked(errPwdByIP, now)
			errPwdByIP[ipScope] = append(errPwdByIP[ipScope], now)
			enforcePwdErrorMapLimitLocked(errPwdByIP, ipPwdErrorLockThreshold)
		}
	} else if succeeded {
		delete(errPwd, scope)
		if ipScope != "" {
			delete(errPwdByIP, ipScope)
		}
	}

	if inFlightPwd[scope] <= 1 {
		delete(inFlightPwd, scope)
	} else {
		inFlightPwd[scope]--
	}
	if ipScope != "" {
		if inFlightPwdByIP[ipScope] <= 1 {
			delete(inFlightPwdByIP, ipScope)
		} else {
			inFlightPwdByIP[ipScope]--
		}
	}
}

func prunePwdErrorMapLocked(store map[string][]int64, now int64) {
	for scope, failures := range store {
		cleaned := failures[:0]
		for _, t := range failures {
			if t+pwdErrorWindowSeconds >= now {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) == 0 {
			delete(store, scope)
			continue
		}
		store[scope] = cleaned
	}
}

// enforcePwdErrorMapLimitLocked evicts the oldest unlocked scopes when the map
// exceeds maxPwdErrorScopes. Actively locked (unexpired) entries are never
// evicted so flooding random usernames cannot clear an active lockout.
func enforcePwdErrorMapLimitLocked(store map[string][]int64, lockThreshold int) {
	for len(store) > maxPwdErrorScopes {
		var oldestScope string
		var oldestTime int64
		first := true
		for scope, failures := range store {
			if len(failures) == 0 {
				delete(store, scope)
				continue
			}
			if len(failures) >= lockThreshold {
				continue
			}
			lastFailure := failures[len(failures)-1]
			if first || lastFailure < oldestTime {
				oldestScope = scope
				oldestTime = lastFailure
				first = false
			}
		}
		if oldestScope == "" {
			return
		}
		delete(store, oldestScope)
	}
}

func defaultPwdErrorScope() string {
	return "default"
}

func normalizePwdErrorScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return defaultPwdErrorScope()
	}
	return scope
}

func validatePassword(passwd string) {
	if len(strings.TrimSpace(passwd)) < minPasswordLength {
		panicPublic(msg.PasswdTooShort)
	}
	if len([]byte(passwd)) > maxPasswordBytes {
		panicPublic(msg.PasswdTooLong)
	}
}

func checkDummyPassword(passwd string) {
	_ = crypto.CheckPassword(passwd, dummyPasswordHash)
}

func passwordErrorScope(userID int64, userName string, clientScope string) string {
	principal := strings.ToLower(strings.TrimSpace(userName))
	if principal == "" && userID > 0 {
		principal = fmt.Sprintf("id:%d", userID)
	}
	if principal == "" {
		principal = "unknown"
	}
	clientScope = normalizePwdErrorScope(clientScope)
	return principal + "|" + clientScope
}

// GetUser gets user by ID or username
func GetUser(userID int64, userName string) map[string]interface{} {
	var user map[string]interface{}
	var err error
	if userID > 0 {
		user, err = getUserByID(userID)
	} else {
		user, err = getUserByName(userName)
	}
	if err != nil {
		if errors.Is(err, mapper.ErrUserNotFound) {
			panicPublic(msg.UserNotFound)
		}
		panic(err.Error())
	}
	return user
}

// IsInitialized reports whether the application already has a local account.
func IsInitialized() bool {
	initialized, err := mapper.HasUsers()
	if err != nil {
		panic(err.Error())
	}
	return initialized
}

// InitializeUser creates the first local account from web setup.
func InitializeUser(userName string, passwd string) (map[string]interface{}, string) {
	userName = strings.TrimSpace(userName)
	if userName == "" || strings.TrimSpace(passwd) == "" {
		panicPublic(msg.LostPart)
	}
	validatePassword(passwd)

	if IsInitialized() {
		panicPublic(msg.SystemInitialized)
	}

	hash, err := crypto.HashPassword(passwd)
	if err != nil {
		panic(err.Error())
	}
	recoveryKey, recoveryHash := newRecoveryKeyHash()
	userID, err := mapper.CreateInitialUser(userName, hash, recoveryHash)
	if err != nil {
		if errors.Is(err, mapper.ErrAlreadyInitialized) || IsInitialized() {
			panicPublic(msg.SystemInitialized)
		}
		panic(err.Error())
	}
	user, err := mapper.GetUserByID(userID)
	if err != nil {
		panic(err.Error())
	}
	return user, recoveryKey
}

// CheckPwd validates password and returns user info
func CheckPwd(userID int64, passwd string, userName string) map[string]interface{} {
	return CheckPwdScoped(userID, passwd, userName, "")
}

func CheckPwdScoped(userID int64, passwd string, userName string, clientScope string) map[string]interface{} {
	scope := passwordErrorScope(userID, userName, clientScope)
	ipScope := clientIPScope(clientScope)
	beginPwdAttempt(scope, ipScope)
	failed := false
	succeeded := false
	defer func() {
		finishPwdAttempt(scope, ipScope, failed, succeeded)
	}()

	var user map[string]interface{}
	var err error
	if userID > 0 {
		user, err = getUserByID(userID)
	} else {
		user, err = getUserByName(userName)
	}
	if err != nil {
		if errors.Is(err, mapper.ErrUserNotFound) {
			checkDummyPassword(passwd)
			failed = true
			panicPublic(msg.PasswdWrong)
		}
		panic(err.Error())
	}
	storedHash := fmt.Sprintf("%v", user["passwd"])
	if !crypto.CheckPassword(passwd, storedHash) {
		failed = true
		panicPublic(msg.PasswdWrong)
	}
	succeeded = true
	return user
}

// EditPasswd changes user password
func EditPasswd(userID int64, passwd string, oldPasswd string) {
	CheckPwd(userID, oldPasswd, "")
	validatePassword(passwd)
	hash, err := crypto.HashPassword(passwd)
	if err != nil {
		panic(err.Error())
	}
	if err := mapper.ResetPasswd(userID, hash); err != nil {
		panic(err.Error())
	}
}

// ResetPasswd resets user password after recovery key verification.
// It returns the replacement recovery key, which must be shown once to the user.
func ResetPasswd(userName string, recoveryKey string, passwd string) string {
	userName = strings.TrimSpace(userName)
	recoveryKey = strings.TrimSpace(recoveryKey)
	if userName == "" || recoveryKey == "" || strings.TrimSpace(passwd) == "" {
		panicPublic(msg.LostPart)
	}
	// Rate-limit recovery key attempts the same way login attempts are limited,
	// scoped per username. The 24-char key is not practically brute-forceable,
	// but this endpoint is unauthenticated, so defense in depth applies.
	scope := passwordErrorScope(0, userName, "recovery")
	beginPwdAttempt(scope, "")
	failed := false
	succeeded := false
	defer func() {
		finishPwdAttempt(scope, "", failed, succeeded)
	}()

	user, err := getUserByName(userName)
	if err != nil {
		if errors.Is(err, mapper.ErrUserNotFound) {
			checkDummyPassword(recoveryKey)
			failed = true
			panicPublic(msg.KeyWrong)
		}
		panic(err.Error())
	}
	storedRecoveryHash := fmt.Sprintf("%v", user["recoveryKey"])
	if !crypto.CheckPassword(recoveryKey, storedRecoveryHash) {
		failed = true
		panicPublic(msg.KeyWrong)
	}
	succeeded = true
	validatePassword(passwd)
	newRecoveryKey, newRecoveryHash := newRecoveryKeyHash()
	hash, err := crypto.HashPassword(passwd)
	if err != nil {
		panic(err.Error())
	}
	if err := mapper.ResetUserCredentials(util.ToInt64(user["id"]), hash, newRecoveryHash); err != nil {
		panic(err.Error())
	}
	return newRecoveryKey
}

// ResetPasswdForCLI resets user credentials for local server operators.
func ResetPasswdForCLI(userName string) (string, string) {
	userName = strings.TrimSpace(userName)
	if userName == "" {
		panicPublic(msg.LostPart)
	}
	user := GetUser(0, userName)
	newPasswd := crypto.GeneratePassword(cliGeneratedPasswordLength)
	newRecoveryKey, newRecoveryHash := newRecoveryKeyHash()
	hash, err := crypto.HashPassword(newPasswd)
	if err != nil {
		panic(err.Error())
	}
	if err := mapper.ResetUserCredentials(util.ToInt64(user["id"]), hash, newRecoveryHash); err != nil {
		panic(err.Error())
	}
	return newPasswd, newRecoveryKey
}

func newRecoveryKeyHash() (string, string) {
	recoveryKey := crypto.GenerateRecoveryKey()
	hash, err := crypto.HashPassword(recoveryKey)
	if err != nil {
		panic(err.Error())
	}
	return recoveryKey, hash
}
