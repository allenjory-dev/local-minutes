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
//
// Numbers are the exception to "punctuation is ignored": every symbol that can
// change a number's meaning stays part of it, with or without a space in
// between (see numberEnd). So "-5" and "- 5" never match "+5" or "5", "1/2"
// never matches "1.2", and "7.2" never matches part of "7.2.3".
func words(s string) []string {
	rs := []rune(strings.ToLower(normalizeText(s)))
	var out []string
	for i := 0; i < len(rs); {
		if end := numberEnd(rs, i); end > i {
			out = append(out, numberWord(rs[i:end]))
			i = end
			continue
		}
		if unicode.IsLetter(rs[i]) || rs[i] == '\'' {
			j := i + 1
			for j < len(rs) && (unicode.IsLetter(rs[j]) || rs[j] == '\'') {
				j++
			}
			if w := strings.Trim(string(rs[i:j]), "'"); w != "" {
				out = append(out, w)
			}
			i = j
			continue
		}
		if isNumberSymbol(rs[i]) {
			out = append(out, string(rs[i])) // a "%" or "$" with no number next to it
		}
		i++
	}
	return out
}

var textReplacer = strings.NewReplacer(
	"\u2019", "'", "\u2018", "'", "\u02bc", "'", "`", "'", "\u00b4", "'", // apostrophes
	"\u2010", "-", "\u2012", "-", "\u2013", "-", "\u2014", "-", "\u2015", "-", // dashes
	"\u2212", "-", // minus sign
	"\u2044", "/", // fraction slash
)

// normalizeText applies the compatibility normalization used for every
// comparison: "…" becomes "...", "½" becomes "1/2", the minus sign and dashes
// become "-", and curly apostrophes become straight ones. Superscripts and
// vulgar fractions are spaced first so normalization cannot fuse them with a
// neighbouring number ("10²" is not "102", "5½" is not "51/2").
func normalizeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.No, r) {
			b.WriteByte(' ')
			b.WriteRune(r)
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return textReplacer.Replace(norm.NFKC.String(b.String()))
}

const (
	// numberSigns start a number only when no letter, digit or point comes
	// directly before them, so "x-5" and "COVID-19" are not negative numbers.
	numberSigns = "+-±∓"
	// numberMarks before a number always belong to it ("<5", "~5").
	numberMarks = "<>≤≥≈~"
	// numberSuffixes directly after a number belong to it ("5%", "5°", "5+").
	numberSuffixes = "%‰°+\u2032\u2033" // the last two are prime and double prime
	// spacedSuffixes also belong to a number after a space ("5 %", "5 °C").
	spacedSuffixes = "%‰°"
)

// numberEnd returns the end of the number that starts at rs[i], or i when no
// number starts there. A number keeps every symbol that can change its value
// or meaning, with or without spaces in between:
//   - signs, comparison marks and currency symbols before it ("-5", "- 5",
//     "+5", "< 5", "$5") and a leading decimal point (".5");
//   - any symbol between two digits ("1/2", "1.2", "1,200", "7.2.3", "3:30",
//     "5-10"), and a sign or comparison mark between two numbers with spaces
//     around it ("5 - 10" is the range "5-10", not "5" and "-10");
//   - a percent, degree or currency sign after it ("5%", "5 %", "5 €"), and a
//     plus or prime directly after it ("5+").
//
// Sentence punctuation after a number ("5.", "5,", "5)") is not kept, and
// letters always split from digits.
func numberEnd(rs []rune, i int) int {
	afterWord := i > 0 && (unicode.IsLetter(rs[i-1]) || unicode.IsDigit(rs[i-1]) || rs[i-1] == '.')
	if afterWord && strings.ContainsRune(numberSigns, rs[i]) {
		return i // a hyphen inside a word is not a sign
	}
	j := i
	for j < len(rs) && (isCurrency(rs[j]) || strings.ContainsRune(numberSigns+numberMarks, rs[j])) {
		j = skipSpaces(rs, j+1)
	}
	if j+1 < len(rs) && rs[j] == '.' && unicode.IsDigit(rs[j+1]) && !(j == i && afterWord) {
		j++
	}
	if j >= len(rs) || !unicode.IsDigit(rs[j]) {
		return i
	}
body:
	for j < len(rs) {
		switch {
		case unicode.IsDigit(rs[j]):
			j++
		case j+1 < len(rs) && isNumberJoiner(rs[j]) && unicode.IsDigit(rs[j+1]):
			j += 2
		default:
			k := skipSpaces(rs, j)
			if k < len(rs) && strings.ContainsRune(numberSigns+numberMarks, rs[k]) {
				if m := skipSpaces(rs, k+1); m < len(rs) && unicode.IsDigit(rs[m]) {
					j = m // "5 - 10", "5 ± 0.5"
					continue
				}
			}
			break body
		}
	}
	for j < len(rs) {
		k := skipSpaces(rs, j)
		switch {
		case k == j && (strings.ContainsRune(numberSuffixes, rs[j]) || isCurrency(rs[j])):
			j++
		case k < len(rs) && strings.ContainsRune(spacedSuffixes, rs[k]):
			j = k + 1
		case k < len(rs) && isCurrency(rs[k]) && !digitAfter(rs, k+1):
			j = k + 1 // "5 €", but in "5 $10" the "$" belongs to "10"
		default:
			return j
		}
	}
	return j
}

// numberWord is the comparison word for a number: spaces are dropped, so "- 5"
// and "-5", or "5 %" and "5%", are the same word.
func numberWord(rs []rune) string {
	var b strings.Builder
	for _, r := range rs {
		if r != ' ' && r != '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// skipSpaces returns the first index at or after j that is not a space or tab.
func skipSpaces(rs []rune, j int) int {
	for j < len(rs) && (rs[j] == ' ' || rs[j] == '\t') {
		j++
	}
	return j
}

// digitAfter reports a digit at j, after any spaces.
func digitAfter(rs []rune, j int) bool {
	j = skipSpaces(rs, j)
	return j < len(rs) && unicode.IsDigit(rs[j])
}

func isCurrency(r rune) bool { return unicode.Is(unicode.Sc, r) }

// isNumberSymbol reports a symbol that can change the meaning of a number.
// Next to a number it is part of that number's word; with no number next to
// it, it is kept as a word of its own. The hyphen is left out: with no number
// next to it, it is a dash.
func isNumberSymbol(r rune) bool {
	return r != '-' && (isCurrency(r) || strings.ContainsRune(numberSigns+numberMarks+numberSuffixes, r))
}

// isNumberJoiner reports a symbol that, between two digits, is part of the
// number: anything that is not a letter, digit or space.
func isNumberJoiner(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r)
}

// quoteParts splits a quotation at ellipses. Each part must appear, in order,
// in the cited text. Empty parts (a leading or trailing "...") are dropped.
func quoteParts(quote string) [][]string {
	var parts [][]string
	for _, p := range strings.Split(normalizeText(quote), "...") {
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

// Words that change the number next to them. A quote that starts or ends at
// a number but leaves one of these out is flagged (see checkNumberQualifiers):
// "5 kPa" taken from "minus 5 kPa", "not more than 5 kPa" or "5 kPa or more".
// First draft for review. Like the cues above, these are lexical signals and
// miss anything phrased differently.
var (
	leadingQualifiers = phrases(
		"minus", "negative", "plus", "plus or minus",
		"less than", "more than", "fewer than", "greater than", "lower than", "higher than",
		"no less than", "no more than", "no fewer than", "no greater than", "no lower than", "no higher than",
		"under", "over", "below", "above", "up to", "at least", "at most", "within", "between",
		"maximum", "max", "minimum", "min", "exceed", "exceeds", "exceeding",
		"about", "around", "approximately", "approx", "roughly", "nearly", "almost", "close to",
		"not", "never", "isn't", "aren't", "wasn't", "weren't", "don't", "doesn't", "didn't",
		"won't", "can't", "cannot", "shouldn't",
	)
	trailingQualifiers = phrases(
		"or less", "or more", "or fewer", "or lower", "or higher", "or greater",
		"or below", "or above", "or under", "or over", "or so",
		"and up", "and above", "and below", "and under", "and over",
		"maximum", "max", "minimum", "min", "at most", "at least",
		"below", "above", "under", "over", "less", "more", "lower", "higher",
		"plus", "minus", "ish", "short",
	)
	numberWords = map[string]bool{
		"zero": true, "one": true, "two": true, "three": true, "four": true, "five": true,
		"six": true, "seven": true, "eight": true, "nine": true, "ten": true, "eleven": true,
		"twelve": true, "thirteen": true, "fourteen": true, "fifteen": true, "sixteen": true,
		"seventeen": true, "eighteen": true, "nineteen": true, "twenty": true, "thirty": true,
		"forty": true, "fifty": true, "sixty": true, "seventy": true, "eighty": true,
		"ninety": true, "hundred": true, "thousand": true, "million": true, "half": true,
		"quarter": true, "dozen": true,
	}
)

func phrases(list ...string) [][]string {
	out := make([][]string, len(list))
	for i, p := range list {
		out[i] = words(p)
	}
	return out
}

// isNumberWord reports a word that is a number: it has a digit ("5", "-5",
// "1/2") or is a number word ("five", "half").
func isNumberWord(w string) bool {
	if numberWords[w] {
		return true
	}
	return strings.IndexFunc(w, unicode.IsDigit) >= 0
}

// firstNumber and lastNumber return the index of the first or last number in
// ws, or -1.
func firstNumber(ws []string) int {
	for i, w := range ws {
		if isNumberWord(w) {
			return i
		}
	}
	return -1
}

func lastNumber(ws []string) int {
	for i := len(ws) - 1; i >= 0; i-- {
		if isNumberWord(ws[i]) {
			return i
		}
	}
	return -1
}

// phraseEndingAt returns the length of the longest phrase that ends just
// before ws[p] (ws[p-n:p]), or 0.
func phraseEndingAt(ws []string, p int, list [][]string) int {
	best := 0
	for _, ph := range list {
		if n := len(ph); n > best && n <= p && p <= len(ws) && indexSeq(ws[p-n:p], ph, 0) == 0 {
			best = n
		}
	}
	return best
}

// phraseStartingAt returns the length of the longest phrase that starts at
// ws[p] (ws[p:p+n]), or 0.
func phraseStartingAt(ws []string, p int, list [][]string) int {
	best := 0
	for _, ph := range list {
		if n := len(ph); n > best && p >= 0 && p+n <= len(ws) && indexSeq(ws[p:p+n], ph, 0) == 0 {
			best = n
		}
	}
	return best
}

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
