package summary

// BuildDraft parses model output and validates it against the transcript. A
// parse failure returns ErrMalformedOutput and nothing is saved.
func BuildDraft(output string, t *Transcript) (*StoredDraft, error) {
	raw, err := ParseDraft(output)
	if err != nil {
		return nil, err
	}
	candidates, report := Validate(raw, t)
	return &StoredDraft{
		SchemaVersion:    SchemaVersion,
		PromptVersion:    PromptVersion,
		Overview:         collapse(raw.Overview),
		Candidates:       candidates,
		Report:           report,
		TranscriptSHA256: t.SHA256,
		SegmentCount:     len(t.Segments),
	}, nil
}
