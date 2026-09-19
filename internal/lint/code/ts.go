package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/vale-cli/vale/v3/internal/core"
)

func TypeScript() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`//|/\*|\*/`),
		Prefix:  cStylePrefix,
		Parser:  typescript.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: "(comment) @comment", Type: ""}},
		Padding: cStyle,
	}
}
