package workspacesdk

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

// SkillNameRegex is the regular expression used to validate kebab-case skill names.
const SkillNameRegex = "^[a-z0-9]+(-[a-z0-9]+)*$"

// MaxSkillMetaBytes is the maximum raw Markdown size accepted for a skill meta file.
const MaxSkillMetaBytes = 64 * 1024

// SkillNamePattern is the compiled pattern used to validate kebab-case skill names.
var SkillNamePattern = regexp.MustCompile(SkillNameRegex)

// markdownCommentRe strips HTML comments from skill file bodies so
// they don't leak into the LLM prompt.
var markdownCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)

// ErrFrontmatterNameRequired is returned by ParseSkillFrontmatter when
// the frontmatter is missing a required name field.
var ErrFrontmatterNameRequired = xerrors.New("frontmatter missing required 'name' field")

func parseFrontmatterValue(value string) string {
	if len(value) < 2 {
		return value
	}
	if value[0] == '"' && value[len(value)-1] == '"' {
		inner := value[1 : len(value)-1]
		var builder strings.Builder
		builder.Grow(len(inner))
		escaping := false
		for _, r := range inner {
			if escaping {
				if r != '"' && r != '\\' {
					_, _ = builder.WriteRune('\\')
				}
				_, _ = builder.WriteRune(r)
				escaping = false
				continue
			}
			if r == '\\' {
				escaping = true
				continue
			}
			_, _ = builder.WriteRune(r)
		}
		if escaping {
			_, _ = builder.WriteRune('\\')
		}
		return builder.String()
	}
	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

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
		value = parseFrontmatterValue(value)
		switch strings.ToLower(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}

	if name == "" {
		return "", "", "", xerrors.Errorf("%w", ErrFrontmatterNameRequired)
	}

	// Everything after the closing delimiter is the body.
	body = strings.Join(lines[closingIdx+1:], "\n")
	body = markdownCommentRe.ReplaceAllString(body, "")
	body = strings.TrimSpace(body)

	return name, description, body, nil
}
