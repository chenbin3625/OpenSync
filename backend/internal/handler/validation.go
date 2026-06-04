package handler

import (
	"errors"
	"opensync/internal/i18n"
	"strconv"
)

func parseRequiredID(value, field string) (int64, error) {
	if value == "" {
		return 0, errors.New(i18n.G("lost_part"))
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New(i18n.G("lost_part"))
	}
	return id, nil
}
