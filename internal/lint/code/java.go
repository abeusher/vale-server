package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/java"
	"github.com/vale-cli/vale/v3/internal/core"
)

func Java() *Language {
	return &Language{
		Delims: regexp.MustCompile(`//|/\*|\*/`),
		Prefix: cStylePrefix,
		Parser: java.GetLanguage(),
		Queries: []core.Scope{
			{Name: "", Expr: "(line_comment)+ @comment", Type: ""},
			{Name: "", Expr: "(block_comment)+ @comment", Type: ""},
		},
		Padding: cStyle,
	}
}
