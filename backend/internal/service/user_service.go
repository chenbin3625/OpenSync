package service

import (
	"errors"
	"fmt"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"opensync/pkg/crypto"
	"opensync/pkg/util"
	"strings"
	"sync"
	"time"
)

var (
	errPwd        map[string][]int64
	errPwdMu      sync.Mutex
	getUserByID   = mapper.GetUserByID
	getUserByName = mapper.GetUserByName
)

const (
	pwdErrorWindowSeconds      = int64(300)
	maxPwdErrorScopes          = 1024
	cliGeneratedPasswordLength = 16
)

// CheckPwdTime checks if too many password errors in 5 minutes
func CheckPwdTime() {
	CheckPwdTimeForScope(defaultPwdErrorScope())
}

func CheckPwdTimeForScope(scope string) {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	now := time.Now().Unix()
	scope = normalizePwdErrorScope(scope)
	if errPwd == nil {
		errPwd = make(map[string][]int64)
	}
	prunePwdErrorsLocked(now)

	// Remove entries older than 5 minutes
	failures := errPwd[scope]
	cleaned := failures[:0]
	for _, t := range failures {
		if t+pwdErrorWindowSeconds >= now {
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) == 0 {
		delete(errPwd, scope)
	} else {
		errPwd[scope] = cleaned
	}
	if len(cleaned) > 3 {
		panicPublic(i18n.G("passwd_wrong_max_time"))
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
	if errPwd == nil {
		errPwd = make(map[string][]int64)
	}
	now := time.Now().Unix()
	prunePwdErrorsLocked(now)
	errPwd[scope] = append(errPwd[scope], now)
	enforcePwdErrorScopeLimitLocked()
}

func prunePwdErrorsLocked(now int64) {
	for scope, failures := range errPwd {
		cleaned := failures[:0]
		for _, t := range failures {
			if t+pwdErrorWindowSeconds >= now {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) == 0 {
			delete(errPwd, scope)
			continue
		}
		errPwd[scope] = cleaned
	}
}

func enforcePwdErrorScopeLimitLocked() {
	for len(errPwd) > maxPwdErrorScopes {
		var oldestScope string
		var oldestTime int64
		first := true
		for scope, failures := range errPwd {
			if len(failures) == 0 {
				delete(errPwd, scope)
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
		delete(errPwd, oldestScope)
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
			panicPublic(i18n.G("user_not_found"))
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
		panicPublic(i18n.G("lost_part"))
	}

	if IsInitialized() {
		panicPublic(i18n.G("system_initialized"))
	}

	hash, err := crypto.HashPassword(passwd)
	if err != nil {
		panic(err.Error())
	}
	recoveryKey, recoveryHash := newRecoveryKeyHash()
	userID, err := mapper.CreateUser(userName, hash, recoveryHash)
	if err != nil {
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
	CheckPwdTimeForScope(scope)
	user := GetUser(userID, userName)
	storedHash := fmt.Sprintf("%v", user["passwd"])
	if !crypto.CheckPassword(passwd, storedHash) {
		AddPwdErrorForScope(scope)
		panicPublic(i18n.G("passwd_wrong"))
	}
	return user
}

// EditPasswd changes user password
func EditPasswd(userID int64, passwd string, oldPasswd string) {
	CheckPwd(userID, oldPasswd, "")
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
		panicPublic(i18n.G("lost_part"))
	}
	user := GetUser(0, userName)
	storedRecoveryHash := fmt.Sprintf("%v", user["recoveryKey"])
	if !crypto.CheckPassword(recoveryKey, storedRecoveryHash) {
		panicPublic(i18n.G("key_wrong"))
	}
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
		panicPublic(i18n.G("lost_part"))
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
