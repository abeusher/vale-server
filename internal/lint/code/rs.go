package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/rust"
	"github.com/vale-cli/vale/v3/internal/core"
)

func Rust() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`/{2,3}!?`),
		Parser:  rust.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: `(line_comment)+ @comment`, Type: ""}},
		Padding: func(s string) int {
			return computePadding(s, []string{"//", "//!", "///"})
		},
	}
}
