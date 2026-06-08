package service

import (
	"fmt"
	"strings"
)

func toInt64Val(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		val = strings.TrimSpace(val)
		if val == "" {
			return 0
		}
		n := int64(0)
		fmt.Sscanf(val, "%d", &n)
		return n
	default:
		return 0
	}
}

func stringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}
