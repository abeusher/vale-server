package spell

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// suggestionFloor is the fewest of Hunspell's first suggestions each
// fixture's checker must place in its top five. It is raised as the
// suggester improves and never lowered, so a regression fails the run.
var suggestionFloor = map[string]int{
	"1463589":           5,
	"1463589_utf":       5,
	"1695964":           3,
	"IJ":                1,
	"allcaps":           3,
	"allcaps2":          2,
	"allcaps_utf":       3,
	"base":              11,
	"base_utf":          13,
	"checksharpsutf":    1,
	"forceucase":        2,
	"i35725":            10,
	"i54633":            2,
	"i58202":            13,
	"keepcase":          8,
	"map":               3,
	"maputf":            3,
	"oconv":             0,
	"opentaal_keepcase": 6,
	"ph":                9,
	"phone":             1,
	"reputf":            1,
	"sug2":              3,
	"sugutf":            12,
	"utf8_nonbmp":       2,
}

// TestHunspellSuggestions scores the suggester against Hunspell's `.sug`
// files: for each wrong word, whether Hunspell's first suggestion is among
// the top five here, and whether it is the first. Only fixtures whose
// `.sug` has a line per wrong word are scored; Hunspell writes no line for a
// word it has no suggestion for, so the others cannot be aligned.
func TestHunspellSuggestions(t *testing.T) {
	sugs, err := filepath.Glob(filepath.Join("..", "..", "testdata", "hunspell", "*.sug"))
	if err != nil {
		t.Fatal(err)
	}

	var totalWords, top1, top5, scored int
	for _, sug := range sugs {
		base := strings.TrimSuffix(sug, ".sug")
		stem := filepath.Base(base)
		wrong := hunspellLines(t, base+".wrong")
		want := hunspellLines(t, sug)
		if len(wrong) == 0 || len(wrong) != len(want) {
			continue
		}
		scored++

		gs, loadErr := newGoSpellReader(
			bytes.NewReader(readFixture(t, base+".aff")),
			bytes.NewReader(readFixture(t, base+".dic")))
		if loadErr != nil {
			t.Errorf("%s: load: %v", stem, loadErr)
			continue
		}

		hits := 0
		var misses []string
		for i, word := range wrong {
			word = strings.TrimSpace(word)
			expected := strings.Split(want[i], ", ")
			first := strings.TrimSpace(expected[0])

			var got []string
			for _, h := range gs.suggest(word) {
				got = append(got, h.word)
			}
			totalWords++
			if len(got) > 0 && got[0] == first {
				top1++
			}
			if stringIn(first, got) {
				top5++
				hits++
			} else {
				misses = append(misses, fmt.Sprintf("%s: want %q, got %v", word, first, got))
			}
		}

		floor, known := suggestionFloor[stem]
		switch {
		case known && hits < floor:
			t.Errorf("%s: %d of %d in the top five, below the floor of %d:\n  %s",
				stem, hits, len(wrong), floor, strings.Join(misses, "\n  "))
		case known && hits > floor:
			t.Errorf("%s: %d of %d in the top five; raise its floor from %d", stem, hits, len(wrong), floor)
		case !known:
			t.Logf("%s: %d of %d in the top five (unlisted)\n  %s", stem, hits, len(wrong), strings.Join(misses, "\n  "))
		}
	}
	t.Logf("suggestions: %d fixtures scored, %d words: %d top-1, %d top-5", scored, totalWords, top1, top5)
}
