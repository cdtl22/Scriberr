package transcriptindex

import (
	"encoding/json"
	"strings"

	"scriberr/internal/models"
)

type rawSegment struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Text    string  `json:"text"`
	Speaker *string `json:"speaker,omitempty"`
}

type rawWord struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Word  string  `json:"word"`
}

// ParseTranscriptJSON turns stored transcript JSON into indexable segments.
// Supports: raw string, {"text": "..."}, {"segments": [...]}, {"word_segments": [...]}.
func ParseTranscriptJSON(transcriptJSON string) ([]models.TranscriptSegment, error) {
	raw := strings.TrimSpace(transcriptJSON)
	if raw == "" {
		return nil, nil
	}

	// Plain string JSON value
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal([]byte(raw), &text); err == nil && strings.TrimSpace(text) != "" {
			return []models.TranscriptSegment{{
				SegmentIndex:   0,
				Text:           strings.TrimSpace(text),
				StartTime:      0,
				EndTime:        0,
				StartWordIndex: 0,
				EndWordIndex:   wordCount(text) - 1,
			}}, nil
		}
	}

	var generic map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		// Treat as opaque text
		return []models.TranscriptSegment{{
			SegmentIndex: 0,
			Text:         raw,
		}}, nil
	}

	if segRaw, ok := generic["segments"]; ok {
		var segments []rawSegment
		if err := json.Unmarshal(segRaw, &segments); err == nil && len(segments) > 0 {
			return segmentsFromTimed(segments), nil
		}
	}

	if wordRaw, ok := generic["word_segments"]; ok {
		var words []rawWord
		if err := json.Unmarshal(wordRaw, &words); err == nil && len(words) > 0 {
			return segmentsFromWords(words), nil
		}
	}

	if textRaw, ok := generic["text"]; ok {
		var text string
		if err := json.Unmarshal(textRaw, &text); err == nil && strings.TrimSpace(text) != "" {
			return []models.TranscriptSegment{{
				SegmentIndex:   0,
				Text:           strings.TrimSpace(text),
				StartWordIndex: 0,
				EndWordIndex:   wordCount(text) - 1,
			}}, nil
		}
	}

	return nil, nil
}

func segmentsFromTimed(segments []rawSegment) []models.TranscriptSegment {
	out := make([]models.TranscriptSegment, 0, len(segments))
	wordIdx := 0
	for i, seg := range segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		wc := wordCount(text)
		endWord := wordIdx + wc - 1
		if wc == 0 {
			endWord = wordIdx
		}
		speaker := ""
		if seg.Speaker != nil {
			speaker = *seg.Speaker
		}
		out = append(out, models.TranscriptSegment{
			SegmentIndex:   i,
			Text:           text,
			StartTime:      seg.Start,
			EndTime:        seg.End,
			Speaker:        speaker,
			StartWordIndex: wordIdx,
			EndWordIndex:   endWord,
		})
		wordIdx = endWord + 1
	}
	return out
}

func segmentsFromWords(words []rawWord) []models.TranscriptSegment {
	const maxWordsPerSegment = 40
	out := make([]models.TranscriptSegment, 0)
	var chunk []rawWord
	chunkStartIdx := 0
	globalWord := 0

	flush := func(segIndex int) {
		if len(chunk) == 0 {
			return
		}
		parts := make([]string, len(chunk))
		for j, w := range chunk {
			parts[j] = w.Word
		}
		text := strings.TrimSpace(strings.Join(parts, " "))
		if text == "" {
			chunk = nil
			return
		}
		out = append(out, models.TranscriptSegment{
			SegmentIndex:   segIndex,
			Text:           text,
			StartTime:      chunk[0].Start,
			EndTime:        chunk[len(chunk)-1].End,
			StartWordIndex: chunkStartIdx,
			EndWordIndex:   chunkStartIdx + len(chunk) - 1,
		})
		chunk = nil
	}

	segIndex := 0
	for _, w := range words {
		word := strings.TrimSpace(w.Word)
		if word == "" {
			continue
		}
		if len(chunk) == 0 {
			chunkStartIdx = globalWord
		}
		chunk = append(chunk, w)
		globalWord++
		if len(chunk) >= maxWordsPerSegment {
			flush(segIndex)
			segIndex++
		}
	}
	flush(segIndex)
	return out
}

func wordCount(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}
