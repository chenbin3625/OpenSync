package service

import (
	"fmt"
	"sync"
	"time"
	"taosync/internal/config"
	"taosync/internal/i18n"
	"taosync/internal/mapper"
	"taosync/pkg/crypto"
)

var (
	errPwd   []int64
	errPwdMu sync.Mutex
)

// CheckPwdTime checks if too many password errors in 5 minutes
func CheckPwdTime() {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	now := time.Now().Unix()
	// Remove entries older than 5 minutes
	cleaned := errPwd[:0]
	for _, t := range errPwd {
		if t+300 >= now {
			cleaned = append(cleaned, t)
		}
	}
	errPwd = cleaned
	if len(errPwd) > 3 {
		panic(i18n.G("passwd_wrong_max_time"))
	}
}

// AddPwdError records a password error
func AddPwdError() {
	errPwdMu.Lock()
	defer errPwdMu.Unlock()
	errPwd = append(errPwd, time.Now().Unix())
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
		panic(err.Error())
	}
	return user
}

// CheckPwd validates password and returns user info
func CheckPwd(userID int64, passwd string, userName string) map[string]interface{} {
	CheckPwdTime()
	user := GetUser(userID, userName)
	cfg := config.GetConfig()
	if crypto.PasswordToMD5(passwd, cfg.Server.PasswdStr) != fmt.Sprintf("%v", user["passwd"]) {
		AddPwdError()
		panic(i18n.G("passwd_wrong"))
	}
	return user
}

// EditPasswd changes user password
func EditPasswd(userID int64, passwd string, oldPasswd string) {
	CheckPwd(userID, oldPasswd, "")
	cfg := config.GetConfig()
	mapper.ResetPasswd(userID, crypto.PasswordToMD5(passwd, cfg.Server.PasswdStr))
}

// ResetPasswd resets user password (with secret key verification)
// Returns generated password if passwd is empty, otherwise returns nil
func ResetPasswd(userName string, key string, passwd string) string {
	cfg := config.GetConfig()
	user := GetUser(0, userName)
	if key != cfg.Server.PasswdStr {
		panic(i18n.G("key_wrong"))
	}
	if passwd == "" {
		newPasswd := crypto.GeneratePassword(8)
		mapper.ResetPasswd(toInt64(user["id"]), crypto.PasswordToMD5(newPasswd, cfg.Server.PasswdStr))
		return newPasswd
	}
	mapper.ResetPasswd(toInt64(user["id"]), crypto.PasswordToMD5(passwd, cfg.Server.PasswdStr))
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
