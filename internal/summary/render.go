package summary

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// RenderMeta describes how a draft was produced.
type RenderMeta struct {
	Model       string
	Provider    string
	GeneratedAt time.Time
}

var kindHeadings = map[string]string{
	KindDecision:       "Decisions (candidates - need review)",
	KindCommitment:     "Commitments (candidates - need review, not tasks)",
	KindSuggestion:     "Suggestions and proposals (not agreed)",
	KindQuestion:       "Questions",
	KindTechnicalClaim: "Technical, code and regulatory claims - UNVERIFIED",
}

// FormatTimestamp renders seconds as m:ss or h:mm:ss.
func FormatTimestamp(sec float64) string {
	if math.IsNaN(sec) || math.IsInf(sec, 0) || sec < 0 {
		return "time unavailable"
	}
	total := int(sec)
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// RenderMarkdown produces the saved, copyable text of an evidence-linked
// draft. Headings, notices, evidence and timestamps are written by the
// application; only the overview, item text and quotes come from the model.
func RenderMarkdown(d *StoredDraft, meta RenderMeta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "> **%s**\n>\n", DraftNotice)
	short := d.TranscriptSHA256
	if len(short) > 12 {
		short = short[:12]
	}
	fmt.Fprintf(&b, "> Evidence-linked draft generated %s by %s (%s) from the stored transcript (%d segments, sha256 %s). ",
		meta.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC"), mdText(meta.Model), mdText(meta.Provider), d.SegmentCount, short)
	fmt.Fprintf(&b, "%d candidates: %d with existing segment references (%d with the quote found in a cited segment), %d rejected by validation, %d flagged for review.\n\n",
		d.Report.Items, d.Report.Usable, d.Report.Matched, d.Report.Rejected, d.Report.Flagged)

	b.WriteString("## Overview (AI-written, not evidence-linked)\n\n")
	if ov := mdText(d.Overview); ov != "" {
		b.WriteString(ov + "\n\n")
	} else {
		b.WriteString("_No overview provided._\n\n")
	}

	for _, kind := range KindOrder {
		fmt.Fprintf(&b, "## %s\n\n", kindHeadings[kind])
		n := 0
		for _, c := range d.Candidates {
			if c.Kind == kind && !c.Rejected {
				writeCandidate(&b, c)
				n++
			}
		}
		if n == 0 {
			b.WriteString("_None extracted._\n\n")
		}
	}

	b.WriteString("## Rejected by validation\n\n")
	n := 0
	for _, c := range d.Candidates {
		if c.Rejected {
			writeCandidate(&b, c)
			n++
		}
	}
	if n == 0 {
		b.WriteString("_None._\n\n")
	}
	return b.String()
}

func writeCandidate(b *strings.Builder, c Candidate) {
	text := mdText(c.Text)
	if text == "" {
		text = "(no description)"
	}
	fmt.Fprintf(b, "- **%s** [%s] - %s", text, c.ID, "needs review")
	if c.Kind == KindCommitment || c.Kind == KindDecision {
		fmt.Fprintf(b, " - Owner: %s - Due: %s", mdText(c.Owner), mdText(c.Due))
	}
	if c.VerificationStatus != "" {
		fmt.Fprintf(b, " - %s", c.VerificationStatus)
	}
	if c.Rejected {
		fmt.Fprintf(b, " - kind: %s", mdText(c.Kind))
	}
	b.WriteString("\n")
	if c.Quote != "" {
		match := "quote matched in cited segment"
		if !c.QuoteMatched {
			match = "quote NOT found in cited segment"
		}
		fmt.Fprintf(b, "  - Quote (%s): \"%s\"\n", match, mdText(c.Quote))
	}
	for _, e := range c.Evidence {
		fmt.Fprintf(b, "  - Evidence %s - %s - %s-%s: \"%s\"\n",
			e.SegmentID, mdText(e.Speaker), FormatTimestamp(e.Start), FormatTimestamp(e.End), mdText(e.Text))
	}
	for _, f := range c.Flags {
		fmt.Fprintf(b, "  - Flag (%s): %s\n", f.Code, mdText(f.Message))
	}
	b.WriteString("\n")
}

var (
	mdInline    = strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "[", "\\[", "]", "\\]", "|", "\\|", "<", "&lt;", ">", "&gt;")
	mdListStart = regexp.MustCompile(`^(\d+)([.)])`)
)

// mdText puts model or transcript text on one line and escapes Markdown that
// could add headings, quote blocks, lists, links, emphasis or HTML to an
// exported draft (for example a fake notice). Underscores are left alone so
// speaker labels such as SPEAKER_00 stay readable.
func mdText(s string) string {
	s = mdInline.Replace(collapse(s))
	if s != "" && strings.ContainsRune("#+-=~", rune(s[0])) {
		s = "\\" + s
	}
	return mdListStart.ReplaceAllString(s, "$1\\$2")
}
