// Package summary builds, checks and renders evidence-linked meeting-summary
// drafts from a stored transcript.
//
// Everything this package produces is an unverified AI draft. It can show that
// a quoted passage exists in a cited transcript segment and derive the speaker
// and timestamps from that segment; it cannot show that a statement is true,
// that a suggestion became a commitment, or that nothing was omitted. Nothing
// here can mark an item accepted or a technical claim verified: those states do
// not exist in this package.
package summary

const (
	// SchemaVersion identifies the stored draft layout.
	SchemaVersion = "grounded-draft/v1"
	// PromptVersion changes whenever the extraction instructions change.
	PromptVersion = "lm-grounded-2026-10-02"

	// Mode values recorded on attempts and summaries.
	ModeGrounded = "grounded_v1"
	ModeFreeform = "freeform_v1"
	// FormatLegacy marks summaries saved before generation status existed.
	FormatLegacy = "legacy_markdown"

	// DraftStatus is the only draft state in this increment.
	DraftStatus = "unverified_ai_draft"
	// ReviewNeedsReview is the only review state a candidate can leave with.
	ReviewNeedsReview = "needs_review"
	// VerificationUnverified is the only verification state that exists.
	VerificationUnverified = "UNVERIFIED"
	// NotStated replaces a missing owner or deadline.
	NotStated = "Not stated"
)

// DraftNotice is displayed and exported with every summary, whatever the
// model wrote. It is application text, not model output.
const DraftNotice = "UNVERIFIED AI DRAFT - not reviewed. Items are candidates, not accepted decisions or tasks. " +
	"Technical, code and regulatory statements are UNVERIFIED: a matching quote shows only that something was said, " +
	"not that it is correct. Check against the recording."

// LegacyNotice is added for summaries whose completion was never recorded.
const LegacyNotice = "This draft was saved by an earlier version that did not record whether generation completed; it may be partial."

// Candidate kinds. Suggestions include proposals that were not agreed.
const (
	KindDecision       = "decision"
	KindCommitment     = "commitment"
	KindSuggestion     = "suggestion"
	KindQuestion       = "question"
	KindTechnicalClaim = "technical_claim"
)

// KindOrder is the display order for candidate kinds.
var KindOrder = []string{KindDecision, KindCommitment, KindSuggestion, KindQuestion, KindTechnicalClaim}

var allowedKinds = map[string]bool{
	KindDecision:       true,
	KindCommitment:     true,
	KindSuggestion:     true,
	KindQuestion:       true,
	KindTechnicalClaim: true,
}

// Flag codes attached to candidates. Flags never change a candidate into an
// accepted or verified item; they tell the reviewer what to look at.
const (
	FlagMalformedItem           = "malformed_item"
	FlagUnknownKind             = "unknown_kind"
	FlagNoSegmentReference      = "no_segment_reference"
	FlagSegmentNotFound         = "segment_not_found"
	FlagMissingQuote            = "missing_quote"
	FlagQuoteNotInCitedSegments = "quote_not_in_cited_segments"
	FlagQuoteSpansSpeakers      = "quote_spans_speakers"
	FlagQuoteHasGap             = "quote_has_gap"
	FlagSpeakerMismatch         = "speaker_mismatch"
	FlagOwnerNotSupported       = "owner_not_supported"
	FlagOwnerNotInEvidence      = "owner_not_in_evidence"
	FlagDueNotInEvidence        = "due_not_in_evidence"
	FlagDueContradicted         = "due_contradicted_in_evidence"
	FlagPossibleLaterCorrection = "possible_later_correction"
	FlagCommitmentNotExplicit   = "commitment_not_explicit"
	FlagDecisionNotExplicit     = "decision_not_explicit"
	FlagInstructionLikeText     = "cites_instruction_like_text"
	FlagVerificationAsserted    = "speaker_asserts_verification"
	FlagTechnicalContent        = "technical_content"
	FlagInvalidSourceTimestamp  = "invalid_source_timestamp"
)

// Evidence is a transcript segment cited by a candidate. Speaker, timestamps
// and text are copied from the stored transcript, never from model output.
type Evidence struct {
	SegmentID     string  `json:"segment_id"`
	Speaker       string  `json:"speaker"`
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	Text          string  `json:"text"`
	ContainsQuote bool    `json:"contains_quote"`
}

// Flag explains why a reviewer should look closely at a candidate.
type Flag struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	SegmentID string `json:"segment_id,omitempty"`
}

// Candidate is one checked draft item.
type Candidate struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// Text is the model's description. It is never checked for accuracy.
	Text string `json:"text"`
	// Quote is the model's quotation; QuoteMatched records whether its words
	// appear, in order, in the cited segments.
	Quote        string     `json:"quote"`
	QuoteMatched bool       `json:"quote_matched"`
	Evidence     []Evidence `json:"evidence"`
	// Speaker is derived from the segment(s) containing the quote, or from the
	// first cited segment when the quote did not match.
	Speaker      string `json:"speaker"`
	ModelSpeaker string `json:"model_speaker,omitempty"`
	Owner        string `json:"owner"`
	Due          string `json:"due"`
	ReviewState  string `json:"review_state"`
	// VerificationStatus is "UNVERIFIED" for technical claims and for any item
	// with technical content. It has no other possible value.
	VerificationStatus string `json:"verification_status,omitempty"`
	Flags              []Flag `json:"flags"`
	// Rejected candidates have no usable evidence (unknown kind, no existing
	// segment reference or no quote). They are kept only for transparency.
	Rejected bool `json:"rejected"`
}

// Report counts the validation results.
type Report struct {
	Items    int `json:"items"`
	Usable   int `json:"usable"`
	Rejected int `json:"rejected"`
	Flagged  int `json:"flagged"`
	Matched  int `json:"quotes_matched"`
}

// StoredDraft is persisted with an evidence-linked summary.
type StoredDraft struct {
	SchemaVersion    string      `json:"schema_version"`
	PromptVersion    string      `json:"prompt_version"`
	Overview         string      `json:"overview"`
	Candidates       []Candidate `json:"candidates"`
	Report           Report      `json:"report"`
	TranscriptSHA256 string      `json:"transcript_sha256"`
	SegmentCount     int         `json:"segment_count"`
}
