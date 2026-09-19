package code

import (
	"regexp"

	"github.com/smacker/go-tree-sitter/yaml"
	"github.com/vale-cli/vale/v3/internal/core"
)

func YAML() *Language {
	return &Language{
		Delims:  regexp.MustCompile(`#`),
		Parser:  yaml.GetLanguage(),
		Queries: []core.Scope{{Name: "", Expr: "(comment) @comment", Type: ""}},
		Padding: func(s string) int {
			return computePadding(s, []string{"#"})
		},
	}
}
