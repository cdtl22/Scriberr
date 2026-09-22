package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"scriberr/internal/llm"
	"scriberr/internal/transcriptindex"
	"scriberr/pkg/logger"

	"github.com/gin-gonic/gin"
)

// SearchTranscripts searches indexed transcript segments (not recording titles).
// @Summary Search transcript excerpts
// @Tags search
// @Produce json
// @Param q query string false "Search query"
// @Param query query string false "Alias for q"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Page size" default(10)
// @Param recording_ids query string false "Comma-separated recording UUIDs"
// @Param speaker query string false "Speaker filter"
// @Param updated_after query string false "RFC3339 updated filter"
// @Param updated_before query string false "RFC3339 updated filter"
// @Router /api/v1/search/transcripts [get]
// @Security ApiKeyAuth
// @Security BearerAuth
func (h *Handler) ensureTranscriptSearchIndex(c *gin.Context) {
	if h.transcriptIndexer == nil {
		return
	}
	result, err := h.transcriptIndexer.EnsureSearchIndex(
		c.Request.Context(),
		transcriptindex.DefaultEnsureBatch,
	)
	if err != nil {
		logger.Warn("Transcript search index ensure failed", "error", err.Error())
		return
	}
	if result.Indexed > 0 || result.Failed > 0 {
		logger.Info(
			"Transcript search index backfill",
			"indexed", result.Indexed,
			"failed", result.Failed,
			"remaining_missing", result.RemainingMissing,
		)
	}
}

// GetTranscriptSearchIndexStatus reports transcript vs segment index coverage (for ops/debug).
// @Summary Transcript search index status
// @Tags search
// @Produce json
// @Router /api/v1/search/index-status [get]
// @Security ApiKeyAuth
// @Security BearerAuth
func (h *Handler) GetTranscriptSearchIndexStatus(c *gin.Context) {
	if h.transcriptIndexer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Transcript search is not available"})
		return
	}
	stats, err := h.transcriptIndexer.IndexStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read index status"})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) SearchTranscripts(c *gin.Context) {
	if h.transcriptIndexer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Transcript search is not available"})
		return
	}
	h.ensureTranscriptSearchIndex(c)

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		query = strings.TrimSpace(c.Query("query"))
	}
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q or query is required"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	var recordingIDs []string
	if raw := strings.TrimSpace(c.Query("recording_ids")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			id := strings.TrimSpace(part)
			if id != "" {
				recordingIDs = append(recordingIDs, id)
			}
		}
	}

	var updatedAfter, updatedBefore *time.Time
	if s := strings.TrimSpace(c.Query("updated_after")); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			updatedAfter = &t
		}
	}
	if s := strings.TrimSpace(c.Query("updated_before")); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			updatedBefore = &t
		}
	}

	result, err := h.transcriptIndexer.Search(c.Request.Context(), transcriptindex.SearchParams{
		Query:         query,
		Page:          page,
		Limit:         limit,
		RecordingIDs:  recordingIDs,
		Speaker:       strings.TrimSpace(c.Query("speaker")),
		UpdatedAfter:  updatedAfter,
		UpdatedBefore: updatedBefore,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

type TranscriptAskRequest struct {
	Question     string   `json:"question" binding:"required"`
	SearchQuery  string   `json:"search_query"`
	Model        string   `json:"model"`
	Limit        int      `json:"limit"`
	RecordingIDs []string `json:"recording_ids,omitempty"`
}

type TranscriptAskSource struct {
	RecordingID string  `json:"recording_id"`
	Title       string  `json:"title"`
	Excerpt     string  `json:"excerpt"`
	StartTime   float64 `json:"start_time"`
	EndTime     float64 `json:"end_time"`
}

type TranscriptAskResponse struct {
	Answer  string                `json:"answer"`
	Sources []TranscriptAskSource `json:"sources"`
	Message string                `json:"message,omitempty"`
}

// AskTranscripts runs excerpt search then answers with the configured LLM and citations.
// @Summary Answer a question using transcript excerpts
// @Tags search
// @Accept json
// @Produce json
// @Router /api/v1/search/transcripts/ask [post]
// @Security ApiKeyAuth
// @Security BearerAuth
func (h *Handler) AskTranscripts(c *gin.Context) {
	if h.transcriptIndexer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Transcript search is not available"})
		return
	}
	h.ensureTranscriptSearchIndex(c)

	var req TranscriptAskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	question := strings.TrimSpace(req.Question)
	if question == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question is required"})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > transcriptindex.MaxSearchLimit {
		limit = transcriptindex.MaxSearchLimit
	}

	searchQuery := strings.TrimSpace(req.SearchQuery)
	if searchQuery == "" {
		searchQuery = question
	}

	searchResult, err := h.transcriptIndexer.Search(c.Request.Context(), transcriptindex.SearchParams{
		Query:        searchQuery,
		Page:         1,
		Limit:        limit,
		RecordingIDs: req.RecordingIDs,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(searchResult.Results) == 0 {
		c.JSON(http.StatusOK, TranscriptAskResponse{
			Answer:  "",
			Sources: []TranscriptAskSource{},
			Message: searchResult.Message,
		})
		return
	}

	sources := make([]TranscriptAskSource, 0, len(searchResult.Results))
	var contextParts []string
	for i, hit := range searchResult.Results {
		sources = append(sources, TranscriptAskSource{
			RecordingID: hit.RecordingID,
			Title:       hit.Title,
			Excerpt:     hit.Excerpt,
			StartTime:   hit.StartTime,
			EndTime:     hit.EndTime,
		})
		contextParts = append(contextParts, fmt.Sprintf(
			"[%d] recording_id=%s title=%q start=%.2fs end=%.2fs\n%s",
			i+1, hit.RecordingID, hit.Title, hit.StartTime, hit.EndTime, hit.Excerpt,
		))
	}

	svc, _, err := h.getLLMService(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		models, err := svc.GetModels(c.Request.Context())
		if err != nil || len(models) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model is required and no default models are configured"})
			return
		}
		model = models[0]
	}

	systemPrompt := `You answer questions using ONLY the provided transcript excerpts.
Cite sources using [n] notation matching excerpt numbers.
Include recording title and start/end timestamps when citing.
If excerpts do not contain enough information, say you cannot answer from the transcripts.`

	userPrompt := fmt.Sprintf("Question: %s\n\nExcerpts:\n%s", question, strings.Join(contextParts, "\n\n"))

	resp, err := svc.ChatCompletion(c.Request.Context(), model, []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, 0.2)
	if err != nil || resp == nil || len(resp.Choices) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LLM request failed"})
		return
	}

	answer := strings.TrimSpace(resp.Choices[0].Message.Content)
	c.JSON(http.StatusOK, TranscriptAskResponse{
		Answer:  answer,
		Sources: sources,
	})
}

// ReindexTranscripts rebuilds the segment index for completed jobs (bounded batch).
// @Summary Reindex transcript search segments
// @Tags search
// @Produce json
// @Param limit query int false "Max jobs to reindex" default(50)
// @Router /api/v1/search/reindex [post]
// @Security ApiKeyAuth
// @Security BearerAuth
func (h *Handler) ReindexTranscripts(c *gin.Context) {
	if h.transcriptIndexer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Transcript search is not available"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = transcriptindex.DefaultEnsureBatch
	}
	if limit > transcriptindex.MaxEnsureBatch {
		limit = transcriptindex.MaxEnsureBatch
	}

	result, err := h.transcriptIndexer.ReindexMissing(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reindex transcripts"})
		return
	}

	stats, _ := h.transcriptIndexer.IndexStats(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"indexed":            result.Indexed,
		"failed":             result.Failed,
		"limit":              result.Limit,
		"remaining_missing":  result.RemainingMissing,
		"jobs_with_transcript": stats.JobsWithTranscript,
		"jobs_indexed":       stats.JobsIndexed,
	})
}
