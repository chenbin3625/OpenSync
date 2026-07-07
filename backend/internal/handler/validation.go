package handler

import (
	"errors"
	"opensync/internal/i18n"
	"strconv"
	"strings"
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

func parseEnableValue(value interface{}) (int, error) {
	switch v := value.(type) {
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case int:
		if v == 0 || v == 1 {
			return v, nil
		}
	case int64:
		if v == 0 || v == 1 {
			return int(v), nil
		}
	case float64:
		if v == 0 || v == 1 {
			return int(v), nil
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "0", "false":
			return 0, nil
		case "1", "true":
			return 1, nil
		}
	}
	return 0, errors.New(i18n.G("lost_part"))
}
