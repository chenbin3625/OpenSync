package service

import "strings"

// parseExcludePatterns splits the exclude input on newlines only. A single
// line is one gitignore-style pattern — splitting on ":" as older versions did
// corrupted patterns that legitimately contain a colon (e.g. "foo:bar.txt").
// Legacy colon-separated input fails toward the safe direction: the whole line
// becomes one narrow pattern that excludes less, never more.
func parseExcludePatterns(exclude string) []string {
	exclude = strings.TrimSpace(exclude)
	if exclude == "" {
		return nil
	}

	parts := strings.Split(exclude, "\n")
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
