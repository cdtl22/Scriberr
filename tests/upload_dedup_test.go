package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"scriberr/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *APIHandlerTestSuite) postAudioUpload(filePath, fieldName, title string) *httptest.ResponseRecorder {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	file, err := os.Open(filePath)
	suite.Require().NoError(err)
	defer file.Close()
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	suite.Require().NoError(err)
	_, err = io.Copy(part, file)
	suite.Require().NoError(err)
	if title != "" {
		suite.Require().NoError(writer.WriteField("title", title))
	}
	suite.Require().NoError(writer.Close())

	req, err := http.NewRequest("POST", "/api/v1/transcription/upload", body)
	suite.Require().NoError(err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", suite.helper.TestAPIKey)

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)
	return w
}

func (suite *APIHandlerTestSuite) TestUploadDuplicateTitleDifferentContent() {
	dir := suite.T().TempDir()
	pathA := filepath.Join(dir, "first.mp3")
	pathB := filepath.Join(dir, "second.mp3")
	require.NoError(suite.T(), os.WriteFile(pathA, []byte("content-a"), 0644))
	require.NoError(suite.T(), os.WriteFile(pathB, []byte("content-b"), 0644))

	w1 := suite.postAudioUpload(pathA, "audio", "Same Title")
	assert.Equal(suite.T(), http.StatusOK, w1.Code)

	w2 := suite.postAudioUpload(pathB, "audio", "Same Title")
	assert.Equal(suite.T(), http.StatusConflict, w2.Code)

	var resp map[string]interface{}
	require.NoError(suite.T(), json.Unmarshal(w2.Body.Bytes(), &resp))
	assert.Equal(suite.T(), "duplicate_upload", resp["error"])
}

func (suite *APIHandlerTestSuite) TestUploadDuplicateTitleCaseInsensitive() {
	dir := suite.T().TempDir()
	pathA := filepath.Join(dir, "a.mp3")
	pathB := filepath.Join(dir, "b.mp3")
	require.NoError(suite.T(), os.WriteFile(pathA, []byte("a"), 0644))
	require.NoError(suite.T(), os.WriteFile(pathB, []byte("b"), 0644))

	w1 := suite.postAudioUpload(pathA, "audio", "My Meeting")
	assert.Equal(suite.T(), http.StatusOK, w1.Code)

	w2 := suite.postAudioUpload(pathB, "audio", "my meeting")
	assert.Equal(suite.T(), http.StatusConflict, w2.Code)
}

func (suite *APIHandlerTestSuite) TestUploadDifferentTitlesSameContentAccepted() {
	dir := suite.T().TempDir()
	pathA := filepath.Join(dir, "a.mp3")
	pathB := filepath.Join(dir, "b.mp3")
	content := []byte("identical-bytes")
	require.NoError(suite.T(), os.WriteFile(pathA, content, 0644))
	require.NoError(suite.T(), os.WriteFile(pathB, content, 0644))

	w1 := suite.postAudioUpload(pathA, "audio", "Title One")
	assert.Equal(suite.T(), http.StatusOK, w1.Code)
	w2 := suite.postAudioUpload(pathB, "audio", "Title Two")
	assert.Equal(suite.T(), http.StatusOK, w2.Code)
}

func (suite *APIHandlerTestSuite) TestUploadStoresTitleWithoutExtension() {
	dir := suite.T().TempDir()
	path := filepath.Join(dir, "clip.m4a")
	require.NoError(suite.T(), os.WriteFile(path, []byte("x"), 0644))

	w := suite.postAudioUpload(path, "audio", "")
	require.Equal(suite.T(), http.StatusOK, w.Code)

	var job models.TranscriptionJob
	require.NoError(suite.T(), json.Unmarshal(w.Body.Bytes(), &job))
	require.NotNil(suite.T(), job.Title)
	assert.Equal(suite.T(), "clip", *job.Title)
}

func (suite *APIHandlerTestSuite) TestDuplicateUploadDoesNotLeaveOrphanFile() {
	dir := suite.T().TempDir()
	path := filepath.Join(dir, "once.mp3")
	require.NoError(suite.T(), os.WriteFile(path, []byte("single"), 0644))

	w1 := suite.postAudioUpload(path, "audio", "Once")
	assert.Equal(suite.T(), http.StatusOK, w1.Code)

	before, err := os.ReadDir(suite.helper.Config.UploadDir)
	require.NoError(suite.T(), err)

	w2 := suite.postAudioUpload(path, "audio", "Once")
	assert.Equal(suite.T(), http.StatusConflict, w2.Code)

	after, err := os.ReadDir(suite.helper.Config.UploadDir)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), len(before), len(after))
}
