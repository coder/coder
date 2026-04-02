package workspacesdk

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

// markdownCommentRe strips HTML comments from skill file bodies so
// they don't leak into the LLM prompt.
var markdownCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)

// ParseSkillFrontmatter extracts name, description, and the
// remaining body from a skill meta file. The expected format is
// YAML-ish frontmatter delimited by "---" lines:
//
//	---
//	name: my-skill
//	description: Does a thing
//	---
//	Body text here...
func ParseSkillFrontmatter(content string) (name, description, body string, err error) {
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", "", xerrors.New(
			"missing opening frontmatter delimiter",
		)
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx < 0 {
		return "", "", "", xerrors.New(
			"missing closing frontmatter delimiter",
		)
	}

	for _, line := range lines[1:closingIdx] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// Strip surrounding quotes from YAML string values.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		switch strings.ToLower(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}

	if name == "" {
		return "", "", "", xerrors.New(
			"frontmatter missing required 'name' field",
		)
	}

	// Everything after the closing delimiter is the body.
	body = strings.Join(lines[closingIdx+1:], "\n")
	body = markdownCommentRe.ReplaceAllString(body, "")
	body = strings.TrimSpace(body)

	return name, description, body, nil
}
