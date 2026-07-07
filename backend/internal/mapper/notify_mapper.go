package mapper

import (
	"errors"
	"opensync/internal/i18n"
)

// GetNotifyList gets notify list, optionally only enabled ones
func GetNotifyList(needEnable bool) ([]map[string]interface{}, error) {
	if needEnable {
		return FetchAllToTable("SELECT * FROM notify WHERE enable=1")
	}
	return FetchAllToTable("SELECT * FROM notify")
}

// GetNotifyByID gets a single notify config by ID (raw params, internal use only).
func GetNotifyByID(notifyID int64) (map[string]interface{}, error) {
	rows, err := FetchAllToTable("SELECT * FROM notify WHERE id=?", notifyID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// AddNotify inserts a new notify config
func AddNotify(notify map[string]interface{}) (int64, error) {
	return ExecuteInsert(
		"INSERT INTO notify(enable, method, params) VALUES (?, ?, ?)",
		notify["enable"], notify["method"], notify["params"],
	)
}

// EditNotify updates a notify config
func EditNotify(notify map[string]interface{}) error {
	return executeNotifyUpdate(
		"UPDATE notify SET enable=?, method=?, params=? WHERE id=?",
		notify["enable"], notify["method"], notify["params"], notify["id"],
	)
}

// UpdateNotifyStatus updates notify enable status
func UpdateNotifyStatus(notifyID int64, enable int) error {
	return executeNotifyUpdate("UPDATE notify SET enable=? WHERE id=?", enable, notifyID)
}

// DeleteNotify deletes a notify config
func DeleteNotify(notifyID int64) error {
	return executeNotifyUpdate("DELETE FROM notify WHERE id=?", notifyID)
}

func executeNotifyUpdate(query string, args ...interface{}) error {
	result, err := GetDB().Exec(query, args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New(i18n.G("lost_part"))
	}
	return nil
}
