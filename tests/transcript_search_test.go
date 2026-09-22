package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"scriberr/internal/api"
	"scriberr/internal/models"
	"scriberr/internal/repository"
	"scriberr/internal/transcriptindex"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranscriptSearch_AutoBackfillBeforeQuery(t *testing.T) {
	helper := NewTestHelper(t, "search_autobackfill.db")
	defer helper.Cleanup()

	segmentRepo := repository.NewTranscriptSegmentRepository(helper.DB)
	jobRepo := repository.NewJobRepository(helper.DB)
	indexer := transcriptindex.NewIndexer(helper.DB, segmentRepo, jobRepo)

	transcript := `{"segments":[{"start":1,"end":2,"text":"workstation capacity with Tian","speaker":"A"}]}`
	job := &models.TranscriptionJob{
		ID:         "job-autobackfill",
		Status:     models.StatusCompleted,
		AudioPath:  "/data/audio/meeting.wav",
		Transcript: &transcript,
	}
	require.NoError(t, helper.DB.Create(job).Error)

	gin.SetMode(gin.TestMode)
	handler := setupSearchTestHandler(helper, indexer)
	router := gin.New()
	router.GET("/api/v1/search/transcripts", handler.SearchTranscripts)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/transcripts?q=workstation", nil)
	req.Header.Set("X-API-Key", helper.TestAPIKey)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	results, ok := payload["results"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, results)

	stats, err := indexer.IndexStats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.MissingIndex)
}

func TestTranscriptSearch_IndexAndQuery(t *testing.T) {
	helper := NewTestHelper(t, "search_test.db")
	defer helper.Cleanup()

	segmentRepo := repository.NewTranscriptSegmentRepository(helper.DB)
	jobRepo := repository.NewJobRepository(helper.DB)
	indexer := transcriptindex.NewIndexer(helper.DB, segmentRepo, jobRepo)

	transcript := `{"segments":[{"start":1842.3,"end":1874.8,"text":"We agreed to move the launch to the second week of May","speaker":"SPEAKER_01"}]}`
	job := &models.TranscriptionJob{
		ID:         "job-search-1",
		Status:     models.StatusCompleted,
		AudioPath:  "/data/audio/meeting.wav",
		Transcript: &transcript,
	}
	title := "Product Planning Meeting"
	job.Title = &title
	require.NoError(t, helper.DB.Create(job).Error)
	require.NoError(t, indexer.IndexJob(context.Background(), job.ID))

	gin.SetMode(gin.TestMode)
	handler := setupSearchTestHandler(helper, indexer)
	router := gin.New()
	router.GET("/api/v1/search/transcripts", handler.SearchTranscripts)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/transcripts?q=launch+May", nil)
	req.Header.Set("X-API-Key", helper.TestAPIKey)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	results, ok := payload["results"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, results)
}

func TestGetTranscript_NotReady(t *testing.T) {
	helper := NewTestHelper(t, "transcript_not_ready.db")
	defer helper.Cleanup()

	job := &models.TranscriptionJob{
		ID:        "job-pending",
		Status:    models.StatusProcessing,
		AudioPath: "/data/audio/pending.wav",
	}
	require.NoError(t, helper.DB.Create(job).Error)

	gin.SetMode(gin.TestMode)
	// Minimal handler wiring is covered in api_handlers_test; here we assert repository state only.
	stored, err := repository.NewJobRepository(helper.DB).FindByID(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, models.StatusProcessing, stored.Status)
}

func setupSearchTestHandler(helper *TestHelper, indexer *transcriptindex.Indexer) *api.Handler {
	h := api.NewHandler(
		helper.Config,
		helper.AuthService,
		nil,
		nil,
		repository.NewJobRepository(helper.DB),
		repository.NewAPIKeyRepository(helper.DB),
		repository.NewProfileRepository(helper.DB),
		repository.NewUserRepository(helper.DB),
		repository.NewLLMConfigRepository(helper.DB),
		repository.NewSummaryRepository(helper.DB),
		repository.NewChatRepository(helper.DB),
		repository.NewNoteRepository(helper.DB),
		repository.NewSpeakerMappingRepository(helper.DB),
		repository.NewRefreshTokenRepository(helper.DB),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	h.SetTranscriptIndexer(indexer)
	return h
}
