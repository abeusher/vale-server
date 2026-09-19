package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/css"
	"github.com/vale-cli/vale/v3/internal/core"
)

func CSS() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`/\*!?|\*/`),
		Parser:  css.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: "(comment) @comment", Type: ""}},
		Padding: func(s string) int {
			return computePadding(s, []string{"/*"})
		},
	}
}
