package spell

import (
	"strings"
	"testing"
)

func speller(t *testing.T, aff, dic string) *goSpell {
	t.Helper()
	gs, err := newGoSpellReader(strings.NewReader(aff), strings.NewReader(dic))
	if err != nil {
		t.Fatalf("newGoSpellReader error: %v", err)
	}
	return gs
}

func checkWords(t *testing.T, gs *goSpell, want map[string]bool) {
	t.Helper()
	for word, ok := range want {
		if got := gs.spell(word); got != ok {
			t.Errorf("spell(%q) = %v, want %v", word, got, ok)
		}
	}
}

// A forbidden entry overrides the form another entry's affixes generate.
func TestForbiddenWord(t *testing.T) {
	gs := speller(t, `FORBIDDENWORD !
SFX S Y 1
SFX S 0 s .
`, `3
foo/S
foos/!
bar/!S
`)
	checkWords(t, gs, map[string]bool{
		"foo": true, "foos": false, "Foos": false, "bar": false, "bars": false,
	})
}

// A NEEDAFFIX stem is only a word once affixed; PSEUDOROOT is its old name.
func TestNeedAffix(t *testing.T) {
	for _, directive := range []string{"NEEDAFFIX", "PSEUDOROOT"} {
		gs := speller(t, directive+` X
SFX S Y 1
SFX S 0 s .
`, `2
virtual/XS
real/S
`)
		checkWords(t, gs, map[string]bool{
			"virtual": false, "virtuals": true, "real": true, "reals": true,
		})
	}
}

// A KEEPCASE word is accepted only as written; other words still accept
// their capitalized and upper-cased forms.
func TestKeepCase(t *testing.T) {
	gs := speller(t, `KEEPCASE K
SFX S Y 1
SFX S 0 s .
`, `2
macOS/K
iPhone/KS
`)
	checkWords(t, gs, map[string]bool{
		"macOS": true, "MacOS": false, "MACOS": false, "macos": false,
		"iPhone": true, "iPhones": true, "IPhones": false,
	})
}

// A CIRCUMFIX prefix and suffix are only valid together; a prefix without
// the flag still pairs with the suffixes that lack it.
func TestCircumfix(t *testing.T) {
	gs := speller(t, `CIRCUMFIX C
PFX P Y 2
PFX P 0 ge/C .
PFX P 0 un .
SFX T Y 1
SFX T 0 t/C .
SFX N Y 1
SFX N 0 en .
`, `1
mach/PTN
`)
	checkWords(t, gs, map[string]bool{
		"mach": true, "machen": true, "gemacht": true,
		"unmach": true, "unmachen": true,
		"gemach": false, "macht": false, "gemachen": false, "unmacht": false,
	})
}

// An AF alias names a flag set by its line number, for entries and for the
// continuation flags an affix rule carries.
func TestFlagAliases(t *testing.T) {
	gs := speller(t, `AF 2
AF S
AF ST
SFX S Y 1
SFX S 0 s .
SFX T Y 1
SFX T 0 ed/1 .
`, `2
walk/2
dog/1
`)
	checkWords(t, gs, map[string]bool{
		"walk": true, "walks": true, "walked": true, "walkeds": true,
		"dog": true, "dogs": true, "doged": false,
	})
}
