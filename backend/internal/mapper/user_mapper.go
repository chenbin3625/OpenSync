package mapper

import (
	"errors"
	"fmt"
	"opensync/internal/i18n"
)

var ErrUserNotFound = errors.New("user_not_found")

// HasUsers reports whether any local account exists.
func HasUsers() (bool, error) {
	var count int64
	if err := GetDB().QueryRow("SELECT COUNT(*) FROM user_list").Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateUser inserts a local user and returns its ID.
func CreateUser(userName string, passwd string, recoveryKey string) (int64, error) {
	return ExecuteInsert("INSERT INTO user_list(userName, passwd, recoveryKey) VALUES (?, ?, ?)", userName, passwd, recoveryKey)
}

// GetUserByName gets user by username
func GetUserByName(name string) (map[string]interface{}, error) {
	rst, err := FetchAllToTable("SELECT * FROM user_list WHERE userName=?", name)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUserNotFound, i18n.G("user_not_found"))
	}
	return rst[0], nil
}

// GetUserByID gets user by ID
func GetUserByID(id int64) (map[string]interface{}, error) {
	rst, err := FetchAllToTable("SELECT * FROM user_list WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUserNotFound, i18n.G("user_not_found"))
	}
	return rst[0], nil
}

// ResetPasswd updates user password
func ResetPasswd(userID int64, passwd string) error {
	return ExecuteUpdate("UPDATE user_list SET passwd=? WHERE id=?", passwd, userID)
}

// ResetUserCredentials updates password and recovery key together.
func ResetUserCredentials(userID int64, passwd string, recoveryKey string) error {
	return ExecuteUpdate("UPDATE user_list SET passwd=?, recoveryKey=? WHERE id=?", passwd, recoveryKey, userID)
}
