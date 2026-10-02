package summary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	Description  string                     `json:"description"`
	Transcript   json.RawMessage            `json:"transcript"`
	ModelOutputs map[string]json.RawMessage `json:"model_outputs"`
}

func loadFixture(t *testing.T, name string) (fixture, *Transcript) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var f fixture
	require.NoError(t, json.Unmarshal(b, &f))
	tr, err := ParseStoredTranscript(string(f.Transcript))
	require.NoError(t, err)
	return f, tr
}

// draftFor runs the mocked model output for a scenario through parsing and
// validation, exactly as the server does after a completed stream.
func draftFor(t *testing.T, file, scenario string) (*StoredDraft, *Transcript) {
	t.Helper()
	f, tr := loadFixture(t, file)
	out, ok := f.ModelOutputs[scenario]
	require.True(t, ok, "scenario %s missing from %s", scenario, file)
	d, err := BuildDraft(string(out), tr)
	require.NoError(t, err)
	return d, tr
}

func flagCodes(c Candidate) []string {
	var codes []string
	for _, f := range c.Flags {
		codes = append(codes, f.Code)
	}
	return codes
}

func onlyCandidate(t *testing.T, d *StoredDraft) Candidate {
	t.Helper()
	require.Len(t, d.Candidates, 1)
	return d.Candidates[0]
}

// Every candidate, whatever the scenario, must leave validation unaccepted.
func assertNeverAccepted(t *testing.T, d *StoredDraft) {
	t.Helper()
	for _, c := range d.Candidates {
		assert.Equal(t, ReviewNeedsReview, c.ReviewState, "candidate %s", c.ID)
		if c.VerificationStatus != "" {
			assert.Equal(t, VerificationUnverified, c.VerificationStatus, "candidate %s", c.ID)
		}
	}
}

func TestExplicitCommitmentVersusUnassignedSuggestion(t *testing.T) {
	d, _ := draftFor(t, "supplier_commitment.json", "correct")
	require.Len(t, d.Candidates, 2)
	assertNeverAccepted(t, d)

	commitment := d.Candidates[0]
	assert.Equal(t, KindCommitment, commitment.Kind)
	assert.True(t, commitment.QuoteMatched)
	assert.Equal(t, "SPEAKER_00", commitment.Speaker)
	assert.Equal(t, "SPEAKER_00", commitment.Owner)
	assert.Equal(t, "Friday", commitment.Due)
	assert.Empty(t, commitment.Flags, "an explicit 'I will ... by Friday' should not be flagged")
	require.Len(t, commitment.Evidence, 1)
	assert.Equal(t, "S2", commitment.Evidence[0].SegmentID)
	assert.Equal(t, 4.2, commitment.Evidence[0].Start)
	assert.Equal(t, 8.9, commitment.Evidence[0].End)

	suggestion := d.Candidates[1]
	assert.Equal(t, KindSuggestion, suggestion.Kind)
	assert.Equal(t, NotStated, suggestion.Owner)
	assert.Equal(t, NotStated, suggestion.Due)
	assert.Equal(t, "SPEAKER_01", suggestion.Speaker)
}

// Regression: the qualified model promoted "someone should check" to an action item.
func TestSuggestionPromotedToCommitmentIsFlagged(t *testing.T) {
	d, _ := draftFor(t, "supplier_commitment.json", "suggestion_promoted_to_commitment")
	c := onlyCandidate(t, d)
	assertNeverAccepted(t, d)
	assert.False(t, c.Rejected, "the item stays visible for review")
	assert.Contains(t, flagCodes(c), FlagCommitmentNotExplicit)
	assert.Contains(t, flagCodes(c), FlagDueNotInEvidence, "Friday belongs to a different segment")
	for _, f := range c.Flags {
		if f.Code == FlagCommitmentNotExplicit {
			assert.Contains(t, f.Message, "suggestion")
		}
	}
}

func TestOwnerMovedToAnotherSpeakerIsFlagged(t *testing.T) {
	d, _ := draftFor(t, "supplier_commitment.json", "owner_moved_to_other_speaker")
	c := onlyCandidate(t, d)
	assert.Contains(t, flagCodes(c), FlagOwnerNotSupported)
	assert.Contains(t, flagCodes(c), FlagCommitmentNotExplicit)
}

func TestProposedPurchaseNotAgreedIsNotAcceptedAsDecision(t *testing.T) {
	d, _ := draftFor(t, "purchase_not_agreed.json", "proposal_as_decision")
	c := onlyCandidate(t, d)
	assertNeverAccepted(t, d)
	assert.Equal(t, KindDecision, c.Kind)
	assert.Contains(t, flagCodes(c), FlagDecisionNotExplicit)

	d, _ = draftFor(t, "purchase_not_agreed.json", "refusal_as_commitment")
	c = onlyCandidate(t, d)
	assert.Contains(t, flagCodes(c), FlagCommitmentNotExplicit, "a refusal is not a commitment")

	d, _ = draftFor(t, "purchase_not_agreed.json", "correct")
	require.Len(t, d.Candidates, 2)
	assert.Equal(t, KindSuggestion, d.Candidates[0].Kind)
	assert.True(t, d.Candidates[0].QuoteMatched)
	assert.True(t, d.Candidates[0].Evidence[0].ContainsQuote)
	assert.False(t, d.Candidates[0].Evidence[1].ContainsQuote, "S2 is cited context, not the quote source")
	assert.NotContains(t, flagCodes(d.Candidates[1]), FlagDecisionNotExplicit)
}

func TestMondayCorrectedToTuesday(t *testing.T) {
	d, _ := draftFor(t, "deadline_correction.json", "superseded_deadline")
	c := onlyCandidate(t, d)
	assert.Equal(t, "Monday", c.Due)
	require.Contains(t, flagCodes(c), FlagPossibleLaterCorrection)
	for _, f := range c.Flags {
		if f.Code == FlagPossibleLaterCorrection {
			assert.Equal(t, "S3", f.SegmentID)
		}
	}

	d, _ = draftFor(t, "deadline_correction.json", "corrected_deadline")
	c = onlyCandidate(t, d)
	assert.Equal(t, "Tuesday", c.Due)
	assert.True(t, c.QuoteMatched)
	assert.Empty(t, c.Flags, "the corrected deadline cites the correcting segment")

	d, _ = draftFor(t, "deadline_correction.json", "monday_despite_cited_correction")
	c = onlyCandidate(t, d)
	assert.Contains(t, flagCodes(c), FlagDueContradicted)
}

func TestMissingOwnerAndDeadlineStayNotStated(t *testing.T) {
	d, _ := draftFor(t, "missing_owner_deadline.json", "blank_fields")
	c := onlyCandidate(t, d)
	assert.Equal(t, NotStated, c.Owner)
	assert.Equal(t, NotStated, c.Due)
	assert.NotContains(t, flagCodes(c), FlagOwnerNotInEvidence)

	d, _ = draftFor(t, "missing_owner_deadline.json", "invented_owner_and_date")
	c = onlyCandidate(t, d)
	assert.Equal(t, "Dave", c.Owner, "model values are shown as written, with flags")
	assert.Contains(t, flagCodes(c), FlagOwnerNotInEvidence)
	assert.Contains(t, flagCodes(c), FlagDueNotInEvidence)
}

func TestQuoteAttributedToWrongSpeaker(t *testing.T) {
	d, _ := draftFor(t, "wrong_speaker_and_fabrication.json", "wrong_speaker")
	c := onlyCandidate(t, d)
	assert.True(t, c.QuoteMatched)
	assert.Equal(t, "SPEAKER_00", c.Speaker, "speaker comes from the transcript")
	assert.Equal(t, "SPEAKER_01", c.ModelSpeaker)
	assert.Contains(t, flagCodes(c), FlagSpeakerMismatch)
	// The model's invented timestamp (00:45:00) is ignored; times come from S1.
	assert.Equal(t, 0.0, c.Evidence[0].Start)
	assert.Equal(t, 5.0, c.Evidence[0].End)
	assert.Equal(t, VerificationUnverified, c.VerificationStatus)

	d, _ = draftFor(t, "wrong_speaker_and_fabrication.json", "quote_cites_wrong_segment")
	c = onlyCandidate(t, d)
	assert.False(t, c.QuoteMatched)
	require.Contains(t, flagCodes(c), FlagQuoteNotInCitedSegments)
	for _, f := range c.Flags {
		if f.Code == FlagQuoteNotInCitedSegments {
			assert.Equal(t, "S1", f.SegmentID)
			assert.Contains(t, f.Message, "SPEAKER_00")
		}
	}
}

func TestParaphraseAndFabricatedQuotesAreFlagged(t *testing.T) {
	d, _ := draftFor(t, "wrong_speaker_and_fabrication.json", "paraphrased_quote")
	c := onlyCandidate(t, d)
	assert.False(t, c.QuoteMatched, "'should' is not what was said ('can')")
	assert.Contains(t, flagCodes(c), FlagQuoteNotInCitedSegments)

	d, _ = draftFor(t, "wrong_speaker_and_fabrication.json", "fabricated_quote")
	c = onlyCandidate(t, d)
	assert.False(t, c.QuoteMatched)
	assert.Contains(t, flagCodes(c), FlagQuoteNotInCitedSegments)
}

func TestFabricatedSegmentReferenceIsRejected(t *testing.T) {
	d, _ := draftFor(t, "wrong_speaker_and_fabrication.json", "fabricated_reference")
	c := onlyCandidate(t, d)
	assert.True(t, c.Rejected)
	assert.Contains(t, flagCodes(c), FlagSegmentNotFound)
	assert.Empty(t, c.Evidence)
	assert.Equal(t, 1, d.Report.Rejected)
	assert.Equal(t, 0, d.Report.Usable)
}

func TestUnusableItemsAreRejectedNotDropped(t *testing.T) {
	d, _ := draftFor(t, "wrong_speaker_and_fabrication.json", "unusable_items")
	require.Len(t, d.Candidates, 4)
	for _, c := range d.Candidates {
		assert.True(t, c.Rejected, "candidate %s", c.ID)
	}
	assert.Contains(t, flagCodes(d.Candidates[0]), FlagUnknownKind)
	assert.Contains(t, flagCodes(d.Candidates[1]), FlagMissingQuote)
	assert.Contains(t, flagCodes(d.Candidates[2]), FlagNoSegmentReference)
	assert.Contains(t, flagCodes(d.Candidates[3]), FlagMalformedItem)
}

func TestConflictingTechnicalClaimsStayUnverified(t *testing.T) {
	d, _ := draftFor(t, "conflicting_technical_claims.json", "model_claims_verified")
	require.Len(t, d.Candidates, 4)
	assertNeverAccepted(t, d)
	for _, c := range d.Candidates {
		assert.Equal(t, VerificationUnverified, c.VerificationStatus,
			"candidate %s: speaker assertions, model agreement, matching quotes and repetition never verify", c.ID)
	}
	assert.True(t, d.Candidates[0].QuoteMatched, "a matching quote proves only that it was said")
	assert.Contains(t, flagCodes(d.Candidates[1]), FlagVerificationAsserted)
	assert.Contains(t, flagCodes(d.Candidates[2]), FlagVerificationAsserted)
	assert.Contains(t, flagCodes(d.Candidates[3]), FlagTechnicalContent)

	// Nothing the model wrote ("verified": true, "VERIFIED", "accepted") survives.
	b, err := json.Marshal(d)
	require.NoError(t, err)
	s := string(b)
	assert.NotContains(t, strings.ReplaceAll(s, "UNVERIFIED", ""), "VERIFIED")
	assert.NotContains(t, s, `"verified":`)
	assert.NotContains(t, s, "accepted")
	assert.NotContains(t, s, "approved\"")
}

func TestTranscriptInjectionCannotDirectTheResult(t *testing.T) {
	d, _ := draftFor(t, "prompt_injection.json", "obeys_injection")
	require.Len(t, d.Candidates, 2)
	assertNeverAccepted(t, d)

	obeyed := d.Candidates[0]
	assert.Contains(t, flagCodes(obeyed), FlagInstructionLikeText)
	assert.Contains(t, flagCodes(obeyed), FlagOwnerNotSupported, "SPEAKER_00 never took this on")
	assert.Contains(t, flagCodes(obeyed), FlagCommitmentNotExplicit)
	assert.Equal(t, "SPEAKER_01", obeyed.Speaker)

	claim := d.Candidates[1]
	assert.Equal(t, VerificationUnverified, claim.VerificationStatus)
}

func TestConversationWithNoActions(t *testing.T) {
	d, tr := draftFor(t, "no_actions.json", "correct")
	assert.Empty(t, d.Candidates)
	assert.Equal(t, Report{}, d.Report)
	assert.Equal(t, tr.SHA256, d.TranscriptSHA256)
	assert.Equal(t, 3, d.SegmentCount)
}

func TestQuoteMatchingRules(t *testing.T) {
	tr := &Transcript{byID: map[string]int{}}
	for i, s := range []Segment{
		{Speaker: "SPEAKER_00", Text: "I will call the supplier,"},
		{Speaker: "SPEAKER_00", Text: "by Friday at the latest."},
		{Speaker: "SPEAKER_01", Text: "Unrelated."},
		{Speaker: "SPEAKER_00", Text: "Friday works."},
	} {
		s.ID, s.Index = "S"+string(rune('1'+i)), i
		tr.byID[s.ID] = i
		tr.Segments = append(tr.Segments, s)
	}
	run := func(segs []string, quote string) bool {
		var cited []Segment
		for _, id := range segs {
			s, _ := tr.Segment(id)
			cited = append(cited, s)
		}
		ok, _ := matchInRuns(quoteParts(quote), cited)
		return ok
	}
	assert.True(t, run([]string{"S1", "S2"}, "I will call the supplier by Friday"), "adjacent segments may be joined")
	assert.True(t, run([]string{"S1"}, "“I will call the supplier.”"), "case, quotes and punctuation are ignored")
	assert.True(t, run([]string{"S1", "S2"}, "I will call ... at the latest"), "ellipsis gaps keep word order")
	assert.False(t, run([]string{"S1"}, "I'll call the supplier"), "contractions are different words")
	assert.False(t, run([]string{"S1", "S4"}, "the supplier Friday works"), "non-adjacent segments are never joined")
	assert.False(t, run([]string{"S2"}, "at the latest ... by Friday"), "ellipsis parts must stay in order")

	assert.Equal(t, []string{"S3", "S12", "S7"}, canonicalSegmentIDs([]string{"s03", "[S12]", "7", "S3"}))
	assert.Equal(t, normLabel("SPEAKER_00"), normLabel("Speaker 0"))
	assert.NotEqual(t, normLabel("SPEAKER_00"), normLabel("SPEAKER_01"))
	assert.Equal(t, []string{"300", "mm", "clearance"}, words("300mm clearance"))
}

// Found by review: "-5 kPa" matched "+5 kPa" and "1/2 inch" matched "1.2 inch"
// because signs and number separators were dropped as punctuation. A quote
// that changes a number must never count as found.
func TestQuoteMatchingKeepsNumbersExact(t *testing.T) {
	cases := []struct {
		segment, quote string
		want           bool
	}{
		{"Set the regulator to -5 kPa.", "+5 kPa", false},
		{"Set the regulator to -5 kPa.", "to 5 kPa", false},
		{"Set the regulator to -5 kPa.", "the regulator to 5", false},
		{"Set the regulator to +5 kPa.", "-5 kPa", false},
		{"Set the regulator to -5 kPa.", "-5 kPa", true},
		{"Set the regulator to -5 kPa.", "to \u22125 kPa", true}, // Unicode minus sign
		{"Use a 1/2 inch line.", "a 1.2 inch line", false},
		{"Use a 1.2 inch line.", "a 1/2 inch line", false},
		{"Use a 1.2 inch line.", "a 12 inch line", false},
		{"Use a 1/2 inch line.", "a \u00bd inch line", true}, // "½"
		{"Use a 1\u00bd inch line.", "a 1 1/2 inch line", true},
		{"Use a 1\u00bd inch line.", "a 11/2 inch line", false},
		{"Leave a .5 inch gap.", "a 5 inch gap", false},
		{"Clause 7.2.3 applies here.", "Clause 7.2 applies", false},
		{"Clause 7.2.3 applies here.", "clause 7.2.3 applies", true},
		{"Order 1,200 litres.", "Order 1200 litres", false},
		{"Order 1.200 litres.", "Order 1,200 litres", false},
		{"A 5% slope.", "A 5 slope", false},
		{"A 5 % slope.", "A 5 slope", false},
		{"Keep it under 5 kPa.", "under <5 kPa", false},
		{"The budget is $500.", "The budget is €500", false},
		{"Plan for 5+ years.", "Plan for 5 years", false},
		{"Between 5\u201310 kPa.", "between 5-10 kPa", true}, // en dash range
		{"Meet at 3:30 then.", "meet at 3:30", true},
		{"Allow 300mm clearance.", "allow 300 mm clearance", true},
		{"Set it to 5.", "set it to 5", true},
	}
	for _, tc := range cases {
		seg := Segment{ID: "S1", Speaker: "SPEAKER_00", Text: tc.segment}
		got, _ := matchInRuns(quoteParts(tc.quote), []Segment{seg})
		assert.Equal(t, tc.want, got, "quote %q in segment %q", tc.quote, tc.segment)
	}

	assert.Equal(t, []string{"-5", "kpa"}, words("\u22125 kPa"))
	assert.Equal(t, []string{"1/2", "inch"}, words("\u00bd inch"))
	assert.Equal(t, []string{"clause", "7.2.3", "b"}, words("Clause 7.2.3(b)."))
	assert.Equal(t, []string{"5", "year", "old", "covid", "19"}, words("5-year-old COVID-19"), "hyphens next to letters are not signs")
	assert.Equal(t, []string{"between", "-5", "and", "-10"}, words("between -5 and -10"))

	raw := `{"segments":[
		{"start":0,"end":4,"text":"Set the regulator to -5 kPa.","speaker":"SPEAKER_00"},
		{"start":4,"end":6,"text":"Okay.","speaker":"SPEAKER_01"}]}`
	tr, err := ParseStoredTranscript(raw)
	require.NoError(t, err)
	cands, _ := Validate(&RawDraft{Items: []RawItem{
		{Kind: KindTechnicalClaim, Text: "Regulator setting", SegmentIDs: []string{"S1"}, Quote: "Set the regulator to +5 kPa"},
		{Kind: KindTechnicalClaim, Text: "Regulator setting", SegmentIDs: []string{"S1"}, Quote: "Set the regulator to -5 kPa"},
	}}, tr)
	require.Len(t, cands, 2)
	assert.False(t, cands[0].QuoteMatched, "a changed sign is not the transcript's quote")
	assert.Contains(t, flagCodes(cands[0]), FlagQuoteNotInCitedSegments)
	assert.Equal(t, VerificationUnverified, cands[0].VerificationStatus)
	assert.True(t, cands[1].QuoteMatched)
	assert.Equal(t, VerificationUnverified, cands[1].VerificationStatus, "a matched quote still proves only that it was said")
}

// Found by review: "\u2212 5 kPa" (minus sign, space, digit) matched "5 kPa" with
// no warning, because a sign separated from its digits by a space was dropped.
// The same applied to other symbols set apart from a number ("< 5", "5 %").
func TestQuoteMatchingKeepsSpacedSignsAndSymbols(t *testing.T) {
	cases := []struct {
		segment, quote string
		want           bool
	}{
		{"Set the regulator to \u2212 5 kPa.", "to 5 kPa", false},
		{"Set the regulator to \u2212 5 kPa.", "the regulator to 5", false},
		{"Set the regulator to \u2212 5 kPa.", "5 kPa", false},
		{"Set the regulator to - 5 kPa.", "to 5 kPa", false},
		{"Set the regulator to 5 kPa.", "to \u2212 5 kPa", false},
		{"Set the regulator to 5 kPa.", "to - 5 kPa", false},
		{"Set the regulator to \u2212 5 kPa.", "to -5 kPa", true},
		{"Set the regulator to -5 kPa.", "to \u2212 5 kPa", true},
		{"The reading - 5 kPa - was fine.", "the reading 5 kPa", false}, // a dash that may be a minus is never dropped silently
		{"Keep it < 5 kPa.", "it 5 kPa", false},
		{"Keep it < 5 kPa.", "keep it <5 kPa", true},
		{"Keep it < 5 kPa.", "5 kPa", false},
		{"Tolerance \u00b1 0.5 kPa.", "0.5 kPa", false},
		{"Tolerance \u00b1 0.5 kPa.", "tolerance 0.5 kPa", false},
		{"The budget is $ 500.", "budget is 500", false},
		{"The budget is $ 500.", "budget is $500", true},
		{"A 5 % slope.", "a 5% slope", true},
		{"A 5 % slope.", "a 5", false},
		{"Hold 5 - 10 kPa.", "10 kPa", false},
		{"Hold 5 - 10 kPa.", "hold 5-10 kPa", true},
		{"Set it between -5 and -10.", "between -5 and -10", true},
	}
	for _, tc := range cases {
		seg := Segment{ID: "S1", Speaker: "SPEAKER_00", Text: tc.segment}
		got, _ := matchInRuns(quoteParts(tc.quote), []Segment{seg})
		assert.Equal(t, tc.want, got, "quote %q in segment %q", tc.quote, tc.segment)
	}

	assert.Equal(t, []string{"to", "-5", "kpa"}, words("to \u2212 5 kPa"))
	assert.Equal(t, []string{"5-10", "kpa"}, words("5 - 10 kPa"), "a dash between two numbers is a range, not a sign")
	assert.Equal(t, []string{"x", "5", "x", "-5"}, words("x-5 x - 5"), "a hyphen inside a word is not a sign")

	raw := `{"segments":[
		{"start":0,"end":4,"text":"Then set the bypass to \u2212 5 kPa.","speaker":"SPEAKER_00"},
		{"start":4,"end":6,"text":"Okay.","speaker":"SPEAKER_01"}]}`
	tr, err := ParseStoredTranscript(raw)
	require.NoError(t, err)
	cands, _ := Validate(&RawDraft{Items: []RawItem{
		{Kind: KindTechnicalClaim, Text: "Bypass setting", SegmentIDs: []string{"S1"}, Quote: "set the bypass to 5 kPa"},
	}}, tr)
	require.Len(t, cands, 1)
	assert.False(t, cands[0].QuoteMatched, "a dropped minus sign is not the transcript's quote")
	assert.Contains(t, flagCodes(cands[0]), FlagQuoteNotInCitedSegments)
	assert.Equal(t, VerificationUnverified, cands[0].VerificationStatus)
}

// Requested by review: "5 kPa" is literally inside "minus 5 kPa", so the
// quote is found, but leaving out "minus" changes the number. The match stays;
// a separate flag tells the reviewer what was left out.
func TestQuoteLeavingOutANumberQualifierIsFlagged(t *testing.T) {
	raw := `{"segments":[
		{"start":0,"end":3,"text":"Set the regulator to minus 5 kPa.","speaker":"SPEAKER_00"},
		{"start":3,"end":6,"text":"Keep the line at not more than 5 kPa.","speaker":"SPEAKER_00"},
		{"start":6,"end":9,"text":"Allow up to 5 mm of play.","speaker":"SPEAKER_01"},
		{"start":9,"end":12,"text":"Hold it at 7 kPa or less.","speaker":"SPEAKER_01"},
		{"start":12,"end":15,"text":"Set the bypass to 3 kPa.","speaker":"SPEAKER_00"},
		{"start":15,"end":17,"text":"It dropped to minus","speaker":"SPEAKER_00"},
		{"start":17,"end":19,"text":"five degrees overnight.","speaker":"SPEAKER_00"},
		{"start":19,"end":20,"text":"Was it minus?","speaker":"SPEAKER_01"},
		{"start":20,"end":21,"text":"4 degrees.","speaker":"SPEAKER_00"},
		{"start":21,"end":24,"text":"Tolerance is plus or minus 2 mm.","speaker":"SPEAKER_00"}]}`
	tr, err := ParseStoredTranscript(raw)
	require.NoError(t, err)
	cases := []struct {
		seg, quote string
		left       string // the words the flag must name; "" means no flag
	}{
		{"S1", "5 kPa", `"minus" just before "5" (S1)`},
		{"S1", "regulator to minus 5 kPa", ""},
		{"S2", "more than 5 kPa", `"not" just before "more than 5" (S2)`},
		{"S2", "5 kPa", `"not more than" just before "5" (S2)`},
		{"S3", "to 5 mm of play", `"up" just before "to 5" (S3)`},
		{"S4", "Hold it at 7 kPa", `"or less" right after "7 kpa" (S4)`},
		{"S4", "hold it at 7", `"kpa or less" right after "7" (S4)`},
		{"S4", "7 kPa or less", ""},
		{"S5", "set the bypass to 3 kPa", ""},
		{"S7", "five degrees overnight", `"minus" just before "five" (S6, not cited)`},
		{"S9", "4 degrees", ""}, // "minus?" was another speaker's question
		{"S10", "minus 2 mm", `"plus or" just before "minus 2" (S10)`},
	}
	var items []RawItem
	for _, tc := range cases {
		items = append(items, RawItem{Kind: KindTechnicalClaim, Text: "x", SegmentIDs: []string{tc.seg}, Quote: tc.quote})
	}
	cands, _ := Validate(&RawDraft{Items: items}, tr)
	require.Len(t, cands, len(cases))
	for i, tc := range cases {
		c := cands[i]
		assert.True(t, c.QuoteMatched, "quote %q stays a literal match", tc.quote)
		assert.NotContains(t, flagCodes(c), FlagQuoteNotInCitedSegments, "quote %q", tc.quote)
		var msg string
		for _, f := range c.Flags {
			if f.Code == FlagNumberQualifierOmitted {
				msg = f.Message
			}
		}
		if tc.left == "" {
			assert.Empty(t, msg, "quote %q should not be flagged", tc.quote)
			continue
		}
		assert.Contains(t, msg, "The quote leaves out "+tc.left+".", "quote %q", tc.quote)
	}
	assert.Equal(t, "S6", cands[9].Flags[len(cands[9].Flags)-1].SegmentID, "the flag points at the segment holding the left-out word")
}

// Cases found by independent review: quotes that hide a refusal or question,
// verbatim deadlines and owners borrowed from a neighbouring cited segment.
func TestReviewFoundEvasions(t *testing.T) {
	raw := `{"segments":[
		{"start":0,"end":3,"text":"I will not approve the budget.","speaker":"SPEAKER_00"},
		{"start":3,"end":7,"text":"We agreed we will not go ahead with the purchase.","speaker":"SPEAKER_01"},
		{"start":7,"end":9,"text":"Is the budget approved?","speaker":"SPEAKER_00"},
		{"start":9,"end":10,"text":"No idea.","speaker":"SPEAKER_01"},
		{"start":10,"end":12,"text":"Budget approved for the fittings then.","speaker":"SPEAKER_00"},
		{"start":12,"end":15,"text":"I'll send the drawings by Monday.","speaker":"SPEAKER_01"},
		{"start":15,"end":16,"text":"Hmm, okay.","speaker":"SPEAKER_00"},
		{"start":16,"end":19,"text":"Actually not Monday then, Tuesday.","speaker":"SPEAKER_01"}]}`
	tr, err := ParseStoredTranscript(raw)
	require.NoError(t, err)
	item := func(kind string, ids []string, quote, owner, due string) RawItem {
		return RawItem{Kind: kind, Text: "x", SegmentIDs: ids, Quote: quote, Owner: owner, Due: due}
	}
	cands, _ := Validate(&RawDraft{Items: []RawItem{
		item(KindCommitment, []string{"S1"}, "I will ... approve the budget", "SPEAKER_00", "Not stated"),
		item(KindDecision, []string{"S2"}, "We agreed ... go ahead with the purchase", "Not stated", "Not stated"),
		item(KindDecision, []string{"S3", "S4", "S5"}, "the budget approved ... budget approved for the fittings", "Not stated", "Not stated"),
		item(KindCommitment, []string{"S6"}, "I'll send the drawings by Monday", "SPEAKER_01", "by Monday"),
		item(KindCommitment, []string{"S6", "S7"}, "I'll send the drawings", "SPEAKER_00", "Not stated"),
	}}, tr)
	require.Len(t, cands, 5)

	assert.True(t, cands[0].QuoteMatched)
	assert.Contains(t, flagCodes(cands[0]), FlagQuoteHasGap)
	assert.Contains(t, flagCodes(cands[0]), FlagCommitmentNotExplicit, "the skipped 'not' is in the segment")

	assert.Contains(t, flagCodes(cands[1]), FlagQuoteHasGap)
	assert.Contains(t, flagCodes(cands[1]), FlagDecisionNotExplicit)

	assert.Contains(t, flagCodes(cands[2]), FlagQuoteHasGap)
	assert.Contains(t, flagCodes(cands[2]), FlagDecisionNotExplicit, "the stitched span contains a question")

	assert.Contains(t, flagCodes(cands[3]), FlagPossibleLaterCorrection, "'by Monday' is corrected by 'not Monday'")
	assert.NotContains(t, flagCodes(cands[3]), FlagDueNotInEvidence)

	assert.Contains(t, flagCodes(cands[4]), FlagOwnerNotSupported, "SPEAKER_00 only said 'Hmm, okay.'")
	assert.Equal(t, []string{"monday"}, coreDueWords("no later than Monday"))
	assert.Equal(t, []string{"next", "quarter"}, coreDueWords("next quarter"))
}
