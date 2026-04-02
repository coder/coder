package cli

import (
	"bytes"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// TemplateFrontmatter holds metadata extracted from YAML frontmatter
// in a template's README.md file. Templates in the coder/registry
// and examples/templates/ directories already use this format to
// declare display_name, description, and icon.
type TemplateFrontmatter struct {
	DisplayName string `yaml:"display_name"`
	Description string `yaml:"description"`
	Icon        string `yaml:"icon"`
}

// ParseTemplateFrontmatter extracts YAML frontmatter from the raw
// bytes of a README.md file. The frontmatter is delimited by a
// pair of "---" lines at the very start of the file. If the file
// does not begin with "---", an empty TemplateFrontmatter is
// returned without error.
func ParseTemplateFrontmatter(data []byte) (TemplateFrontmatter, error) {
	var fm TemplateFrontmatter

	data = bytes.TrimLeft(data, "\n\r")
	if !bytes.HasPrefix(data, []byte("---")) {
		return fm, nil
	}

	// Skip past the opening "---" line.
	data = data[3:]
	idx := bytes.Index(data, []byte("\n"))
	if idx == -1 {
		return fm, nil
	}
	data = data[idx+1:]

	// Find the closing "---" delimiter.
	end := bytes.Index(data, []byte("\n---"))
	if end == -1 {
		return fm, nil
	}
	yamlBlock := data[:end]

	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return fm, xerrors.Errorf("parse README.md frontmatter: %w", err)
	}
	return fm, nil
}
