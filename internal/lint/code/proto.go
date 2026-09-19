package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/vale-cli/vale/v3/internal/core"
)

func Protobuf() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`//|/\*|\*/`),
		Parser:  protobuf.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: "(comment) @comment", Type: ""}},
		Padding: cStyle,
	}
}
