package core

import (
	"strings"
	"testing"
)

// A field is placed by its scalar, whatever quoting it uses, and a field
// that is not a string is left out.
func TestFrontMatterValues(t *testing.T) {
	fm := "---\ntitle: \"My document\"\ndate: 2024-01-01\ntags: [a, b]\nsummary: plain text\n---\n"
	values, err := FrontMatterValues(fm, strings.Split(fm, "\n"))
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		scope  string
		text   string
		line   int
		column int
	}{
		{"frontmatter.title", "My document", 2, 9},
		{"frontmatter.summary", "plain text", 5, 10},
	}
	if len(values) != len(want) {
		t.Fatalf("got %d values, want %d", len(values), len(want))
	}
	for i, w := range want {
		got := values[i]
		sv := got.Values[0]
		if got.Scope != w.scope || sv.Text != w.text || sv.Line != w.line || sv.Column != w.column {
			t.Errorf("value %d = %q %q %d:%d, want %q %q %d:%d",
				i, got.Scope, sv.Text, sv.Line, sv.Column, w.scope, w.text, w.line, w.column)
		}
	}
}

// JSON front matter sits between `;;;` lines and is placed the same way.
func TestFrontMatterValuesJSON(t *testing.T) {
	fm := ";;;\n{\n  \"title\": \"My document\"\n}\n;;;\n"
	values, err := FrontMatterValues(fm, strings.Split(fm, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("got %d values, want 1", len(values))
	}
	if sv := values[0].Values[0]; sv.Text != "My document" || sv.Line != 3 || sv.Column != 13 {
		t.Errorf("got %q at %d:%d, want %q at 3:13", sv.Text, sv.Line, sv.Column, "My document")
	}
}
