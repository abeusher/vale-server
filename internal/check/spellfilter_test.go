package check

import (
	"math/rand"
	"strings"
	"testing"
)

// TestSpellFiltersMatchRegex checks the hand-written filters against the
// patterns they replace, over both fixed cases and random strings.
func TestSpellFiltersMatchRegex(t *testing.T) {
	cases := []string{
		"", "a", "A", "hello", "Hello", "HELLO", "helloW", "CamelCase",
		"camelCase", "XMLHttpRequest", "iOS", "IDs", "don't", "it's", "_foo",
		"foo_bar", "foo-bar", "foo.bar", "café", "naïve", "Straße", "42",
		"a1b2", "ABCd", "aBC", "aBCd", "HTTPServer", "getHTTPResponse",
		"McDonald", "O'Brien", "e.g", "U.S.A", "ZZ", "aZ", "AaB", "AaBc",
	}
	// Coverage, not secrecy: these strings are fed to two implementations of
	// the same filter to check they agree, so a predictable generator is all
	// that is wanted -- and a fixed seed makes a failure reproducible.
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // not security-sensitive
	for i := 0; i < 4000; i++ {
		n := 1 + rng.Intn(12)
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteByte(" aAzZ_'0-.é"[rng.Intn(11)])
		}
		cases = append(cases, b.String())
	}

	for _, w := range cases {
		for i, re := range defaultFilters {
			var got bool
			switch i {
			case 0:
				got = skipsCamel(w)
			case 1:
				got = skipsTrailingCaps(w)
			case 2:
				got = skipsNonWord(w)
			}
			if want := re.MatchString(w); got != want {
				t.Fatalf("filter %d (%s) on %q: got %v, want %v",
					i, re.String(), w, got, want)
			}
		}
	}
}

// An identifier splits at an underscore, a hyphen, a digit, and a change of
// case, and each part knows where it starts.
func TestSplitIdentifier(t *testing.T) {
	cases := map[string][]identifierPart{
		"getHTTPResponse_v2": {{"get", 0}, {"HTTP", 3}, {"Response", 7}, {"v", 16}},
		"recieveMessage":     {{"recieve", 0}, {"Message", 7}},
		"RecieveMessage":     {{"Recieve", 0}, {"Message", 7}},
		"recieve_message":    {{"recieve", 0}, {"message", 8}},
		"teh-thing":          {{"teh", 0}, {"thing", 4}},
		"foo2bar":            {{"foo", 0}, {"bar", 4}},
		"HTML":               {{"HTML", 0}},
		"iPhone":             {{"i", 0}, {"Phone", 1}},
		"plain":              {{"plain", 0}},
		"ÜberGröße":          {{"Über", 0}, {"Größe", 5}},
	}
	for word, want := range cases {
		got := splitIdentifier(word)
		if len(got) != len(want) {
			t.Errorf("splitIdentifier(%q) = %v, want %v", word, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("splitIdentifier(%q)[%d] = %v, want %v", word, i, got[i], want[i])
			}
		}
	}
}
