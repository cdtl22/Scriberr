package transcriptindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTranscriptJSON_Segments(t *testing.T) {
	raw := `{"segments":[{"start":1.2,"end":3.4,"text":"hello world","speaker":"SPEAKER_01"}]}`
	segs, err := ParseTranscriptJSON(raw)
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Equal(t, "hello world", segs[0].Text)
	assert.Equal(t, 1.2, segs[0].StartTime)
	assert.Equal(t, "SPEAKER_01", segs[0].Speaker)
}

func TestParseTranscriptJSON_TextOnly(t *testing.T) {
	raw := `{"text":"plain transcript"}`
	segs, err := ParseTranscriptJSON(raw)
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Equal(t, "plain transcript", segs[0].Text)
}
