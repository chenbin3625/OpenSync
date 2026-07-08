package service

import (
	"html"
	"net/url"
	"strings"
)

type dstNameMatchEntry struct {
	key       string
	value     interface{}
	ambiguous bool
}

type dstNameMatchIndex struct {
	items     map[string]interface{}
	canonical map[string]dstNameMatchEntry
}

type srcNameMatchIndex struct {
	items     map[string]interface{}
	canonical map[string]int
}

func newDstNameMatchIndex(items map[string]interface{}) dstNameMatchIndex {
	index := dstNameMatchIndex{
		items:     items,
		canonical: make(map[string]dstNameMatchEntry),
	}
	for key, value := range items {
		canonicalKey := canonicalFileKey(key)
		entry, exists := index.canonical[canonicalKey]
		if exists && entry.key != key {
			entry.ambiguous = true
			index.canonical[canonicalKey] = entry
			continue
		}
		index.canonical[canonicalKey] = dstNameMatchEntry{key: key, value: value}
	}
	return index
}

func newSrcNameMatchIndex(items map[string]interface{}) srcNameMatchIndex {
	index := srcNameMatchIndex{
		items:     items,
		canonical: make(map[string]int),
	}
	for key := range items {
		index.canonical[canonicalFileKey(key)]++
	}
	return index
}

func (i dstNameMatchIndex) find(srcKey string, srcIndex srcNameMatchIndex) (string, interface{}, bool) {
	if value, ok := i.items[srcKey]; ok {
		return srcKey, value, true
	}
	canonicalKey := canonicalFileKey(srcKey)
	if srcIndex.canonical[canonicalKey] > 1 {
		return "", nil, false
	}
	entry, ok := i.canonical[canonicalKey]
	if !ok || entry.ambiguous {
		return "", nil, false
	}
	if _, sourceOwnsMatchedName := srcIndex.items[entry.key]; sourceOwnsMatchedName {
		return "", nil, false
	}
	return entry.key, entry.value, true
}

func canonicalFileKey(key string) string {
	suffix := ""
	name := key
	if strings.HasSuffix(key, "/") {
		suffix = "/"
		name = strings.TrimSuffix(key, "/")
	}
	return canonicalFileName(name) + suffix
}

func canonicalFileName(name string) string {
	current := name
	for i := 0; i < 4; i++ {
		next := html.UnescapeString(current)
		if decoded, err := url.PathUnescape(next); err == nil && !strings.Contains(decoded, "/") {
			next = decoded
		}
		next = foldCompatibleFullwidthPunctuation(next)
		if next == current {
			return current
		}
		current = next
	}
	return current
}

func foldCompatibleFullwidthPunctuation(s string) string {
	var b strings.Builder
	changed := false
	for _, r := range s {
		folded, ok := compatibleFullwidthPunctuation[r]
		if ok {
			r = folded
			changed = true
		}
		b.WriteRune(r)
	}
	if !changed {
		return s
	}
	return b.String()
}

var compatibleFullwidthPunctuation = map[rune]rune{
	'\uFF06': '&',
	'\uFF1C': '<',
	'\uFF1E': '>',
	'\uFF02': '"',
	'\uFF07': '\'',
	'\uFF1A': ':',
	'\uFF0A': '*',
	'\uFF1F': '?',
	'\uFF5C': '|',
	'\uFF03': '#',
	'\uFF05': '%',
}
