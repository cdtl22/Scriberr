package titlededup

import (
	"scriberr/internal/titleutil"
)

// CleanTitle trims whitespace and removes a trailing file extension from a title or filename.
func CleanTitle(title string) string {
	return titleutil.CleanTitle(title)
}

// DedupKey returns the case-insensitive key used for title-based duplicate detection.
func DedupKey(title string) string {
	return titleutil.DedupKey(title)
}
