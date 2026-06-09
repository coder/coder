// Package frontmatter parses the YAML frontmatter block at the top of a
// template README. It is intentionally lightweight (a fence split plus a
// yaml.Unmarshal) so it can be imported by both the coderd chat tools and the
// codersdk agent tools without pulling a heavy markdown dependency into the
// server or agent binary.
package frontmatter

import (
	"bufio"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	coderstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// AgentDescriptionMaxRunes bounds the agent_description value surfaced to
// agents. It matches the cap used by the agent template-selection tools.
const AgentDescriptionMaxRunes = 2048

// Frontmatter is the locked set of recognized README frontmatter keys. Unknown
// keys are ignored by yaml.Unmarshal, so adding keys here is the only way to
// recognize new frontmatter.
type Frontmatter struct {
	DisplayName      string   `yaml:"display_name"`
	Description      string   `yaml:"description"`
	Icon             string   `yaml:"icon"`
	MaintainerGithub string   `yaml:"maintainer_github"`
	Verified         bool     `yaml:"verified"`
	Tags             []string `yaml:"tags"`
	AgentDescription string   `yaml:"agent_description"`
}

// Parse splits the leading "---" fenced YAML frontmatter from the markdown
// body and unmarshals it into a Frontmatter. Unknown keys are ignored.
//
// It returns an error when the README is empty, lacks two frontmatter fences,
// or the frontmatter is not valid YAML. Callers that treat missing frontmatter
// as benign (the agent tools) should ignore the error and use the zero value;
// callers that require frontmatter (the example generator) should treat the
// error as fatal. Parse never panics.
func Parse(readme string) (Frontmatter, string, error) {
	raw, body, err := separate(readme)
	if err != nil {
		return Frontmatter{}, "", err
	}
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
		return Frontmatter{}, "", xerrors.Errorf("parse readme frontmatter as yaml: %w", err)
	}
	return fm, body, nil
}

// AgentDescription returns the trimmed, truncated agent_description from a
// README's frontmatter, or empty string when absent, blank, or unparseable.
// This is the single entry point used by the agent template-selection tools.
func AgentDescription(readme string) string {
	fm, _, err := Parse(readme)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(fm.AgentDescription)
	return coderstrings.Truncate(v, AgentDescriptionMaxRunes, coderstrings.TruncateWithEllipsis)
}

// separate returns the raw YAML frontmatter and the markdown body. Frontmatter
// lines are preserved verbatim (indentation intact) so nested YAML parses
// correctly; only the body is trimmed of surrounding whitespace.
func separate(readme string) (frontmatter string, body string, err error) {
	if strings.TrimSpace(readme) == "" {
		return "", "", xerrors.New("readme is empty")
	}

	const fence = "---"
	var fm, bd strings.Builder
	fenceCount := 0

	sc := bufio.NewScanner(strings.NewReader(strings.TrimSpace(readme)))
	// Readmes can be up to 1 MiB; allow long lines so the body is not split.
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for sc.Scan() {
		// bufio.Scanner strips a trailing carriage return, so CRLF readmes
		// compare cleanly against the fence and produce LF bodies.
		line := sc.Text()
		if fenceCount < 2 && line == fence {
			fenceCount++
			continue
		}
		// If the first non-blank line is not a fence, there is no frontmatter.
		if fenceCount == 0 {
			break
		}
		if fenceCount >= 2 {
			_, _ = bd.WriteString(line)
			_ = bd.WriteByte('\n')
		} else {
			_, _ = fm.WriteString(line)
			_ = fm.WriteByte('\n')
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", xerrors.Errorf("scan readme: %w", err)
	}
	if fenceCount < 2 {
		return "", "", xerrors.New("readme does not have two frontmatter fences")
	}

	return fm.String(), strings.TrimSpace(bd.String()), nil
}
