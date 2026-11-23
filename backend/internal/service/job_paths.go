package service

import (
	"encoding/json"
	"path"
	"strings"
)

func parsePathList(value interface{}) []string {
	return cleanPathList(parseRawPathList(value))
}

// parseRawPathList extracts path strings from JSON-encoded arrays, slices, or
// plain strings without silently deduplicating entries, so validation can
// detect and reject duplicate or overlapping selections. Non-string and nil
// elements are skipped instead of being stringified into "<nil>".
func parseRawPathList(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "[") {
			var rawItems []interface{}
			if err := json.Unmarshal([]byte(trimmed), &rawItems); err == nil {
				return extractStringPaths(rawItems)
			}
		}
		cleaned := sanitizePathEntry(v)
		if cleaned == "" {
			return nil
		}
		return []string{cleaned}
	case []string:
		paths := make([]string, 0, len(v))
		for _, item := range v {
			if cleaned := sanitizePathEntry(item); cleaned != "" {
				paths = append(paths, cleaned)
			}
		}
		return paths
	case []interface{}:
		return extractStringPaths(v)
	default:
		return nil
	}
}

func extractStringPaths(items []interface{}) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if cleaned := sanitizePathEntry(s); cleaned != "" {
			paths = append(paths, cleaned)
		}
	}
	return paths
}

// sanitizePathEntry trims line breaks and drops blank/whitespace-only entries
// while preserving leading/trailing spaces that belong to actual path
// components (e.g. "/dir/name " or "/dir/ name").
func sanitizePathEntry(item string) string {
	item = strings.Trim(item, "\r\n")
	if strings.TrimSpace(item) == "" {
		return ""
	}
	return item
}

// canonicalJobPath normalizes slashes and ensures a leading "/" for comparison
// (overlap, ancestor/descendant, and duplicate checks) without stripping spaces
// from path components.
func canonicalJobPath(raw string) string {
	cleaned := sanitizePathEntry(raw)
	if cleaned == "" {
		return ""
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return path.Clean(cleaned)
}

// pathsOverlap reports whether two canonical absolute paths are identical or
// one is an ancestor of the other.
func pathsOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b || a == "/" || b == "/" {
		return true
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func encodePathList(paths []string) string {
	cleaned := cleanPathList(paths)
	data, err := json.Marshal(cleaned)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func encodeRawPathList(paths []string) string {
	if paths == nil {
		paths = []string{}
	}
	data, err := json.Marshal(paths)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func normalizePathListForStorage(value interface{}) string {
	return encodeRawPathList(parseRawPathList(value))
}

// cleanPathList drops blank entries and duplicates by canonical path.
// Duplicates matter because a path repeated in the selection is compared
// against itself when the shared parent is derived, which yields a suffix
// carrying a leading slash.
func cleanPathList(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, item := range paths {
		item = sanitizePathEntry(item)
		if item == "" {
			continue
		}
		key := canonicalJobPath(item)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, item)
	}
	return cleaned
}
