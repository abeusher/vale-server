package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/c"
	"github.com/vale-cli/vale/v3/internal/core"
)

func C() *Language {
	return &Language{
		Delims: regexp.MustCompile(`//|/\*|\*/`),
		Prefix: cStylePrefix,
		Parser: c.GetLanguage(),
		Queries: []core.Scope{
			{Name: "", Expr: "(comment) @comment", Type: ""},
		},
		Padding: cStyle,
	}
}
