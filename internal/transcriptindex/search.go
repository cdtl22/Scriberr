package transcriptindex

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"scriberr/internal/models"
)

const (
	DefaultSearchLimit = 10
	MaxSearchLimit     = 50
	MaxSearchPage      = 100
)

// SearchParams controls transcript excerpt search.
type SearchParams struct {
	Query        string
	Page         int
	Limit        int
	RecordingIDs []string
	Speaker      string
	UpdatedAfter *time.Time
	UpdatedBefore *time.Time
}

// SearchHit is one excerpt result with citation metadata.
type SearchHit struct {
	RecordingID       string  `json:"recording_id"`
	Title             string  `json:"title"`
	Score             float64 `json:"score"`
	Excerpt           string  `json:"excerpt"`
	StartTime         float64 `json:"start_time"`
	EndTime           float64 `json:"end_time"`
	Speaker           string  `json:"speaker,omitempty"`
	StartSegmentIndex int     `json:"start_segment_index"`
	EndSegmentIndex   int     `json:"end_segment_index"`
	StartWordIndex    int     `json:"start_word_index"`
	EndWordIndex      int     `json:"end_word_index"`
	AudioURL          string  `json:"audio_url"`
	TranscriptURL     string  `json:"transcript_url"`
}

type SearchResult struct {
	Query      string            `json:"query"`
	Results    []SearchHit       `json:"results"`
	Message    string            `json:"message,omitempty"`
	Pagination map[string]int64  `json:"pagination"`
}

type segmentRow struct {
	ID                 uint64
	TranscriptionJobID string
	SegmentIndex       int
	Text               string
	StartTime          float64
	EndTime            float64
	Speaker            string
	StartWordIndex     int
	EndWordIndex       int
	Title              *string
	Score              float64
}

// Search runs FTS5-backed excerpt retrieval for completed jobs only.
func (idx *Indexer) Search(ctx context.Context, p SearchParams) (*SearchResult, error) {
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	limit := p.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	page := p.Page
	if page <= 0 {
		page = 1
	}
	if page > MaxSearchPage {
		page = MaxSearchPage
	}
	offset := (page - 1) * limit

	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return &SearchResult{
			Query:   query,
			Results: []SearchHit{},
			Message: "No searchable terms in query",
			Pagination: map[string]int64{
				"page": int64(page), "limit": int64(limit), "total": 0, "pages": 0,
			},
		}, nil
	}

	rows, total, err := idx.searchRows(ctx, ftsQuery, p, offset, limit*3)
	if err != nil {
		return nil, err
	}

	merged := mergeAdjacentHits(rows, limit)
	hits := make([]SearchHit, 0, len(merged))
	for _, m := range merged {
		row := m.segmentRow
		title := ""
		if row.Title != nil {
			title = *row.Title
		}
		hits = append(hits, SearchHit{
			RecordingID:       row.TranscriptionJobID,
			Title:             title,
			Score:             row.Score,
			Excerpt:           truncateExcerpt(row.Text),
			StartTime:         row.StartTime,
			EndTime:           row.EndTime,
			Speaker:           row.Speaker,
			StartSegmentIndex: row.SegmentIndex,
			EndSegmentIndex:   m.EndSegmentIndex,
			StartWordIndex:    row.StartWordIndex,
			EndWordIndex:      row.EndWordIndex,
			AudioURL:          fmt.Sprintf("/api/v1/transcription/%s/audio", row.TranscriptionJobID),
			TranscriptURL:     fmt.Sprintf("/api/v1/transcription/%s/transcript", row.TranscriptionJobID),
		})
	}

	pages := int64(0)
	if limit > 0 {
		pages = (total + int64(limit) - 1) / int64(limit)
	}

	out := &SearchResult{
		Query:   query,
		Results: hits,
		Pagination: map[string]int64{
			"page": int64(page), "limit": int64(limit), "total": total, "pages": pages,
		},
	}
	if len(hits) == 0 {
		out.Message = "No transcript excerpts matched your query"
	}
	return out, nil
}

func (idx *Indexer) searchRows(ctx context.Context, ftsQuery string, p SearchParams, offset, fetchLimit int) ([]segmentRow, int64, error) {
	args := []interface{}{ftsQuery}
	where := `
		j.deleted_at IS NULL
		AND j.status = ?
	`
	args = append(args, models.StatusCompleted)

	if p.Speaker != "" {
		where += " AND s.speaker = ?"
		args = append(args, p.Speaker)
	}
	if p.UpdatedAfter != nil {
		where += " AND j.updated_at > ?"
		args = append(args, *p.UpdatedAfter)
	}
	if p.UpdatedBefore != nil {
		where += " AND j.updated_at < ?"
		args = append(args, *p.UpdatedBefore)
	}
	if len(p.RecordingIDs) > 0 {
		placeholders := strings.Repeat("?,", len(p.RecordingIDs))
		placeholders = placeholders[:len(placeholders)-1]
		where += fmt.Sprintf(" AND s.transcription_job_id IN (%s)", placeholders)
		for _, id := range p.RecordingIDs {
			args = append(args, id)
		}
	}

	countSQL := fmt.Sprintf(`
		SELECT COUNT(DISTINCT s.id)
		FROM transcript_segments_fts fts
		JOIN transcript_segments s ON s.id = fts.rowid
		JOIN transcription_jobs j ON j.id = s.transcription_job_id
		WHERE transcript_segments_fts MATCH ? AND %s
	`, where)

	var total int64
	if err := idx.db.WithContext(ctx).Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	selectSQL := fmt.Sprintf(`
		SELECT s.id, s.transcription_job_id, s.segment_index, s.text,
			s.start_time, s.end_time, s.speaker, s.start_word_index, s.end_word_index,
			j.title, bm25(transcript_segments_fts) AS score
		FROM transcript_segments_fts fts
		JOIN transcript_segments s ON s.id = fts.rowid
		JOIN transcription_jobs j ON j.id = s.transcription_job_id
		WHERE transcript_segments_fts MATCH ? AND %s
		ORDER BY score
		LIMIT ? OFFSET ?
	`, where)

	selectArgs := append(args, fetchLimit, offset)
	var rows []segmentRow
	if err := idx.db.WithContext(ctx).Raw(selectSQL, selectArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func buildFTSQuery(query string) string {
	terms := strings.Fields(strings.TrimSpace(query))
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		clean := strings.Trim(term, "\"'")
		if len(clean) < 2 {
			continue
		}
		clean = strings.ReplaceAll(clean, "\"", "")
		parts = append(parts, fmt.Sprintf("%q", clean))
	}
	return strings.Join(parts, " AND ")
}

func truncateExcerpt(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= MaxExcerptChars {
		return text
	}
	return text[:MaxExcerptChars-1] + "…"
}

type mergedRow struct {
	segmentRow
	EndSegmentIndex int
}

func mergeAdjacentHits(rows []segmentRow, limit int) []mergedRow {
	if len(rows) == 0 {
		return nil
	}
	const gapSeconds = 2.0
	out := make([]mergedRow, 0, len(rows))
	seen := make(map[string]bool)

	for _, row := range rows {
		key := fmt.Sprintf("%s:%.1f:%.1f", row.TranscriptionJobID, row.StartTime, row.EndTime)
		if seen[key] {
			continue
		}
		seen[key] = true

		m := mergedRow{segmentRow: row, EndSegmentIndex: row.SegmentIndex}
		if len(out) > 0 {
			last := &out[len(out)-1]
			if last.TranscriptionJobID == row.TranscriptionJobID &&
				row.StartTime-last.EndTime <= gapSeconds &&
				last.Speaker == row.Speaker {
				last.Text = strings.TrimSpace(last.Text + " " + row.Text)
				last.EndTime = math.Max(last.EndTime, row.EndTime)
				last.EndSegmentIndex = row.SegmentIndex
				last.EndWordIndex = row.EndWordIndex
				if row.Score < last.Score {
					last.Score = row.Score
				}
				continue
			}
		}
		out = append(out, m)
		if len(out) >= limit {
			break
		}
	}
	return out
}
