package service

import (
	"fmt"
	"log"
	"opensync/internal/config"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"opensync/pkg/crypto"
	"strings"
	"sync"
	"time"
)

var (
	errPwd   map[string][]int64
	errPwdMu sync.Mutex
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

	// Remove entries older than 5 minutes
	failures := errPwd[scope]
	cleaned := failures[:0]
	for _, t := range failures {
		if t+300 >= now {
			cleaned = append(cleaned, t)
		}
	}
	errPwd[scope] = cleaned
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
	errPwd[scope] = append(errPwd[scope], time.Now().Unix())
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
		user, err = mapper.GetUserByID(userID)
	} else {
		user, err = mapper.GetUserByName(userName)
	}
	if err != nil {
		if err.Error() == i18n.G("user_not_found") {
			panicPublic(err.Error())
		}
		panic(err.Error())
	}
	return user
}

// CheckPwd validates password and returns user info
func CheckPwd(userID int64, passwd string, userName string) map[string]interface{} {
	return CheckPwdScoped(userID, passwd, userName, "")
}

func CheckPwdScoped(userID int64, passwd string, userName string, clientScope string) map[string]interface{} {
	scope := passwordErrorScope(userID, userName, clientScope)
	CheckPwdTimeForScope(scope)
	user := GetUser(userID, userName)
	cfg := config.GetConfig()
	storedHash := fmt.Sprintf("%v", user["passwd"])
	if !crypto.CheckPassword(passwd, storedHash, cfg.Server.PasswdStr) {
		AddPwdErrorForScope(scope)
		panicPublic(i18n.G("passwd_wrong"))
	}
	if !crypto.IsModernPasswordHash(storedHash) {
		newHash, err := crypto.HashPassword(passwd)
		if err != nil {
			log.Printf("Failed to upgrade password hash: %v", err)
		} else {
			userID := toInt64(user["id"])
			if err := mapper.ResetPasswd(userID, newHash); err != nil {
				log.Printf("Failed to persist upgraded password hash: %v", err)
			} else {
				user["passwd"] = newHash
			}
		}
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

// ResetPasswd resets user password (with secret key verification)
// Returns generated password if passwd is empty, otherwise returns nil
func ResetPasswd(userName string, key string, passwd string) string {
	cfg := config.GetConfig()
	user := GetUser(0, userName)
	if key != cfg.Server.PasswdStr {
		panicPublic(i18n.G("key_wrong"))
	}
	if passwd == "" {
		newPasswd := crypto.GeneratePassword(8)
		hash, err := crypto.HashPassword(newPasswd)
		if err != nil {
			panic(err.Error())
		}
		if err := mapper.ResetPasswd(toInt64(user["id"]), hash); err != nil {
			panic(err.Error())
		}
		return newPasswd
	}
	hash, err := crypto.HashPassword(passwd)
	if err != nil {
		panic(err.Error())
	}
	if err := mapper.ResetPasswd(toInt64(user["id"]), hash); err != nil {
		panic(err.Error())
	}
	return ""
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
