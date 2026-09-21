package transcriptindex

import (
	"fmt"

	"gorm.io/gorm"
)

// EnsureFTSSchema creates the FTS5 virtual table used for transcript search.
func EnsureFTSSchema(db *gorm.DB) error {
	if err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS transcript_segments_fts USING fts5(
			text,
			content='transcript_segments',
			content_rowid='id'
		);
	`).Error; err != nil {
		return fmt.Errorf("create fts table: %w", err)
	}
	return nil
}
