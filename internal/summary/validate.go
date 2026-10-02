package summary

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Validate checks every model candidate against the stored transcript.
//
// The server, not the model, decides which segments exist and supplies their
// speakers, timestamps and text. Candidates without a usable reference or quote
// are rejected (kept for transparency). Everything else leaves as a
// "needs_review" candidate with flags; no candidate is ever accepted and no
// claim is ever verified here.
func Validate(raw *RawDraft, t *Transcript) ([]Candidate, Report) {
	out := make([]Candidate, 0, len(raw.Items))
	var rep Report
	for i, item := range raw.Items {
		c := validateItem(fmt.Sprintf("C%d", i+1), item, t)
		rep.Items++
		if c.Rejected {
			rep.Rejected++
		} else {
			rep.Usable++
		}
		if len(c.Flags) > 0 {
			rep.Flagged++
		}
		if c.QuoteMatched {
			rep.Matched++
		}
		out = append(out, c)
	}
	return out, rep
}

func (c *Candidate) flag(code, segmentID, format string, args ...any) {
	for _, f := range c.Flags {
		if f.Code == code && f.SegmentID == segmentID {
			return
		}
	}
	c.Flags = append(c.Flags, Flag{Code: code, SegmentID: segmentID, Message: fmt.Sprintf(format, args...)})
}

func validateItem(id string, item RawItem, t *Transcript) Candidate {
	c := Candidate{
		ID:           id,
		Kind:         normalizeKind(item.Kind),
		Text:         collapse(item.Text),
		Quote:        collapse(item.Quote),
		ModelSpeaker: collapse(item.Speaker),
		Owner:        normalizeNotStated(item.Owner),
		Due:          normalizeNotStated(item.Due),
		ReviewState:  ReviewNeedsReview,
		Evidence:     []Evidence{},
		Flags:        []Flag{},
	}
	if item.Malformed {
		c.Rejected = true
		c.flag(FlagMalformedItem, "", "The model wrote this item in a form that could not be read.")
		return c
	}
	if !allowedKinds[c.Kind] {
		c.Rejected = true
		c.flag(FlagUnknownKind, "", "Kind %q is not one of decision, commitment, suggestion, question or technical_claim.", item.Kind)
	}

	// The model's segment IDs only select segments; everything shown as
	// evidence is copied from the stored transcript.
	var cited []Segment
	ids := canonicalSegmentIDs(item.SegmentIDs)
	if len(ids) == 0 {
		c.Rejected = true
		c.flag(FlagNoSegmentReference, "", "No transcript segment was cited.")
	}
	for _, sid := range ids {
		seg, ok := t.Segment(sid)
		if !ok {
			c.flag(FlagSegmentNotFound, sid, "Cited segment %s does not exist in the stored transcript.", sid)
			continue
		}
		cited = append(cited, seg)
	}
	sort.Slice(cited, func(i, j int) bool { return cited[i].Index < cited[j].Index })
	if len(ids) > 0 && len(cited) == 0 {
		c.Rejected = true
	}
	parts := quoteParts(c.Quote)
	if len(parts) == 0 {
		c.Rejected = true
		c.flag(FlagMissingQuote, "", "No quotation was supplied, so the item cannot be checked against the transcript.")
	}

	if c.Rejected {
		c.Evidence = evidenceFor(cited, nil)
		if len(cited) > 0 {
			c.Speaker = cited[0].Speaker
		}
		applyTechnicalStatus(&c, cited)
		return c
	}

	matched, covered := matchInRuns(parts, cited)
	c.QuoteMatched = matched
	c.Evidence = evidenceFor(cited, covered)
	if len(parts) > 1 {
		c.flag(FlagQuoteHasGap, "", "The quote leaves out words (\"...\"); omitted words can change the meaning, so read the full segment.")
	}
	speakerSegs := cited
	if matched {
		speakerSegs = covered
		speakers := distinctSpeakers(covered)
		c.Speaker = strings.Join(speakers, ", ")
		if len(speakers) > 1 {
			c.flag(FlagQuoteSpansSpeakers, "", "The quoted words run across segments from different speakers (%s).", c.Speaker)
		}
	} else {
		c.Speaker = cited[0].Speaker
		if ok, found := matchInRuns(parts, t.Segments); ok {
			loc := segmentRange(found)
			c.flag(FlagQuoteNotInCitedSegments, found[0].ID,
				"The quoted words do not appear in the cited segment(s); they appear in %s (%s). The reference or speaker may be wrong.",
				loc, strings.Join(distinctSpeakers(found), ", "))
		} else {
			c.flag(FlagQuoteNotInCitedSegments, "",
				"The quoted words do not appear in the cited segment(s) or anywhere in the transcript. This may be a paraphrase presented as a quotation.")
		}
	}

	if c.ModelSpeaker != "" {
		want := normLabel(c.ModelSpeaker)
		ok := false
		for _, s := range speakerSegs {
			if normLabel(s.Speaker) == want {
				ok = true
			}
		}
		if !ok {
			c.flag(FlagSpeakerMismatch, speakerSegs[0].ID,
				"The model attributed this to %s, but the transcript attributes %s to %s.",
				c.ModelSpeaker, speakerSegs[0].ID, strings.Join(distinctSpeakers(speakerSegs), ", "))
		}
	}

	for _, s := range cited {
		if !s.TimesValid() {
			c.flag(FlagInvalidSourceTimestamp, s.ID, "Segment %s has invalid source timestamps.", s.ID)
		}
	}

	checkOwner(&c, cited, speakerSegs, t)
	checkDue(&c, cited, t)
	checkKindLanguage(&c, speakerSegs)
	checkLaterCorrection(&c, cited, t)
	for _, s := range cited {
		if matchesAny(instructionPatterns, s.Text) {
			c.flag(FlagInstructionLikeText, s.ID,
				"Segment %s contains text addressed to an AI or instructions; it is treated as speech, never as an instruction.", s.ID)
		}
	}
	applyTechnicalStatus(&c, cited)
	return c
}

func normalizeKind(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	k = strings.NewReplacer(" ", "_", "-", "_").Replace(k)
	return k
}

var segmentIDPattern = regexp.MustCompile(`^S?0*(\d+)$`)

// canonicalSegmentIDs accepts "S3", "s03", "[S3]" or "3" and removes duplicates.
func canonicalSegmentIDs(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range raw {
		id := strings.ToUpper(strings.Trim(strings.TrimSpace(r), "[]() "))
		if m := segmentIDPattern.FindStringSubmatch(id); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				id = "S" + strconv.Itoa(n)
			}
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// matchInRuns looks for all quote parts, in order, inside one run of
// consecutive segments. A quote may continue across a segment boundary (ASR
// often splits sentences) but never jumps between non-adjacent segments.
func matchInRuns(parts [][]string, segs []Segment) (bool, []Segment) {
	for _, run := range consecutiveRuns(segs) {
		var hay []string
		var owner []int // index into run for each word
		for i, s := range run {
			for _, w := range words(s.Text) {
				hay = append(hay, w)
				owner = append(owner, i)
			}
		}
		pos, first, last := 0, -1, -1
		ok := true
		for _, p := range parts {
			at := indexSeq(hay, p, pos)
			if at < 0 {
				ok = false
				break
			}
			if first < 0 {
				first = at
			}
			last = at + len(p) - 1
			pos = at + len(p)
		}
		if ok && first >= 0 {
			return true, run[owner[first] : owner[last]+1]
		}
	}
	return false, nil
}

func consecutiveRuns(segs []Segment) [][]Segment {
	var runs [][]Segment
	for i, s := range segs {
		if i == 0 || s.Index != segs[i-1].Index+1 {
			runs = append(runs, []Segment{s})
			continue
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], s)
	}
	return runs
}

func evidenceFor(cited, covered []Segment) []Evidence {
	in := map[string]bool{}
	for _, s := range covered {
		in[s.ID] = true
	}
	ev := make([]Evidence, 0, len(cited))
	for _, s := range cited {
		ev = append(ev, Evidence{SegmentID: s.ID, Speaker: s.Speaker, Start: s.Start, End: s.End, Text: s.Text, ContainsQuote: in[s.ID]})
	}
	return ev
}

func distinctSpeakers(segs []Segment) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range segs {
		if !seen[s.Speaker] {
			seen[s.Speaker] = true
			out = append(out, s.Speaker)
		}
	}
	return out
}

func segmentRange(segs []Segment) string {
	if len(segs) == 1 {
		return segs[0].ID
	}
	return segs[0].ID + "-" + segs[len(segs)-1].ID
}

// checkOwner: an owner that is a speaker label must be the speaker of the
// quoted words (quoteSegs); any other owner must appear in the cited text.
func checkOwner(c *Candidate, cited, quoteSegs []Segment, t *Transcript) {
	if c.Owner == NotStated {
		return
	}
	want := normLabel(c.Owner)
	for _, label := range t.Speakers() {
		if normLabel(label) != want {
			continue
		}
		for _, s := range quoteSegs {
			if normLabel(s.Speaker) == want {
				return
			}
		}
		c.flag(FlagOwnerNotSupported, "",
			"Owner %s is not the speaker of the quoted words; nothing quoted shows %s taking this on.", c.Owner, c.Owner)
		return
	}
	ownerWords := words(c.Owner)
	for _, s := range cited {
		if hasSeq(words(s.Text), ownerWords) {
			return
		}
	}
	c.flag(FlagOwnerNotInEvidence, "", "Owner %q does not appear in the cited segment(s).", c.Owner)
}

// checkDue requires the deadline wording in the cited text and looks for
// corrections ("Tuesday, not Monday") in the cited and later segments.
func checkDue(c *Candidate, cited []Segment, t *Transcript) {
	if c.Due == NotStated {
		return
	}
	due := coreDueWords(c.Due)
	if len(due) == 0 {
		return
	}
	found := false
	for _, s := range cited {
		w := words(s.Text)
		if hasSeq(w, due) {
			found = true
		}
		if contradicts(w, due) {
			c.flag(FlagDueContradicted, s.ID, "Cited segment %s itself contradicts the deadline %q.", s.ID, c.Due)
		}
	}
	if !found {
		c.flag(FlagDueNotInEvidence, "", "Deadline %q does not appear in the cited segment(s).", c.Due)
	}
	last := cited[len(cited)-1].Index
	for _, s := range t.Segments[last+1:] {
		w := words(s.Text)
		if hasSeq(w, due) && (contradicts(w, due) || hasAny(w, correctionCues)) {
			c.flag(FlagPossibleLaterCorrection, s.ID,
				"Later segment %s mentions %q with correction wording; the deadline may have changed.", s.ID, c.Due)
			return
		}
	}
}

// checkLaterCorrection looks a few segments ahead for the same speaker
// correcting themselves ("correction", "scratch that", "make that").
func checkLaterCorrection(c *Candidate, cited []Segment, t *Transcript) {
	if c.Kind != KindCommitment && c.Kind != KindDecision {
		return
	}
	speakers := map[string]bool{}
	for _, s := range cited {
		speakers[s.Speaker] = true
	}
	last := cited[len(cited)-1].Index
	for i := last + 1; i < len(t.Segments) && i <= last+5; i++ {
		s := t.Segments[i]
		if speakers[s.Speaker] && hasAny(words(s.Text), strongCorrectionCues) {
			c.flag(FlagPossibleLaterCorrection, s.ID,
				"Segment %s by the same speaker contains correction wording; check whether this item changed.", s.ID)
			return
		}
	}
}

// checkKindLanguage flags commitments and decisions whose quote lacks explicit
// wording. Negation and questions are also looked for in the full text of the
// quoted segments, because a quote can stop short of, or skip over, a refusal.
// This is a lexical signal only.
func checkKindLanguage(c *Candidate, quoteSegs []Segment) {
	q := words(c.Quote)
	var seg []string
	segQuestion := false
	for _, s := range quoteSegs {
		seg = append(seg, words(s.Text)...)
		segQuestion = segQuestion || strings.Contains(s.Text, "?")
	}
	switch c.Kind {
	case KindCommitment:
		switch {
		case hasAny(q, commitmentCues) && (hasAny(q, negationCues) || hasAny(seg, negationCues)):
			c.flag(FlagCommitmentNotExplicit, "", "The quote or its segment contains negative wording; it may be a refusal rather than a commitment.")
		case hasAny(q, commitmentCues):
		case hasAny(q, suggestionCues):
			c.flag(FlagCommitmentNotExplicit, "", "The quote reads like a suggestion or request, not an explicit commitment.")
		default:
			c.flag(FlagCommitmentNotExplicit, "", "The quote has no explicit first-person commitment or accepted assignment.")
		}
	case KindDecision:
		switch {
		case strings.Contains(c.Quote, "?"):
			c.flag(FlagDecisionNotExplicit, "", "The quote is a question, not an agreement.")
		case hasAny(q, suggestionCues) || hasAny(q, negationCues) || hasAny(seg, negationCues):
			c.flag(FlagDecisionNotExplicit, "", "The quote or its segment reads like a proposal or a refusal, not an explicit agreement.")
		case segQuestion:
			c.flag(FlagDecisionNotExplicit, "", "The quoted segment contains a question; check that this was actually agreed.")
		case hasAny(q, strongDecisionCues):
		case hasAny(q, weakDecisionCues):
		default:
			c.flag(FlagDecisionNotExplicit, "", "The quote has no explicit agreement wording.")
		}
	}
}

// applyTechnicalStatus marks technical items UNVERIFIED. There is no code path
// that sets any other verification state.
func applyTechnicalStatus(c *Candidate, cited []Segment) {
	technical := c.Kind == KindTechnicalClaim || matchesAny(technicalPatterns, c.Quote) || matchesAny(technicalPatterns, c.Text)
	if !technical {
		return
	}
	c.VerificationStatus = VerificationUnverified
	if c.Kind != KindTechnicalClaim {
		c.flag(FlagTechnicalContent, "", "Contains technical, code or regulatory content, which stays UNVERIFIED.")
	}
	asserted := verificationPattern.MatchString(c.Quote)
	for _, s := range cited {
		if verificationPattern.MatchString(s.Text) {
			asserted = true
		}
	}
	if asserted {
		c.flag(FlagVerificationAsserted, "",
			"A speaker says this was verified or checked. A spoken assertion, agreement or repetition does not verify a claim; it remains UNVERIFIED.")
	}
}
