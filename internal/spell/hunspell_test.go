package spell

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// hunspellUnsupported names the fixtures the checker does not pass yet, with
// the directive or behavior each one needs. A fixture that starts passing
// fails the run until it is removed from here, so the list stays honest.
var hunspellUnsupported = map[string]string{
	"hu":                         "a hyphenated compound whose left part is forbidden on its own",
	"limit-multiple-compounding": "a three-part compound one edit from a dictionary word",
	"opentaal_cpdpat2":           "CHECKCOMPOUNDPATTERN",
	"base_utf":                   "Turkish dotless i in case folding",
	"checksharps":                "CHECKSHARPS",
	"checksharpsutf":             "CHECKSHARPS",
	"checksharpsutf2":            "CHECKSHARPS",
	"dotless_i":                  "Turkish dotless i in case folding",
	"ph2":                        "FORBIDDENWORD against case variants",
	"iconv":                      "ICONV with multi-character and ordered rules",
	"iconv2":                     "ICONV with multi-character and ordered rules",
	"iconv3":                     "ICONV with multi-character and ordered rules",
	"oconv2":                     "OCONV",
	"ignore":                     "IGNORE",
	"ignoresug":                  "IGNORE",
	"ignoreutf":                  "IGNORE",
	"nepali":                     "IGNORE",
	"right_to_left_mark":         "IGNORE",
	"gh353":                      "non-ASCII digits",
	"fullstrip":                  "FULLSTRIP",
	"gh1044":                     "FULLSTRIP",
	"gh1122":                     "escaped slash in a .dic entry",
	"slash":                      "escaped slash in a .dic entry",
}

// TestHunspellCorpus runs Hunspell's own test fixtures: every word in a
// .good file is accepted, and every line in a .wrong file is rejected.
func TestHunspellCorpus(t *testing.T) {
	affs, err := filepath.Glob(filepath.Join("..", "..", "testdata", "hunspell", "*.aff"))
	if err != nil {
		t.Fatal(err)
	}
	if len(affs) == 0 {
		t.Fatal("no fixtures found")
	}

	passed, listed := 0, 0
	for _, aff := range affs {
		stem := strings.TrimSuffix(filepath.Base(aff), ".aff")
		reason, known := hunspellUnsupported[stem]

		t.Run(stem, func(t *testing.T) {
			failures := runHunspellFixture(t, strings.TrimSuffix(aff, ".aff"))
			switch {
			case known && len(failures) > 0:
				listed++
				t.Skipf("unsupported (%s): %s", reason, strings.Join(failures, "; "))
			case known:
				t.Errorf("passes now; remove it from hunspellUnsupported (%s)", reason)
			case len(failures) > 0:
				t.Errorf("%s", strings.Join(failures, "\n"))
			default:
				passed++
			}
		})
	}
	t.Logf("hunspell fixtures: %d passing, %d listed as unsupported, %d total",
		passed, listed, len(affs))
}

// runHunspellFixture loads one fixture and returns each expectation it fails.
func runHunspellFixture(t *testing.T, base string) []string {
	t.Helper()

	gs, err := newGoSpellReader(
		bytes.NewReader(readFixture(t, base+".aff")),
		bytes.NewReader(readFixture(t, base+".dic")))
	if err != nil {
		return []string{"load: " + err.Error()}
	}

	// Hunspell's runner feeds the word lists in as UTF-8 whatever the
	// dictionary's SET says.
	var failures []string
	for _, line := range hunspellLines(t, base+".good") {
		for _, word := range strings.Fields(line) {
			if !gs.spell(word) {
				failures = append(failures, "rejected good word "+word)
			}
		}
	}
	for _, line := range hunspellLines(t, base+".wrong") {
		wrong := false
		for _, word := range strings.Fields(line) {
			if !gs.spell(word) {
				wrong = true
			}
		}
		if !wrong {
			failures = append(failures, "accepted wrong word "+line)
		}
	}
	sort.Strings(failures)
	return failures
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// hunspellLines returns a fixture file's non-empty lines, or none when the
// file does not exist.
func hunspellLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}

	var lines []string
	for _, line := range strings.Split(string(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
