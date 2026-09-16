package lint

import (
	"errors"
	"fmt"
	"strings"

	"github.com/adrg/frontmatter"

	"github.com/vale-cli/vale/v3/internal/check"
	"github.com/vale-cli/vale/v3/internal/core"
	"github.com/vale-cli/vale/v3/internal/nlp"
)

func (l *Linter) lintMetadata(f *core.File) error {
	metadata := make(map[string]any)

	body, err := frontmatter.Parse(strings.NewReader(f.Content), &metadata)
	if errors.Is(err, frontmatter.ErrNotFound) {
		return nil
	} else if err != nil {
		return core.NewE201FromPosition(err.Error(), f.Path, 1)
	}

	fm, fmErr := extractFrontMatter(f.Content, string(body))
	if fmErr != nil {
		return core.NewE201FromPosition(fmErr.Error(), f.Path, 1)
	}

	ignored := check.NewScope(l.Manager.Config.IgnoredScopes)
	if !strings.HasPrefix(strings.TrimSpace(fm), "+++") {
		return l.lintFrontMatterValues(f, fm, ignored)
	}

	// TOML front matter is placed by searching for each value.
	for key, value := range metadata {
		if s, ok := value.(string); ok {
			i, _ := findBestLineBySubstring(fm, s)
			if i < 0 {
				continue
			}
			scope := "text.frontmatter." + key + f.RealExt

			block := nlp.NewLinedBlock(f.Content, s, scope, i-1)
			if ignored.Matches(block) {
				continue
			}

			lErr := l.lintBlock(f, block, len(f.Lines), 0, false)
			if lErr != nil {
				return lErr
			}
		}
	}

	return nil
}

// lintFrontMatterValues lints YAML or JSON front matter field by field,
// each placed by its scalar's position rather than by a text search, which
// found a value inside an earlier field that shared its prefix.
func (l *Linter) lintFrontMatterValues(f *core.File, fm string, ignored check.Scope) error {
	values, err := core.FrontMatterValues(fm, strings.Split(f.Content, "\n"))
	if err != nil {
		return core.NewE201FromPosition(err.Error(), f.Path, 1)
	}

	kept := values[:0]
	for _, v := range values {
		if !ignored.Matches(nlp.Block{Scope: "text." + v.Scope + f.RealExt}) {
			kept = append(kept, v)
		}
	}

	// The values are linted in the file's place; the body follows, so what
	// they change is put back.
	normed, meta := f.NormedExt, f.MetaScope
	err = l.lintScopedValues(f, kept)
	f.NormedExt, f.MetaScope = normed, meta
	return err
}

func extractFrontMatter(file, body string) (string, error) {
	startIndex := strings.Index(file, body)
	if startIndex == -1 {
		return "", fmt.Errorf("body not found in the file")
	}
	return file[:startIndex], nil
}
