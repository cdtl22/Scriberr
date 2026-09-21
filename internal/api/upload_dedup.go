package api

import (
	"errors"
	"net/http"

	"scriberr/internal/models"
	"scriberr/internal/service/titlededup"

	"github.com/gin-gonic/gin"
)

func duplicateUploadJSON(existing *models.TranscriptionJob) gin.H {
	title := ""
	if existing != nil && existing.Title != nil {
		title = *existing.Title
	}
	message := "A recording with this title has already been uploaded."
	if title != "" {
		message = "A recording titled \"" + title + "\" has already been uploaded."
	}
	resp := gin.H{
		"error":   "duplicate_upload",
		"message": message,
	}
	if existing != nil {
		resp["existing_job_id"] = existing.ID
		if title != "" {
			resp["existing_title"] = title
		}
	}
	return resp
}

func (h *Handler) respondDuplicateUpload(c *gin.Context, err error, cleanup func()) bool {
	var dup *titlededup.DuplicateUploadError
	if !errors.As(err, &dup) {
		return false
	}
	if cleanup != nil {
		cleanup()
	}
	c.JSON(http.StatusConflict, duplicateUploadJSON(dup.ExistingJob))
	return true
}

// createJobUnlessDuplicateTitle persists the job or writes a 409 when the title already exists.
// Returns true when the HTTP response has been written.
func (h *Handler) createJobUnlessDuplicateTitle(c *gin.Context, job *models.TranscriptionJob, cleanup func()) bool {
	err := h.titleDedup.GuardCreate(c.Request.Context(), job)
	if h.respondDuplicateUpload(c, err, cleanup) {
		return true
	}
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job"})
		return true
	}
	return false
}
