package summary

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrMalformedOutput means the model output is not a single JSON object with
// the expected fields. Such output is never saved as a summary.
var ErrMalformedOutput = errors.New("model output is not a valid draft object")

// RawDraft is the model's output before any checking. Fields the model adds,
// such as "verified" or "status", are ignored by construction.
type RawDraft struct {
	Overview string
	Items    []RawItem
}

// RawItem is one unchecked candidate as written by the model. Malformed is set
// when the item itself could not be decoded; it is then rejected, not dropped.
type RawItem struct {
	Kind       string     `json:"kind"`
	Text       string     `json:"text"`
	SegmentIDs stringList `json:"segment_ids"`
	Quote      string     `json:"quote"`
	Speaker    string     `json:"speaker"`
	Owner      string     `json:"owner"`
	Due        string     `json:"due"`
	Malformed  bool       `json:"-"`
}

// stringList accepts either ["S1","S2"] or a single "S1".
type stringList []string

func (l *stringList) UnmarshalJSON(b []byte) error {
	var many []string
	if err := json.Unmarshal(b, &many); err == nil {
		*l = many
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*l = []string{one}
	return nil
}

// ParseDraft accepts exactly one JSON object, optionally wrapped in a single
// Markdown code fence. "overview" must be a string and "items" an array; a
// missing field is treated as malformed rather than as "nothing found".
func ParseDraft(output string) (*RawDraft, error) {
	s := strings.TrimSpace(output)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```JSON")
		s = strings.TrimPrefix(s, "```")
		if !strings.HasSuffix(s, "```") {
			return nil, fmt.Errorf("%w: unterminated code fence", ErrMalformedOutput)
		}
		s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
	}
	if !strings.HasPrefix(s, "{") {
		return nil, fmt.Errorf("%w: output is not a JSON object", ErrMalformedOutput)
	}

	var shape struct {
		Overview *string            `json:"overview"`
		Items    *[]json.RawMessage `json:"items"`
	}
	dec := json.NewDecoder(strings.NewReader(s))
	if err := dec.Decode(&shape); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedOutput, err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%w: unexpected content after the JSON object", ErrMalformedOutput)
	}
	if shape.Overview == nil || shape.Items == nil {
		return nil, fmt.Errorf("%w: missing \"overview\" or \"items\"", ErrMalformedOutput)
	}

	draft := &RawDraft{Overview: strings.TrimSpace(*shape.Overview)}
	for _, raw := range *shape.Items {
		var item RawItem
		if err := json.Unmarshal(raw, &item); err != nil {
			item = RawItem{Malformed: true}
		}
		draft.Items = append(draft.Items, item)
	}
	return draft, nil
}
