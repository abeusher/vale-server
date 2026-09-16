package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vale-cli/vale/v3/internal/glob"
)

// TestNewFileSectionOrderWins verifies that when multiple config sections
// match the same file, NewFile resolves Lang and Transform deterministically
// by taking the LAST matching section in the order it was written -- the
// same "later one wins" semantics the rule/level resolution loop documents
// for #965 -- rather than depending on Go's randomized map iteration order.
func TestNewFileSectionOrderWins(t *testing.T) {
	sections := []struct {
		name string
		lang string
		xslt string
	}{
		{"*.md", "en", "one.xsl"},
		{"*.md", "fr", "two.xsl"},
		{"*.md", "de", "three.xsl"},
		{"*.md", "ja", "four.xsl"},
		{"*.md", "es", "five.xsl"},
		{"*.md", "it", "six.xsl"},
	}

	cfg, err := NewConfig(&CLIFlags{})
	if err != nil {
		t.Fatal(err)
	}

	for i, s := range sections {
		// Reuse the same glob pattern across sections, but track each
		// occurrence under its own key so every one can carry its own
		// Lang/Transform value -- mirroring how repeated real config
		// sections (e.g. multiple `[*.md]` blocks) are keyed internally.
		key := s.name
		if i > 0 {
			key = s.name + string(rune('a'+i))
		}

		pat, cerr := glob.Compile(s.name)
		if cerr != nil {
			t.Fatal(cerr)
		}

		cfg.SecToPat[key] = pat
		cfg.RuleKeys = append(cfg.RuleKeys, key)
		cfg.FormatToLang[key] = s.lang
		cfg.Stylesheets[key] = s.xslt
		cfg.SChecks[key] = map[string]bool{}
		cfg.SLevels[key] = map[string]string{}
	}

	wantLang := sections[len(sections)-1].lang
	wantTransform := sections[len(sections)-1].xslt

	docPath := filepath.Join(t.TempDir(), "doc.md")
	if werr := os.WriteFile(docPath, []byte("# Title\n"), 0600); werr != nil {
		t.Fatal(werr)
	}

	const iterations = 50
	for i := 0; i < iterations; i++ {
		f, ferr := NewFile(docPath, cfg)
		if ferr != nil {
			t.Fatal(ferr)
		}

		if f.NLP.Lang != wantLang {
			t.Fatalf("iteration %d: expected Lang %q (the last written matching section), got %q",
				i, wantLang, f.NLP.Lang)
		}

		if f.Transform != wantTransform {
			t.Fatalf("iteration %d: expected Transform %q (the last written matching section), got %q",
				i, wantTransform, f.Transform)
		}
	}
}

// TestNewFileGlobalLangFallback verifies that the `[*] Lang = ...` global
// default still applies when no format-specific section matches the file.
func TestNewFileGlobalLangFallback(t *testing.T) {
	cfg, err := NewConfig(&CLIFlags{})
	if err != nil {
		t.Fatal(err)
	}

	cfg.FormatToLang["*"] = "ja"

	docPath := filepath.Join(t.TempDir(), "doc.txt")
	if werr := os.WriteFile(docPath, []byte("hello\n"), 0600); werr != nil {
		t.Fatal(werr)
	}

	f, err := NewFile(docPath, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if f.NLP.Lang != "ja" {
		t.Fatalf("expected global Lang fallback %q, got %q", "ja", f.NLP.Lang)
	}
}

// TestDirectiveRegions verifies the position-aware half of comment handling:
// a directive whose offset is known records a region, and a located alert
// inside it is suppressed -- by its own name, its style, or a match key --
// while one before the NO, after the YES, or from another check is not.
func TestDirectiveRegions(t *testing.T) {
	f := &File{Comments: map[string]bool{}}
	f.SetText("one\ntwo <!-- vale T.Rule = NO -->\nthree\n<!-- vale T.Rule = YES --> four\n")

	noAt := len("one\ntwo <!-- ")
	yesAt := len("one\ntwo <!-- vale T.Rule = NO -->\nthree\n<!-- ")

	f.UpdateCommentsAt("vale T.Rule = NO", noAt)
	f.UpdateCommentsAt("vale T.Rule = YES", yesAt)

	if f.RegionDisabled("T.Rule", "", 1, 1) {
		t.Error("suppressed before the NO")
	}
	if !f.RegionDisabled("T.Rule", "", 3, 1) {
		t.Error("not suppressed inside the region")
	}
	if f.RegionDisabled("T.Rule", "", 4, 30) {
		t.Error("suppressed after the YES")
	}
	if f.RegionDisabled("T.Other", "", 3, 1) {
		t.Error("suppressed another rule")
	}

	// A style-level directive covers every rule in the style, and an
	// unclosed region runs to the end of the file.
	f.UpdateCommentsAt("vale T = NO", yesAt+40)
	if !f.RegionDisabled("T.Other", "", 99, 1) {
		t.Error("style-level region did not cover the rule")
	}

	// An unknown position records nothing.
	g := &File{Comments: map[string]bool{}}
	g.SetText("text\n")
	g.UpdateCommentsAt("vale T.Rule = NO", -1)
	if g.RegionDisabled("T.Rule", "", 1, 1) {
		t.Error("recorded a region without a position")
	}
}

// An empty BasedOnStyles resets what a file inherited, its own keys still
// apply after it, and UNSET drops one inherited key.
func TestNewFileEmptyStylesResetAndUnset(t *testing.T) {
	root := t.TempDir()
	ini := "StylesPath = " + root + "\n\n" +
		"[*]\nGoogle.Semicolons = warning\nVale.Spelling = NO\n\n" +
		"[*.md]\nBasedOnStyles = Google\nGoogle.Colons = NO\n\n" +
		"[**/docs/*.md]\nBasedOnStyles =\nGoogle.Headings = YES\n\n" +
		"[**/api/*.md]\nGoogle.Colons = UNSET\n"

	cfg, err := NewConfig(&CLIFlags{IgnoreGlobal: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = FromString(ini, cfg, false); err != nil {
		t.Fatal(err)
	}

	open := func(dir string) *File {
		t.Helper()
		if err = os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, dir, "page.md")
		if err = os.WriteFile(path, []byte("Hello.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, ferr := NewFile(path, cfg)
		if ferr != nil {
			t.Fatal(ferr)
		}
		return f
	}

	other := open("other")
	if len(other.BaseStyles) != 1 || other.BaseStyles[0] != "Google" {
		t.Errorf("other: BaseStyles = %v, want [Google]", other.BaseStyles)
	}
	if v, ok := other.Checks["Google.Colons"]; !ok || v {
		t.Errorf("other: Google.Colons = %v, %v; want false, true", v, ok)
	}

	docs := open("docs")
	if len(docs.BaseStyles) != 0 {
		t.Errorf("docs: BaseStyles = %v, want none", docs.BaseStyles)
	}
	for _, k := range []string{"Google.Semicolons", "Vale.Spelling"} {
		if v, ok := docs.Checks[k]; !ok || v {
			t.Errorf("docs: %s = %v, %v; want false, true (global setting turned off)", k, v, ok)
		}
	}
	if _, ok := docs.Checks["Google.Colons"]; ok {
		t.Error("docs: Google.Colons was inherited past an empty BasedOnStyles")
	}
	if v, ok := docs.Checks["Google.Headings"]; !ok || !v {
		t.Errorf("docs: Google.Headings = %v, %v; want true, true (set after the reset)", v, ok)
	}

	api := open("api")
	if _, ok := api.Checks["Google.Colons"]; ok {
		t.Error("api: Google.Colons survived UNSET")
	}
	if !api.Unset["Google.Colons"] {
		t.Error("api: Google.Colons is not marked unset")
	}
	if len(api.BaseStyles) != 1 || api.BaseStyles[0] != "Google" {
		t.Errorf("api: BaseStyles = %v, want [Google]", api.BaseStyles)
	}
}
