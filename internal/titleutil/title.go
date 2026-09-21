package titleutil

import (
	"path/filepath"
	"strings"
)

// CleanTitle trims whitespace and removes a trailing file extension from a title or filename.
func CleanTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	ext := filepath.Ext(title)
	if ext != "" && len(title) > len(ext) {
		return strings.TrimSpace(title[:len(title)-len(ext)])
	}
	return title
}

// DedupKey returns the case-insensitive key used for title-based duplicate detection.
func DedupKey(title string) string {
	return strings.ToLower(CleanTitle(title))
}
