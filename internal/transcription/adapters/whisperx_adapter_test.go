package adapters

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scriberr/internal/transcription/interfaces"
)

func TestWhisperXRemoteBaseURL(t *testing.T) {
	t.Setenv("WHISPERX_REMOTE_URL", "  http://whisperx-worker.example:8000/  ")
	if got := whisperXRemoteBaseURL(); got != "http://whisperx-worker.example:8000" {
		t.Fatalf("unexpected normalized URL: %q", got)
	}

	t.Setenv("WHISPERX_REMOTE_URL", "")
	if got := whisperXRemoteBaseURL(); got != "" {
		t.Fatalf("expected empty URL, got %q", got)
	}
}

func TestWhisperXTranscribeViaRemote(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "sample.wav")
	if err := os.WriteFile(audioPath, []byte("fake-audio"), 0644); err != nil {
		t.Fatalf("failed to write audio fixture: %v", err)
	}

	const responseJSON = `{"language":"en","text":"hello world","segments":[{"start":0,"end":1,"text":"hello world"}]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transcribe" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		if header.Filename != "sample.wav" {
			http.Error(w, "unexpected filename", http.StatusBadRequest)
			return
		}
		if _, err := io.Copy(io.Discard, file); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	t.Setenv("WHISPERX_REMOTE_URL", strings.TrimSuffix(server.URL, "/"))

	adapter := NewWhisperXAdapter(t.TempDir())
	tempDir := filepath.Join(t.TempDir(), "whisperx", "job-1")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	if err := adapter.transcribeViaRemote(context.Background(), interfaces.AudioInput{
		FilePath: audioPath,
		Format:   "wav",
	}, tempDir, strings.TrimSuffix(server.URL, "/")); err != nil {
		t.Fatalf("transcribeViaRemote failed: %v", err)
	}

	result, err := adapter.parseResult(tempDir, interfaces.AudioInput{FilePath: audioPath}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("parseResult failed: %v", err)
	}
	if result.Text != "hello world" {
		t.Fatalf("unexpected transcript text: %q", result.Text)
	}
	if result.Language != "en" {
		t.Fatalf("unexpected language: %q", result.Language)
	}
}

func TestWhisperXTranscribeViaRemoteMultipartFieldName(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "clip.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatalf("failed to write audio fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			http.Error(w, "unexpected form field", http.StatusBadRequest)
			return
		}
		_, _ = io.Copy(io.Discard, part)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"language":"en","segments":[],"text":"ok"}`))
	}))
	defer server.Close()

	adapter := NewWhisperXAdapter(t.TempDir())
	tempDir := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		t.Fatalf("mkdir tempDir: %v", err)
	}

	if err := adapter.transcribeViaRemote(context.Background(), interfaces.AudioInput{
		FilePath: audioPath,
		Format:   "mp3",
	}, tempDir, strings.TrimSuffix(server.URL, "/")); err != nil {
		t.Fatalf("transcribeViaRemote: %v", err)
	}
}
