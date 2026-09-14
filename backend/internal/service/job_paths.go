package service

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

func parsePathList(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return nil
		}
		if strings.HasPrefix(raw, "[") {
			var paths []string
			if err := json.Unmarshal([]byte(raw), &paths); err == nil {
				return cleanPathList(paths)
			}
		}
		return cleanPathList([]string{raw})
	case []string:
		return cleanPathList(v)
	case []interface{}:
		paths := make([]string, 0, len(v))
		for _, item := range v {
			paths = append(paths, fmt.Sprintf("%v", item))
		}
		return cleanPathList(paths)
	default:
		return cleanPathList([]string{fmt.Sprintf("%v", v)})
	}
}

func encodePathList(paths []string) string {
	cleaned := cleanPathList(paths)
	data, err := json.Marshal(cleaned)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func normalizePathListForStorage(value interface{}) string {
	return encodePathList(parsePathList(value))
}

// cleanPathList drops blank entries and duplicates. Duplicates matter because a
// path repeated in the selection is compared against itself when the shared
// parent is derived, which yields a suffix carrying a leading slash.
func cleanPathList(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, item := range paths {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := path.Clean(item)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, item)
	}
	return cleaned
}
