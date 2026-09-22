package transcriptindex

import (
	"context"
	"fmt"

	"scriberr/internal/models"
)

// DefaultEnsureBatch is how many unindexed jobs we backfill per search/ask request.
const DefaultEnsureBatch = 50

// MaxEnsureBatch caps manual reindex and ensure batches.
const MaxEnsureBatch = 500

// IndexStats compares completed transcripts to searchable segment rows.
type IndexStats struct {
	JobsWithTranscript int64 `json:"jobs_with_transcript"`
	JobsIndexed        int64 `json:"jobs_indexed"`
	MissingIndex       int64 `json:"missing_index"`
}

// ReindexResult reports a missing-only backfill batch.
type ReindexResult struct {
	Indexed          int   `json:"indexed"`
	Failed           int   `json:"failed"`
	Limit            int   `json:"limit"`
	RemainingMissing int64 `json:"remaining_missing"`
}

// EnsureResult is returned when search/ask triggers automatic backfill.
type EnsureResult struct {
	Backfilled       bool  `json:"backfilled"`
	Indexed          int   `json:"indexed"`
	Failed           int   `json:"failed"`
	RemainingMissing int64 `json:"remaining_missing"`
}

const searchableJobsSQL = `
	SELECT COUNT(*) FROM transcription_jobs j
	WHERE j.deleted_at IS NULL
	  AND j.status = ?
	  AND j.transcript IS NOT NULL
	  AND TRIM(j.transcript) != ''
`

const missingIndexJobsSQL = `
	SELECT COUNT(*) FROM transcription_jobs j
	WHERE j.deleted_at IS NULL
	  AND j.status = ?
	  AND j.transcript IS NOT NULL
	  AND TRIM(j.transcript) != ''
	  AND NOT EXISTS (
	    SELECT 1 FROM transcript_segments s
	    WHERE s.transcription_job_id = j.id
	  )
`

const missingJobIDsSQL = `
	SELECT j.id FROM transcription_jobs j
	WHERE j.deleted_at IS NULL
	  AND j.status = ?
	  AND j.transcript IS NOT NULL
	  AND TRIM(j.transcript) != ''
	  AND NOT EXISTS (
	    SELECT 1 FROM transcript_segments s
	    WHERE s.transcription_job_id = j.id
	  )
	ORDER BY j.updated_at DESC
	LIMIT ?
`

// IndexStats counts jobs that should be searchable vs jobs that have segment rows.
func (idx *Indexer) IndexStats(ctx context.Context) (IndexStats, error) {
	var out IndexStats
	if err := idx.db.WithContext(ctx).Raw(searchableJobsSQL, models.StatusCompleted).Scan(&out.JobsWithTranscript).Error; err != nil {
		return IndexStats{}, fmt.Errorf("count searchable jobs: %w", err)
	}
	if err := idx.db.WithContext(ctx).Raw(missingIndexJobsSQL, models.StatusCompleted).Scan(&out.MissingIndex).Error; err != nil {
		return IndexStats{}, fmt.Errorf("count missing index: %w", err)
	}
	out.JobsIndexed = out.JobsWithTranscript - out.MissingIndex
	if out.JobsIndexed < 0 {
		out.JobsIndexed = 0
	}
	return out, nil
}

// ReindexMissing indexes completed jobs that have a transcript but no segment rows yet.
func (idx *Indexer) ReindexMissing(ctx context.Context, limit int) (ReindexResult, error) {
	if limit <= 0 {
		limit = DefaultEnsureBatch
	}
	if limit > MaxEnsureBatch {
		limit = MaxEnsureBatch
	}

	var jobIDs []string
	if err := idx.db.WithContext(ctx).Raw(missingJobIDsSQL, models.StatusCompleted, limit).Scan(&jobIDs).Error; err != nil {
		return ReindexResult{}, fmt.Errorf("list missing jobs: %w", err)
	}

	result := ReindexResult{Limit: limit}
	for _, id := range jobIDs {
		if err := idx.IndexJob(ctx, id); err != nil {
			result.Failed++
			continue
		}
		result.Indexed++
	}

	stats, err := idx.IndexStats(ctx)
	if err != nil {
		return result, err
	}
	result.RemainingMissing = stats.MissingIndex
	return result, nil
}

// EnsureSearchIndex backfills a bounded batch when transcripts exist but the FTS index is incomplete.
// Safe to call before every search; no-ops when nothing is missing.
func (idx *Indexer) EnsureSearchIndex(ctx context.Context, batchLimit int) (EnsureResult, error) {
	stats, err := idx.IndexStats(ctx)
	if err != nil {
		return EnsureResult{}, err
	}
	if stats.MissingIndex == 0 {
		return EnsureResult{RemainingMissing: 0}, nil
	}

	reindex, err := idx.ReindexMissing(ctx, batchLimit)
	if err != nil {
		return EnsureResult{}, err
	}
	return EnsureResult{
		Backfilled:       reindex.Indexed > 0,
		Indexed:          reindex.Indexed,
		Failed:           reindex.Failed,
		RemainingMissing: reindex.RemainingMissing,
	}, nil
}
