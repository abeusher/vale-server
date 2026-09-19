package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/vale-cli/vale/v3/internal/core"
)

func Ruby() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`#|=begin|=end`),
		Parser:  ruby.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: "(comment) @comment", Type: ""}},
		Padding: func(s string) int {
			return computePadding(s, []string{"#", `=begin`, `=end`})
		},
	}
}
