package summary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// UnknownSpeaker labels segments stored without a diarization speaker.
const UnknownSpeaker = "UNKNOWN"

// ErrNoSegments means the stored transcript has no usable timed segments, so
// an evidence-linked draft cannot cite anything.
var ErrNoSegments = errors.New("transcript has no timed segments with text")

// Segment is one stored transcript segment. ID is "S" + its 1-based position
// in the stored segment list, so IDs follow the transcript, not the model.
type Segment struct {
	ID      string  `json:"segment_id"`
	Index   int     `json:"index"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker string  `json:"speaker"`
	Text    string  `json:"text"`
}

// TimesValid reports whether the source timestamps are usable.
func (s Segment) TimesValid() bool {
	return !math.IsNaN(s.Start) && !math.IsInf(s.Start, 0) &&
		!math.IsNaN(s.End) && !math.IsInf(s.End, 0) &&
		s.Start >= 0 && s.End >= s.Start
}

// Transcript is the parsed stored transcript plus its fingerprint.
type Transcript struct {
	Segments []Segment
	// SHA256 fingerprints the stored transcript JSON, so a draft can tell
	// whether the transcript changed after generation.
	SHA256 string
	byID   map[string]int
}

// Fingerprint returns the hex SHA-256 of a stored transcript string.
func Fingerprint(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ParseStoredTranscript reads the JSON stored on a transcription job
// (interfaces.TranscriptResult: {"segments":[{start,end,text,speaker}]}).
func ParseStoredTranscript(raw string) (*Transcript, error) {
	var stored struct {
		Segments []struct {
			Start   float64 `json:"start"`
			End     float64 `json:"end"`
			Text    string  `json:"text"`
			Speaker *string `json:"speaker"`
		} `json:"segments"`
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, fmt.Errorf("stored transcript is not valid JSON: %w", err)
	}
	t := &Transcript{SHA256: Fingerprint(raw), byID: make(map[string]int, len(stored.Segments))}
	withText := 0
	for i, s := range stored.Segments {
		speaker := UnknownSpeaker
		if s.Speaker != nil && strings.TrimSpace(*s.Speaker) != "" {
			speaker = strings.TrimSpace(*s.Speaker)
		}
		seg := Segment{
			ID:      fmt.Sprintf("S%d", i+1),
			Index:   i,
			Start:   s.Start,
			End:     s.End,
			Speaker: speaker,
			Text:    strings.Join(strings.Fields(s.Text), " "),
		}
		if seg.Text != "" {
			withText++
		}
		t.byID[seg.ID] = len(t.Segments)
		t.Segments = append(t.Segments, seg)
	}
	if withText == 0 {
		return nil, ErrNoSegments
	}
	return t, nil
}

// Segment returns the segment with the given canonical ID ("S3").
func (t *Transcript) Segment(id string) (Segment, bool) {
	i, ok := t.byID[id]
	if !ok {
		return Segment{}, false
	}
	return t.Segments[i], true
}

// Speakers returns the distinct speaker labels in transcript order.
func (t *Transcript) Speakers() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range t.Segments {
		if !seen[s.Speaker] {
			seen[s.Speaker] = true
			out = append(out, s.Speaker)
		}
	}
	return out
}
