package service

import "strings"

func parseExcludePatterns(exclude string) []string {
	exclude = strings.TrimSpace(exclude)
	if exclude == "" {
		return nil
	}

	separator := "\n"
	if !strings.Contains(exclude, "\n") && strings.Contains(exclude, ":") {
		separator = ":"
	}

	parts := strings.Split(exclude, separator)
	patterns := make([]string, 0, len(parts))
	for _, part := range parts {
		pattern := strings.TrimSpace(part)
		if pattern == "" {
			continue
		}
		patterns = append(patterns, pattern)
	}
	return patterns
}

func normalizeExclude(exclude string) string {
	return strings.Join(parseExcludePatterns(exclude), "\n")
}
