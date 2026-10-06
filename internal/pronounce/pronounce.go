// Package pronounce rewrites text with the user's custom pronunciations
// before it's sent to a speech engine.
package pronounce

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Separators between a word and how to say it, e.g. "dachary -> dakahri".
var separators = []string{"->", "→"}

// Rule replaces Word (matched case-insensitively) with Say.
type Rule struct {
	Word string
	Say  string
	re   *regexp.Regexp
}

// Parse reads one rule per line in the form "word -> replacement" (or with
// "→"). Whitespace around both sides is ignored, as are lines without a
// separator or without a word. When a word appears more than once, the last
// line wins.
func Parse(text string) []Rule {
	var rules []Rule
	for line := range strings.Lines(text) {
		word, say, ok := cut(line)
		if !ok {
			continue
		}
		fields := strings.Fields(word)
		if len(fields) == 0 {
			continue
		}
		for i, f := range fields {
			fields[i] = regexp.QuoteMeta(f)
		}
		// Spaces inside a phrase match any run of whitespace, so "New York"
		// still matches across a line break.
		re := regexp.MustCompile(`(?i)` + strings.Join(fields, `\s+`))
		word = strings.Join(strings.Fields(word), " ")
		rules = slices.DeleteFunc(rules, func(r Rule) bool { return strings.EqualFold(r.Word, word) })
		rules = append(rules, Rule{Word: word, Say: strings.TrimSpace(say), re: re})
	}
	return rules
}

// cut splits a line at its first separator.
func cut(line string) (word, say string, ok bool) {
	at, sepLen := -1, 0
	for _, sep := range separators {
		if i := strings.Index(line, sep); i >= 0 && (at < 0 || i < at) {
			at, sepLen = i, len(sep)
		}
	}
	if at < 0 {
		return "", "", false
	}
	return line[:at], line[at+sepLen:], true
}

// Apply replaces each rule's word in text with its replacement. Words only
// match whole: "cat -> kat" changes "Cat" but not "category". Longer words
// take precedence over shorter ones they overlap, and replacements are never
// themselves rewritten by other rules.
func Apply(text string, rules []Rule) string {
	if len(rules) == 0 {
		return text
	}
	ordered := slices.Clone(rules)
	slices.SortStableFunc(ordered, func(a, b Rule) int { return len(b.Word) - len(a.Word) })

	type span struct {
		start, end int
		say        string
	}
	var spans []span
	overlaps := func(start, end int) bool {
		return slices.ContainsFunc(spans, func(s span) bool { return start < s.end && s.start < end })
	}
	for _, r := range ordered {
		for _, m := range r.re.FindAllStringIndex(text, -1) {
			if wholeWord(text, m[0], m[1]) && !overlaps(m[0], m[1]) {
				spans = append(spans, span{m[0], m[1], r.Say})
			}
		}
	}
	if len(spans) == 0 {
		return text
	}
	slices.SortFunc(spans, func(a, b span) int { return a.start - b.start })

	var out strings.Builder
	last := 0
	for _, s := range spans {
		out.WriteString(text[last:s.start])
		out.WriteString(s.say)
		last = s.end
	}
	out.WriteString(text[last:])
	return out.String()
}

// wholeWord reports whether text[start:end] isn't part of a longer word. A
// match that begins or ends with punctuation (e.g. "C++") only needs a
// boundary on its word-character sides.
func wholeWord(text string, start, end int) bool {
	first, _ := utf8.DecodeRuneInString(text[start:end])
	last, _ := utf8.DecodeLastRuneInString(text[start:end])
	if isWordRune(first) {
		if before, _ := utf8.DecodeLastRuneInString(text[:start]); start > 0 && isWordRune(before) {
			return false
		}
	}
	if isWordRune(last) {
		if after, _ := utf8.DecodeRuneInString(text[end:]); end < len(text) && isWordRune(after) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
