package spell

import (
	"reflect"
	"testing"
)

// Hunspell accepts the upper-cased form of any entry; a capitalized entry
// still rejects its lower-cased form.
func TestUpperCaseOfCapitalizedEntry(t *testing.T) {
	gs := speller(t, "SET UTF-8\n", "2\nParis\nhello\n")
	checkWords(t, gs, map[string]bool{
		"Paris": true, "PARIS": true, "paris": false,
		"hello": true, "Hello": true, "HELLO": true,
	})
}

// A suggestion is returned in the case the word was written in, and an
// apostrophe does not start a new word.
func TestSuggestionsKeepTheWordsCase(t *testing.T) {
	gs := speller(t, "SET UTF-8\n", "2\ndon't\nworld\n")

	cases := map[string]string{
		"Don'y": "Don't",
		"DON'Y": "DON'T",
		"don'y": "don't",
		"Wrold": "World",
		"WROLD": "WORLD",
	}
	for word, want := range cases {
		hits := gs.suggest(word)
		if len(hits) == 0 || hits[0].word != want {
			t.Errorf("suggest(%q) = %v, want %q first", word, hits, want)
		}
	}

	// Fewer entries than suggestions is not an error.
	got := []string{}
	for _, h := range gs.suggest("wrold") {
		got = append(got, h.word)
	}
	if want := []string{"world", "don't"}; !reflect.DeepEqual(got, want) {
		t.Errorf("suggest over a two-word dictionary = %v, want %v", got, want)
	}
}
