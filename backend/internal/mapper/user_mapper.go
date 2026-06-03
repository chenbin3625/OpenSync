package mapper

import (
	"errors"
	"taosync/internal/i18n"
)

// GetUserByName gets user by username
func GetUserByName(name string) (map[string]interface{}, error) {
	rst, err := FetchAllToTable("SELECT * FROM user_list WHERE userName=?", name)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, errors.New(i18n.G("user_not_found"))
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
		return nil, errors.New(i18n.G("user_not_found"))
	}
	return rst[0], nil
}

// ResetPasswd updates user password
func ResetPasswd(userID int64, passwd string) error {
	return ExecuteUpdate("UPDATE user_list SET passwd=? WHERE id=?", passwd, userID)
}
