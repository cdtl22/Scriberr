package titlededup

import (
	"context"
	"errors"
	"strings"

	"scriberr/internal/models"
	"scriberr/internal/repository"

	"gorm.io/gorm"
)

// Service provides title-based upload deduplication.
type Service struct {
	jobRepo repository.JobRepository
}

func NewService(jobRepo repository.JobRepository) *Service {
	return &Service{jobRepo: jobRepo}
}

// GuardCreate rejects the insert when another job already uses the same dedup title.
func (s *Service) GuardCreate(ctx context.Context, job *models.TranscriptionJob) error {
	if job.Title == nil || strings.TrimSpace(*job.Title) == "" {
		return s.jobRepo.Create(ctx, job)
	}

	key := DedupKey(*job.Title)
	existing, err := s.jobRepo.FindByTitleDedupKey(ctx, key)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if existing != nil {
		return &DuplicateUploadError{ExistingJob: existing}
	}
	return s.jobRepo.Create(ctx, job)
}
