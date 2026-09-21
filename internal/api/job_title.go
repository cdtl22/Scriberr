package api

import (
	"scriberr/internal/service/titlededup"
)

// resolveUploadTitle uses the form title when provided, otherwise the original filename.
// Extensions are stripped so titles stay human-readable and dedup keys stay consistent.
func resolveUploadTitle(postTitle, originalFilename string) *string {
	source := postTitle
	if source == "" {
		source = originalFilename
	}
	cleaned := titlededup.CleanTitle(source)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}
