package transcriptindex

import (
	"context"
	"fmt"
	"strings"

	"scriberr/internal/repository"

	"gorm.io/gorm"
)

const MaxExcerptChars = 500

// Indexer maintains transcript_segments rows and the FTS5 mirror table.
type Indexer struct {
	db        *gorm.DB
	segmentRepo repository.TranscriptSegmentRepository
	jobRepo   repository.JobRepository
}

func NewIndexer(db *gorm.DB, segmentRepo repository.TranscriptSegmentRepository, jobRepo repository.JobRepository) *Indexer {
	return &Indexer{db: db, segmentRepo: segmentRepo, jobRepo: jobRepo}
}

// IndexJob rebuilds segment rows for a completed job with a non-empty transcript.
func (idx *Indexer) IndexJob(ctx context.Context, jobID string) error {
	job, err := idx.jobRepo.FindByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Transcript == nil || strings.TrimSpace(*job.Transcript) == "" {
		return idx.DeleteJob(ctx, jobID)
	}

	segments, err := ParseTranscriptJSON(*job.Transcript)
	if err != nil {
		return fmt.Errorf("parse transcript: %w", err)
	}
	if len(segments) == 0 {
		return idx.DeleteJob(ctx, jobID)
	}
	for i := range segments {
		segments[i].TranscriptionJobID = jobID
	}
	return idx.segmentRepo.ReplaceForJob(ctx, jobID, segments)
}

// DeleteJob removes indexed segments for a transcription.
func (idx *Indexer) DeleteJob(ctx context.Context, jobID string) error {
	return idx.segmentRepo.DeleteByJobID(ctx, jobID)
}
