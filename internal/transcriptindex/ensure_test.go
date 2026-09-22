package transcriptindex

import (
	"context"
	"testing"

	"scriberr/internal/models"
	"scriberr/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEnsureTestDB(t *testing.T) (*gorm.DB, *Indexer) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, EnsureFTSSchema(db))
	require.NoError(t, db.AutoMigrate(&models.TranscriptionJob{}, &models.TranscriptSegment{}))

	segmentRepo := repository.NewTranscriptSegmentRepository(db)
	jobRepo := repository.NewJobRepository(db)
	return db, NewIndexer(db, segmentRepo, jobRepo)
}

func TestIndexStats_MissingUntilIndexed(t *testing.T) {
	db, idx := setupEnsureTestDB(t)
	transcript := `{"segments":[{"start":1,"end":2,"text":"hello world","speaker":"A"}]}`
	job := &models.TranscriptionJob{
		ID:         "job-1",
		Status:     models.StatusCompleted,
		AudioPath:  "/audio/a.wav",
		Transcript: &transcript,
	}
	require.NoError(t, db.Create(job).Error)

	stats, err := idx.IndexStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.JobsWithTranscript)
	assert.Equal(t, int64(1), stats.MissingIndex)
	assert.Equal(t, int64(0), stats.JobsIndexed)

	require.NoError(t, idx.IndexJob(context.Background(), job.ID))
	stats, err = idx.IndexStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.MissingIndex)
	assert.Equal(t, int64(1), stats.JobsIndexed)
}

func TestEnsureSearchIndex_BackfillsThenNoOps(t *testing.T) {
	db, idx := setupEnsureTestDB(t)
	transcript := `{"segments":[{"start":1,"end":2,"text":"launch in May","speaker":"A"}]}`
	job := &models.TranscriptionJob{
		ID:         "job-2",
		Status:     models.StatusCompleted,
		AudioPath:  "/audio/b.wav",
		Transcript: &transcript,
	}
	require.NoError(t, db.Create(job).Error)

	first, err := idx.EnsureSearchIndex(context.Background(), DefaultEnsureBatch)
	require.NoError(t, err)
	assert.True(t, first.Backfilled)
	assert.Equal(t, 1, first.Indexed)
	assert.Equal(t, int64(0), first.RemainingMissing)

	second, err := idx.EnsureSearchIndex(context.Background(), DefaultEnsureBatch)
	require.NoError(t, err)
	assert.False(t, second.Backfilled)
	assert.Equal(t, 0, second.Indexed)
}
