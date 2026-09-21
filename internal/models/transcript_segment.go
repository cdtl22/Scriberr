package models

import "time"

// TranscriptSegment is a searchable slice of a completed transcription transcript.
type TranscriptSegment struct {
	ID                 uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TranscriptionJobID string    `json:"transcription_job_id" gorm:"type:varchar(36);not null;index:idx_ts_job_seg,priority:1"`
	SegmentIndex       int       `json:"segment_index" gorm:"not null;index:idx_ts_job_seg,priority:2"`
	Text               string    `json:"text" gorm:"type:text;not null"`
	StartTime          float64   `json:"start_time"`
	EndTime            float64   `json:"end_time"`
	Speaker            string    `json:"speaker" gorm:"type:varchar(64)"`
	StartWordIndex     int       `json:"start_word_index" gorm:"default:-1"`
	EndWordIndex       int       `json:"end_word_index" gorm:"default:-1"`
	CreatedAt          time.Time `json:"created_at" gorm:"autoCreateTime"`
}
