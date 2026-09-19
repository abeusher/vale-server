package lint

import (
	"reflect"
	"testing"

	"golang.org/x/net/html"
)

// TestQuoteSpans pins how plain text is paired: curly and straight marks,
// an apostrophe left inside a single-quoted line, an unclosed quotation
// that stops at its paragraph, and an inch sign that opens nothing.
func TestQuoteSpans(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`"Go," she said. He said "no".`, []string{`"Go,"`, `"no"`}},
		{`“Go,” she said.`, []string{`“Go,”`}},
		{`‘Don’t,’ he said, and Kenny’s dog left.`, []string{`‘Don’t,’`}},
		{"\"Unclosed, she said\n\nNext paragraph.", []string{`"Unclosed, she said`}},
		{`He is 5'10" tall.`, nil},
		{`(“Aside.”)`, []string{`“Aside.”`}},
	}
	for _, c := range cases {
		var got []string
		for _, s := range quoteSpans(c.in) {
			got = append(got, c.in[s[0]:s[1]])
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("quoteSpans(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestWrapQuotes pins the tree: a quotation becomes a `q`, marks inside,
// inline markup within it is kept, an unclosed one runs to the end of its
// element, and code is left alone.
func TestWrapQuotes(t *testing.T) {
	cases := []struct{ in, want string }{
		{`<p>"Go," she said.</p>`, `<p><q>"Go,"</q> she said.</p>`},
		{`<p>She said, "we <em>knew</em>." Then left.</p>`, `<p>She said, <q>"we <em>knew</em>."</q> Then left.</p>`},
		{`<p>"Unclosed, she said</p><p>Next.</p>`, `<p><q>"Unclosed, she said</q></p><p>Next.</p>`},
		{`<p>Run <code>"x"</code> now.</p>`, `<p>Run <code>"x"</code> now.</p>`},
		{`<p>He <q>knew</q> and "she knew".</p>`, `<p>He <q>knew</q> and <q>"she knew"</q>.</p>`},
		{`<ul><li>"One."</li><li>“Two.”</li></ul>`, `<ul><li><q>"One."</q></li><li><q>“Two.”</q></li></ul>`},
	}
	for _, c := range cases {
		got, err := markSelections([]byte(c.in), nil, true)
		if err != nil {
			t.Fatal(err)
		}
		// The renderer escapes a straight mark; the tree is what is pinned.
		if body := html.UnescapeString(between(string(got), "<body>", "</body>")); body != c.want {
			t.Errorf("wrapQuotes(%s):\n got %s\nwant %s", c.in, body, c.want)
		}
	}
}
