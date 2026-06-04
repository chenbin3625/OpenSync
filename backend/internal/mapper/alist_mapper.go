package mapper

import (
	"errors"
	"opensync/internal/i18n"
)

// GetAlistList gets all alist entries
func GetAlistList() ([]map[string]interface{}, error) {
	return FetchAllToTable("SELECT * FROM alist_list")
}

// GetAlistByID gets alist by ID
func GetAlistByID(alistID int64) (map[string]interface{}, error) {
	rst, err := FetchAllToTable("SELECT * FROM alist_list WHERE id=?", alistID)
	if err != nil {
		return nil, err
	}
	if len(rst) == 0 {
		return nil, errors.New(i18n.G("alist_not_found"))
	}
	return rst[0], nil
}

// AddAlist inserts a new alist entry
func AddAlist(remark, url, userName, token string) (int64, error) {
	return ExecuteInsert(
		"INSERT INTO alist_list (remark, url, userName, token) VALUES (?, ?, ?, ?)",
		remark, url, userName, token,
	)
}

// UpdateAlist updates an alist entry
func UpdateAlist(id int64, remark, url string, token *string) error {
	if token != nil {
		return ExecuteUpdate("UPDATE alist_list SET remark=?, url=?, token=? WHERE id=?", remark, url, *token, id)
	}
	return ExecuteUpdate("UPDATE alist_list SET remark=?, url=? WHERE id=?", remark, url, id)
}

// RemoveAlist deletes an alist entry
func RemoveAlist(alistID int64) error {
	return ExecuteUpdate("DELETE FROM alist_list WHERE id=?", alistID)
}
