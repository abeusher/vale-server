package lint

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Quotations are prose no format marks up: to Markdown, a line of dialogue
// is characters in a paragraph. A `quote` scope needs them found, so they
// are paired here -- by their marks, the way a reader pairs them -- and
// wrapped in a `q` in the document view, or carried as runs of a plain-text
// block. The marks stay inside the quotation.
//
// An opening mark with no closing mark runs to the end of its paragraph,
// which is the convention for speech that continues into the next one.

// quoteClosers maps an opening quotation mark to the mark that closes it.
var quoteClosers = map[rune]rune{'“': '”', '"': '"', '‘': '’'}

// quoteSkip names the elements whose text is never a quotation.
var quoteSkip = map[string]bool{
	"head": true, "pre": true, "code": true, "tt": true, "kbd": true,
	"samp": true, "script": true, "style": true, "textarea": true, "q": true,
}

// openingQuote finds the first opening mark in s at or after from. It
// returns where the mark begins, its width, and the mark that closes it,
// or -1.
//
// A straight mark opens only after a space, a bracket, or a dash, since
// the same character closes and doubles as an inch sign.
func openingQuote(s string, from int) (int, int, rune) {
	for i := from; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if closer, ok := quoteClosers[r]; ok && opensQuote(s, i, r) {
			return i, size, closer
		}
		i += size
	}
	return -1, 0, 0
}

func opensQuote(s string, at int, mark rune) bool {
	if at == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(s[:at])
	if mark == '"' {
		return unicode.IsSpace(prev) || strings.ContainsRune("([{—–-/", prev)
	}
	return !unicode.IsLetter(prev) && !unicode.IsDigit(prev)
}

// closingQuote finds the mark that closes a quotation in s at or after
// from, returning the index just past it, or -1.
//
// A curly single mark is also an apostrophe: it closes only when no letter
// follows it, so "don’t" and "Kenny’s" stay inside the quotation.
func closingQuote(s string, closer rune, from int) int {
	for i := from; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == closer {
			next, _ := utf8.DecodeRuneInString(s[i+size:])
			if closer != '’' || !unicode.IsLetter(next) {
				return i + size
			}
		}
		i += size
	}
	return -1
}

// quoteSpans returns the byte ranges of the quotations in plain text, marks
// included. Pairing runs paragraph by paragraph.
func quoteSpans(s string) [][2]int {
	var spans [][2]int
	i := 0
	for i < len(s) {
		at, size, closer := openingQuote(s, i)
		if at < 0 {
			break
		}
		stop := len(s)
		if j := strings.Index(s[at:], "\n\n"); j >= 0 {
			stop = at + j
		}
		end := closingQuote(s[:stop], closer, at+size)
		if end < 0 {
			end = stop
		}
		spans = append(spans, [2]int{at, end})
		i = end
	}
	return spans
}

// wrapQuotes encloses each quotation under n in a `q` element.
//
// Pairing is done among an element's own text nodes, so a quotation may
// hold inline markup -- "we *knew*" -- and one that opens in a text node
// and never closes runs to the end of the element. An element that already
// is a quotation, or holds code, is left alone.
func wrapQuotes(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !quoteSkip[c.Data] {
			wrapQuotes(c)
		}
	}

	c := n.FirstChild
	for c != nil {
		if c.Type != html.TextNode {
			c = c.NextSibling
			continue
		}
		at, size, closer := openingQuote(c.Data, 0)
		if at < 0 {
			c = c.NextSibling
			continue
		}

		// The text before the mark stays where it is.
		rest := c
		if at > 0 {
			rest = &html.Node{Type: html.TextNode, Data: c.Data[at:]}
			c.Data = c.Data[:at]
			n.InsertBefore(rest, c.NextSibling)
		}
		q := &html.Node{Type: html.ElementNode, Data: "q", DataAtom: atom.Q}
		n.InsertBefore(q, rest)

		// Move nodes into the quotation until a text node closes it.
		var after *html.Node
		from := size
		for m := rest; m != nil; {
			next := m.NextSibling
			if m.Type == html.TextNode {
				if end := closingQuote(m.Data, closer, from); end >= 0 {
					if end < len(m.Data) {
						after = &html.Node{Type: html.TextNode, Data: m.Data[end:]}
						m.Data = m.Data[:end]
						n.InsertBefore(after, next)
					} else {
						after = next
					}
					n.RemoveChild(m)
					q.AppendChild(m)
					break
				}
			}
			n.RemoveChild(m)
			q.AppendChild(m)
			m, from = next, 0
		}
		c = after
	}
}
