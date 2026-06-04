package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseSrcPaths(value interface{}) []string {
	return parsePathList(value)
}

func parseDstPaths(value interface{}) []string {
	return parsePathList(value)
}

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

func normalizeSrcPathForStorage(value interface{}) string {
	return encodePathList(parseSrcPaths(value))
}

func normalizeDstPathForStorage(value interface{}) string {
	return encodePathList(parseDstPaths(value))
}

func cleanPathList(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		cleaned = append(cleaned, path)
	}
	return cleaned
}
