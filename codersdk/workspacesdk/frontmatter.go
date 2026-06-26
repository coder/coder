package workspacesdk

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
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

func frontmatterStringField(frontmatter map[string]any, key string) (string, bool, error) {
	value, ok := frontmatter[key]
	if !ok {
		return "", false, nil
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", true, xerrors.Errorf("frontmatter field %q must be a string", key)
	}
	return strings.TrimRight(stringValue, "\r\n"), true, nil
}

// ParseSkillFrontmatter extracts name, description, and the
// remaining body from a skill meta file. The expected format is
// YAML frontmatter delimited by "---" lines:
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

	frontmatterContent := strings.Join(lines[1:closingIdx], "\n")
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(frontmatterContent), &frontmatter); err != nil {
		return "", "", "", xerrors.Errorf("parse frontmatter YAML: %w", err)
	}

	name, ok, err := frontmatterStringField(frontmatter, "name")
	if err != nil {
		return "", "", "", xerrors.Errorf("%w: %v", ErrFrontmatterNameRequired, err)
	}
	if !ok || name == "" {
		return "", "", "", xerrors.Errorf("%w", ErrFrontmatterNameRequired)
	}
	description, _, err = frontmatterStringField(frontmatter, "description")
	if err != nil {
		return "", "", "", err
	}

	// Everything after the closing delimiter is the body.
	body = strings.Join(lines[closingIdx+1:], "\n")
	body = markdownCommentRe.ReplaceAllString(body, "")
	body = strings.TrimSpace(body)

	return name, description, body, nil
}
