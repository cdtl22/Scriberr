package titlededup

import (
	"fmt"

	"scriberr/internal/models"
)

// DuplicateUploadError is returned when a job with the same title already exists.
type DuplicateUploadError struct {
	ExistingJob *models.TranscriptionJob
}

func (e *DuplicateUploadError) Error() string {
	title := "an existing recording"
	if e.ExistingJob != nil && e.ExistingJob.Title != nil && *e.ExistingJob.Title != "" {
		title = *e.ExistingJob.Title
	}
	return fmt.Sprintf("duplicate upload: already uploaded as %s", title)
}
