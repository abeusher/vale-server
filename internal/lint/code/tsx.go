package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/vale-cli/vale/v3/internal/core"
)

func Tsx() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`//|/\*|\*/`),
		Prefix:  cStylePrefix,
		Parser:  tsx.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: "(comment) @comment", Type: ""}},
		Padding: cStyle,
	}
}
