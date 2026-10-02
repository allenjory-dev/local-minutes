package summary

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// words splits text into lowercase comparison words. Case and punctuation are
// ignored; spelling and word order are not, so "I'll" never matches "I will".
// Letter/digit boundaries split words ("300mm" -> "300", "mm") so ASR spacing
// differences do not decide a match.
func words(s string) []string {
	s = norm.NFKC.String(s)
	s = strings.NewReplacer("’", "'", "‘", "'", "ʼ", "'", "`", "'", "´", "'").Replace(s)
	s = strings.ToLower(s)
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			if w := strings.Trim(string(cur), "'"); w != "" {
				out = append(out, w)
			}
			cur = cur[:0]
		}
	}
	for _, r := range s {
		isLetter := unicode.IsLetter(r) || r == '\''
		isDigit := unicode.IsDigit(r)
		if !isLetter && !isDigit {
			flush()
			continue
		}
		if len(cur) > 0 {
			prevDigit := unicode.IsDigit(cur[len(cur)-1])
			if prevDigit != isDigit {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

// quoteParts splits a quotation at ellipses. Each part must appear, in order,
// in the cited text. Empty parts (a leading or trailing "...") are dropped.
func quoteParts(quote string) [][]string {
	q := norm.NFKC.String(quote) // "…" becomes "..."
	var parts [][]string
	for _, p := range strings.Split(q, "...") {
		if w := words(p); len(w) > 0 {
			parts = append(parts, w)
		}
	}
	return parts
}

// indexSeq returns the first index >= from where needle occurs in hay, or -1.
func indexSeq(hay, needle []string, from int) int {
	if len(needle) == 0 {
		return -1
	}
outer:
	for i := from; i+len(needle) <= len(hay); i++ {
		for j := range needle {
			if hay[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

func hasSeq(hay, needle []string) bool { return indexSeq(hay, needle, 0) >= 0 }

func hasAny(hay []string, cues [][]string) bool {
	for _, c := range cues {
		if hasSeq(hay, c) {
			return true
		}
	}
	return false
}

// normLabel compares speaker labels loosely: case and punctuation are
// ignored and leading zeros dropped, so "SPEAKER_00" equals "Speaker 0".
func normLabel(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		if n, err := strconv.Atoi(w); err == nil {
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteString(w)
		}
	}
	return b.String()
}

// collapse turns model text into a single display line.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var notStatedValues = map[string]bool{
	"": true, "not stated": true, "none": true, "n/a": true, "na": true, "unknown": true,
	"unassigned": true, "not specified": true, "unspecified": true, "tbd": true, "-": true,
	"—": true, "null": true, "nobody": true, "no one": true, "not mentioned": true,
	"not given": true, "not provided": true,
}

// normalizeNotStated maps empty or placeholder owners/deadlines to "Not stated".
func normalizeNotStated(s string) string {
	s = collapse(s)
	if notStatedValues[strings.ToLower(strings.Trim(s, " ."))] {
		return NotStated
	}
	return s
}

// Lexical cues. These are review signals, not semantic understanding: they
// catch common phrasing (for example "someone should" presented as a
// commitment) and miss anything phrased differently.
var (
	commitmentCues = [][]string{
		{"i", "will"}, {"i'll"}, {"i", "shall"}, {"we", "will"}, {"we'll"},
		{"i", "can", "do"}, {"i", "can", "take"}, {"i", "am", "going", "to"}, {"i'm", "going", "to"},
		{"i'm", "on", "it"}, {"let", "me"}, {"i", "promise"}, {"i", "commit"}, {"will", "do"},
		{"leave", "it", "with", "me"}, {"i", "take"}, {"consider", "it", "done"}, {"i", "agree", "to"},
	}
	negationCues = [][]string{
		{"won't"}, {"will", "not"}, {"can't"}, {"cannot"}, {"can", "not"}, {"not", "going", "to"},
		{"i'm", "not"}, {"we're", "not"}, {"we", "are", "not"}, {"i", "am", "not"}, {"don't"}, {"do", "not"},
		{"no", "budget"}, {"not", "approved"}, {"rejected"}, {"declined"},
	}
	suggestionCues = [][]string{
		{"should"}, {"could"}, {"maybe"}, {"might"}, {"perhaps"}, {"someone"}, {"somebody"},
		{"anyone"}, {"anybody"}, {"ought"}, {"how", "about"}, {"what", "if"}, {"why", "don't"},
		{"would", "be", "good"}, {"would", "be", "nice"}, {"suggest"}, {"propose"}, {"proposal"},
	}
	strongDecisionCues = [][]string{
		{"agreed"}, {"we", "agree"}, {"decided"}, {"decision", "is"}, {"let's", "go", "with"},
		{"we'll", "go", "with"}, {"we", "will", "go", "with"}, {"we're", "going", "with"},
		{"we", "are", "going", "with"}, {"go", "ahead"}, {"approved"}, {"settled"}, {"that's", "final"},
	}
	weakDecisionCues = [][]string{{"we", "will"}, {"we'll"}, {"let's"}}
	correctionCues   = [][]string{
		{"correction"}, {"actually"}, {"sorry"}, {"instead"}, {"rather"}, {"meant"}, {"changed"},
		{"change"}, {"moved"}, {"postponed"}, {"rescheduled"}, {"not"},
	}
	strongCorrectionCues = [][]string{
		{"correction"}, {"scratch", "that"}, {"i", "meant"}, {"let", "me", "correct"},
		{"make", "that"}, {"change", "that", "to"}, {"not", "what", "i", "meant"},
	}
)

var dueLeadWords = map[string]bool{
	"by": true, "on": true, "before": true, "until": true, "till": true, "at": true,
	"in": true, "the": true, "this": true, "due": true, "no": true, "later": true, "than": true,
}

// coreDueWords drops leading words such as "by", "on" or "no later than", so
// "by Monday" is matched as "Monday" in the evidence and in later corrections.
func coreDueWords(due string) []string {
	w := words(due)
	for len(w) > 1 && dueLeadWords[w[0]] {
		w = w[1:]
	}
	return w
}

// contradicts reports phrasing such as "not Monday" or "instead of Monday".
func contradicts(hay, term []string) bool {
	for _, prefix := range [][]string{{"not"}, {"instead", "of"}, {"rather", "than"}, {"no", "longer"}} {
		if hasSeq(hay, append(append([]string{}, prefix...), term...)) {
			return true
		}
	}
	return false
}

var (
	instructionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(ignore|disregard|forget|override)\b[^.?!]{0,60}\b(instructions?|rules|prompts?|everything above|previous|prior)\b`),
		regexp.MustCompile(`(?i)\b(system prompt|you are an ai|as an ai|ai assistant|language model|chatgpt|llm)\b`),
		regexp.MustCompile(`(?i)\bmark\b[^.?!]{0,60}\b(verified|approved|accepted|certified)\b`),
		regexp.MustCompile(`(?i)\b(new|updated|additional) instructions?\b`),
		regexp.MustCompile(`(?i)\badd (an |a new )?action items?\b`),
	}
	verificationPattern = regexp.MustCompile(`(?i)\b(verified|verify|confirmed|certified|double[- ]checked|checked|signed off|approved|according to the code|per the code|it'?s in the code)\b`)
	technicalPatterns   = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(clause|section|subsection|sentence|article|table|appendix|annex|code|codes|standard|regulation|bylaw|by-law|csa|b149|npc|nbc|nec|abc|iapmo|upc|ipc|ansi|nfpa|asme|astm)\b`),
		regexp.MustCompile(`(?i)\b\d+(?:[.,]\d+)?\s*(?:mm|cm|m|km|metres?|meters?|millimet(?:re|er)s?|centimet(?:re|er)s?|inch(?:es)?|in|ft|feet|foot|psi|psig|kpa|pa|bar|btu|btuh|mbh|kw|watts?|volts?|amps?|gpm|lpm|cfm|degrees?|percent)\b`),
		regexp.MustCompile(`\d\s*(?:%|°)`),
		regexp.MustCompile(`(?i)\b(pressure|clearance|diameter|setback|combustible|venting|fire[- ]rated|fire separation)\b`),
	}
)

func matchesAny(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}
