package mapper

// GetNotifyList gets notify list, optionally only enabled ones
func GetNotifyList(needEnable bool) ([]map[string]interface{}, error) {
	if needEnable {
		return FetchAllToTable("SELECT * FROM notify WHERE enable=1")
	}
	return FetchAllToTable("SELECT * FROM notify")
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
	return ExecuteUpdate(
		"UPDATE notify SET enable=?, method=?, params=? WHERE id=?",
		notify["enable"], notify["method"], notify["params"], notify["id"],
	)
}

// UpdateNotifyStatus updates notify enable status
func UpdateNotifyStatus(notifyID int64, enable int) error {
	return ExecuteUpdate("UPDATE notify SET enable=? WHERE id=?", enable, notifyID)
}

// DeleteNotify deletes a notify config
func DeleteNotify(notifyID int64) error {
	return ExecuteUpdate("DELETE FROM notify WHERE id=?", notifyID)
}
