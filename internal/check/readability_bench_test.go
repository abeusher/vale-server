package check

import (
	"strings"
	"testing"

	"github.com/vale-cli/vale/v3/internal/core"
	"github.com/vale-cli/vale/v3/internal/nlp"
)

// One rule per formula is the common shape; the block is summarized once.
func BenchmarkReadabilityRules(b *testing.B) {
	text := strings.Repeat("The quick brown fox jumps over the lazy dog, and "+
		"then it wanders across the meadow to find its supper. ", 40)

	var rules []Readability
	for _, m := range readabilityMetrics {
		rules = append(rules, Readability{
			Definition: Definition{Name: "Readability." + m, Level: "warning"},
			Metrics:    []string{m},
			Grade:      100,
		})
	}

	b.ReportAllocs()
	for b.Loop() {
		blk := nlp.NewBlock("", text, "text")
		for _, r := range rules {
			if _, err := r.Run(blk, nil, &core.Config{}); err != nil {
				b.Fatal(err)
			}
		}
	}
}
