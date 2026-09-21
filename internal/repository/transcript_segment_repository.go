package repository

import (
	"context"
	"fmt"

	"scriberr/internal/models"

	"gorm.io/gorm"
)

// TranscriptSegmentRepository stores searchable transcript segments.
type TranscriptSegmentRepository interface {
	ReplaceForJob(ctx context.Context, jobID string, segments []models.TranscriptSegment) error
	DeleteByJobID(ctx context.Context, jobID string) error
}

type transcriptSegmentRepository struct {
	db *gorm.DB
}

func NewTranscriptSegmentRepository(db *gorm.DB) TranscriptSegmentRepository {
	return &transcriptSegmentRepository{db: db}
}

func (r *transcriptSegmentRepository) ReplaceForJob(ctx context.Context, jobID string, segments []models.TranscriptSegment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uint64
		if err := tx.Model(&models.TranscriptSegment{}).
			Where("transcription_job_id = ?", jobID).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := deleteFTSRows(tx, ids); err != nil {
				return err
			}
		}
		if err := tx.Where("transcription_job_id = ?", jobID).Delete(&models.TranscriptSegment{}).Error; err != nil {
			return err
		}
		if len(segments) == 0 {
			return nil
		}
		if err := tx.CreateInBatches(&segments, 100).Error; err != nil {
			return err
		}
		for _, seg := range segments {
			if err := insertFTSRow(tx, seg.ID, seg.Text); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *transcriptSegmentRepository) DeleteByJobID(ctx context.Context, jobID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uint64
		if err := tx.Model(&models.TranscriptSegment{}).
			Where("transcription_job_id = ?", jobID).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := deleteFTSRows(tx, ids); err != nil {
				return err
			}
		}
		return tx.Where("transcription_job_id = ?", jobID).Delete(&models.TranscriptSegment{}).Error
	})
}

func deleteFTSRows(tx *gorm.DB, ids []uint64) error {
	for _, id := range ids {
		if err := tx.Exec(`INSERT INTO transcript_segments_fts(transcript_segments_fts, rowid, text) VALUES('delete', ?, '')`, id).Error; err != nil {
			return fmt.Errorf("fts delete: %w", err)
		}
	}
	return nil
}

func insertFTSRow(tx *gorm.DB, rowID uint64, text string) error {
	return tx.Exec(
		`INSERT INTO transcript_segments_fts(rowid, text) VALUES(?, ?)`,
		rowID, text,
	).Error
}
