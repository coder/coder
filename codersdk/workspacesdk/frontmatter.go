package workspacesdk

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// SkillNameRegex is the regular expression used to validate kebab-case skill names.
const SkillNameRegex = "^[a-z0-9]+(-[a-z0-9]+)*$"

// MaxSkillMetaBytes is the maximum raw Markdown size accepted for a skill meta file.
const MaxSkillMetaBytes = 64 * 1024

// SkillNamePattern is the compiled pattern used to validate kebab-case skill names.
var SkillNamePattern = regexp.MustCompile(SkillNameRegex)

// MaxPluginNameLength is the maximum length in bytes of an Agent Plugin name.
const MaxPluginNameLength = 64

// ValidatePluginName checks name against the Agent Plugins manifest name
// grammar: 1 to MaxPluginNameLength bytes from [a-z0-9.-], starting and
// ending with an alphanumeric character, with no "--" or ".." runs.
func ValidatePluginName(name string) error {
	if name == "" {
		return xerrors.New("plugin name is required")
	}
	if len(name) > MaxPluginNameLength {
		return xerrors.Errorf("plugin name is %d bytes, maximum is %d", len(name), MaxPluginNameLength)
	}
	for i, c := range name {
		isAlnum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if isAlnum {
			continue
		}
		if c != '.' && c != '-' {
			return xerrors.Errorf("plugin name %q contains invalid character %q; allowed are a-z, 0-9, '.', and '-'", name, c)
		}
		if i == 0 || i == len(name)-1 {
			return xerrors.Errorf("plugin name %q must start and end with a letter or digit", name)
		}
		if rune(name[i-1]) == c {
			return xerrors.Errorf("plugin name %q must not contain %q", name, string([]rune{c, c}))
		}
	}
	return nil
}

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
	frontmatter, body, err := splitSkillFrontmatter(content)
	if err != nil {
		return "", "", "", err
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

	return name, description, body, nil
}

// MaxSkillCompatibilityRunes is the Agent Skills limit, in characters,
// on a skill's compatibility frontmatter field.
const MaxSkillCompatibilityRunes = 500

// ValidateSkillOptionalFrontmatter checks the optional Agent Skills
// frontmatter fields of a skill meta file: license and allowed-tools
// must be strings, compatibility must be a string of 1 to
// MaxSkillCompatibilityRunes characters, and metadata must map string
// keys to string values. Absent fields and fields outside the Agent
// Skills specification are not checked.
func ValidateSkillOptionalFrontmatter(content string) error {
	frontmatter, _, err := splitSkillFrontmatter(content)
	if err != nil {
		return err
	}
	for _, key := range []string{"license", "allowed-tools"} {
		if _, _, err := frontmatterStringField(frontmatter, key); err != nil {
			return err
		}
	}
	compatibility, ok, err := frontmatterStringField(frontmatter, "compatibility")
	if err != nil {
		return err
	}
	if n := utf8.RuneCountInString(compatibility); ok && (n == 0 || n > MaxSkillCompatibilityRunes) {
		return xerrors.Errorf("frontmatter field \"compatibility\" is %d characters, must be 1 to %d", n, MaxSkillCompatibilityRunes)
	}
	metadata, ok := frontmatter["metadata"]
	if !ok {
		return nil
	}
	fields, ok := metadata.(map[string]any)
	if !ok {
		return xerrors.New("frontmatter field \"metadata\" must be a mapping of strings to strings")
	}
	for key, value := range fields {
		if _, ok := value.(string); !ok {
			return xerrors.Errorf("frontmatter field \"metadata\" value for %q must be a string", key)
		}
	}
	return nil
}

// SkillDisablesModelInvocation reports whether a skill meta file's
// frontmatter sets disable-model-invocation to the YAML boolean true.
// Agents must not discover or load such a skill on their own; only a
// person can invoke it. Any other value, and content whose frontmatter
// does not parse, leaves the skill model-invocable.
func SkillDisablesModelInvocation(content string) bool {
	frontmatter, _, err := splitSkillFrontmatter(content)
	if err != nil {
		return false
	}
	disabled, _ := frontmatter["disable-model-invocation"].(bool)
	return disabled
}

// splitSkillFrontmatter decodes the YAML frontmatter of a skill meta file
// and returns it with the body that follows, with HTML comments stripped
// and surrounding whitespace trimmed.
func splitSkillFrontmatter(content string) (frontmatter map[string]any, body string, err error) {
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", xerrors.New(
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
		return nil, "", xerrors.New(
			"missing closing frontmatter delimiter",
		)
	}

	frontmatterContent := strings.Join(lines[1:closingIdx], "\n")
	if err := yaml.Unmarshal([]byte(frontmatterContent), &frontmatter); err != nil {
		return nil, "", xerrors.Errorf("parse frontmatter YAML: %w", err)
	}

	// Everything after the closing delimiter is the body.
	body = strings.Join(lines[closingIdx+1:], "\n")
	body = markdownCommentRe.ReplaceAllString(body, "")
	body = strings.TrimSpace(body)

	return frontmatter, body, nil
}
