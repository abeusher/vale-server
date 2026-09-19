package check

import (
	"unicode"
	"unicode/utf8"
)

// The default spelling filters, hand-written.
//
// A spelling rule runs these against every word of every block, and blocks
// overlap -- a sentence is also part of a paragraph -- so the same word is
// tested several times over. Profiling a spell-check of a 120 KB file put 72%
// of the run inside regexp.(*machine).match, all of it here.
//
// The three patterns are simple enough to read directly, and the regexes are
// ASCII-only, so a byte scan answers each of them exactly. Order matters:
// skipsNonWord is both the cheapest and the most often true, so it goes first.

// skipsNonWord reports whether a word contains anything outside the pattern
// `[^a-zA-Z_']` -- that is, whether that pattern would match.
//
// Bytes above ASCII count, as they do for the regex: a rune outside the class
// is a rune the class does not contain.
func skipsNonWord(word string) bool {
	for i := 0; i < len(word); i++ {
		c := word[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '\'':
		default:
			return true
		}
	}
	return false
}

// skipsTrailingCaps reports whether `[A-Z]+$` would match: the word ends in at
// least one capital.
func skipsTrailingCaps(word string) bool {
	if word == "" {
		return false
	}
	c := word[len(word)-1]
	return c >= 'A' && c <= 'Z'
}

// skipsCamel reports whether `[A-Z]{1}[a-z]+[A-Z]+\w+` would match: a capital,
// one or more lower-case letters, one or more capitals, then a word character.
//
// `[A-Z]+` backtracks, which is easy to miss: it can hand a capital back to
// `\w+`, so two capitals satisfy the tail on their own and nothing need follow
// them. A greedy scan that consumed every capital and then demanded another
// word character got "'_0ZaAZ _" wrong.
func skipsCamel(word string) bool {
	for i := 0; i < len(word); i++ {
		if !isUpper(word[i]) {
			continue
		}

		j := i + 1
		for j < len(word) && isLower(word[j]) {
			j++
		}
		if j == i+1 { // no lower-case run
			continue
		}

		k := j
		for k < len(word) && isUpper(word[k]) {
			k++
		}
		switch {
		case k-j >= 2:
			// Two capitals: one for `[A-Z]+`, one for `\w+`.
			return true
		case k-j == 1 && k < len(word) && isWordByte(word[k]):
			return true
		}
	}
	return false
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }

// isWordByte answers `\w`, which is [0-9A-Za-z_] for an ASCII pattern.
func isWordByte(c byte) bool {
	return isUpper(c) || isLower(c) || (c >= '0' && c <= '9') || c == '_'
}

// skippedByDefault reports whether any default filter would skip the word.
func skippedByDefault(word string) bool {
	return skipsNonWord(word) || skipsTrailingCaps(word) || skipsCamel(word)
}

// skipsNonIdentifier reports whether a word holds a character no identifier
// does: anything outside letters, digits, `_`, `-`, and `'`.
func skipsNonIdentifier(word string) bool {
	for _, r := range word {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-', r == '\'':
		default:
			return true
		}
	}
	return false
}

// identifierPart is one word within an identifier and where it starts.
type identifierPart struct {
	text string
	at   int
}

// splitIdentifier breaks an identifier at an underscore, a hyphen, a digit,
// and a change of case: `getHTTPResponse_v2` is get, HTTP, Response, v.
// The last capital of a run starts the next part, so HTTPServer is HTTP and
// Server.
func splitIdentifier(word string) []identifierPart {
	var parts []identifierPart
	runes := []rune(word)
	start, at := -1, 0 // start: byte offset of the current part, or -1
	flush := func(end int) {
		if start >= 0 && end > start {
			parts = append(parts, identifierPart{text: word[start:end], at: start})
		}
		start = -1
	}
	for i, r := range runes {
		size := utf8.RuneLen(r)
		switch {
		case !unicode.IsLetter(r):
			flush(at)
		case start < 0:
			start = at
		case unicode.IsUpper(r) && unicode.IsLower(runes[i-1]):
			flush(at)
			start = at
		case unicode.IsUpper(r) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) &&
			unicode.IsUpper(runes[i-1]):
			flush(at)
			start = at
		}
		at += size
	}
	flush(at)
	return parts
}
