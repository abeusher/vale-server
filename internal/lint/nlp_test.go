package lint

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vale-cli/vale/v3/internal/core"
)

// A section that turns off every sentence-scoped rule is not segmented, and
// its other alerts are unchanged.
func TestSegmentationFollowsTheFile(t *testing.T) {
	styles, err := filepath.Abs("../../testdata/styles")
	if err != nil {
		t.Fatal(err)
	}

	sentenceRules := []string{
		"demo.CommasPerSentence", "demo.SentenceLength", "demo.Meet-up", "demo.Meetup",
	}

	var off strings.Builder
	for _, name := range sentenceRules {
		off.WriteString(name + " = NO\n")
	}

	ini := "StylesPath = " + styles + "\n" +
		"MinAlertLevel = suggestion\n\n" +
		"[**/a/*.txt]\nBasedOnStyles = demo\n\n" +
		"[**/b/*.txt]\nBasedOnStyles = demo\n" + off.String()

	cfg, err := core.NewConfig(&core.CLIFlags{IgnoreGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = core.FromString(ini, cfg, false); err != nil {
		t.Fatal(err)
	}

	linter, err := NewLinter(cfg)
	if err != nil {
		t.Fatal(err)
	}

	text := "We will utilize the meet up, and the meetup, and the Meet-up.\n\n" +
		"This is a very long sentence that keeps going, and going, and going, " +
		"and going, and going, and going, and going, and going, and going.\n"

	root := t.TempDir()
	paths := map[string]string{}
	for _, dir := range []string{"a", "b"} {
		if err = os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		paths[dir] = filepath.Join(root, dir, "test.txt")
		if err = os.WriteFile(paths[dir], []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	with := linter.lintFile(paths["a"])
	if with.err != nil {
		t.Fatal(with.err)
	}
	if !with.file.NLP.Segmentation {
		t.Fatal("a section running sentence-scoped rules was not segmented")
	}

	without := linter.lintFile(paths["b"])
	if without.err != nil {
		t.Fatal(without.err)
	}
	if without.file.NLP.Segmentation {
		t.Error("a section running no sentence-scoped rule was segmented")
	}

	var want, got []string
	for _, a := range with.file.Alerts {
		if !core.StringInSlice(a.Check, sentenceRules) {
			want = append(want, a.Check+":"+a.Match)
		}
	}
	for _, a := range without.file.Alerts {
		got = append(got, a.Check+":"+a.Match)
	}
	if len(want) == 0 {
		t.Fatal("the sample produced no alerts from the other rules")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("alerts changed without segmentation:\n got %v\nwant %v", got, want)
	}
}
