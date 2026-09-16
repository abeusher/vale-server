package core

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// FrontMatterValues returns each top-level string field of YAML or JSON
// front matter, in document order, as a value placed by its scalar and
// scoped `frontmatter.<key>`. fm is the front matter as the file holds it,
// from the file's first line, so its positions are the file's.
func FrontMatterValues(fm string, srcLines []string) ([]ScopedValues, error) {
	// The delimiter lines are blanked, not cut, so lines keep their numbers.
	lines := strings.Split(fm, "\n")
	for i, line := range lines {
		if t := strings.TrimSpace(line); t == "---" || t == ";;;" {
			lines[i] = ""
		}
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}

	var found []ScopedValues
	fields := root.Content[0].Content
	for i := 0; i+1 < len(fields); i += 2 {
		key, val := fields[i], fields[i+1]
		if val.Kind != yaml.ScalarNode || val.ShortTag() != "!!str" {
			continue
		}
		found = append(found, ScopedValues{
			Scope:  "frontmatter." + key.Value,
			Values: []ScopedValue{sourceValue(val, srcLines, "")},
		})
	}
	return found, nil
}
