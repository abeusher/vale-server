package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/php"
	"github.com/vale-cli/vale/v3/internal/core"
)

func PHP() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`//|/\*\*?|\*/|#`),
		Prefix:  cStylePrefix,
		Parser:  php.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: "(comment) @comment", Type: ""}},
		Padding: func(s string) int {
			return computePadding(s, []string{"//", "/*", "#"})
		},
	}
}
