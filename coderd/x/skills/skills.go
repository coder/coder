package skills

import (
	"maps"
	"slices"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// MaxPersonalSkillSizeBytes is the maximum raw Markdown size accepted for a
// personal skill upload.
const MaxPersonalSkillSizeBytes = workspacesdk.MaxSkillMetaBytes

// MaxPersonalSkillNameBytes is the maximum skill name length accepted for a
// personal skill upload. Skill names are also used in URL paths.
const MaxPersonalSkillNameBytes = 256

// MaxPersonalSkillDescriptionBytes is the maximum frontmatter description size
// accepted for a personal skill upload.
const MaxPersonalSkillDescriptionBytes = 4096

// MaxPersonalSkillsPerUser is the maximum number of personal skills a user may
// create.
const MaxPersonalSkillsPerUser = 100

// Source identifies where a skill came from.
type Source string

const (
	// SourcePersonal identifies a user-owned, DB-backed skill.
	SourcePersonal Source = "personal"
	// SourceOrganization identifies an organization-owned, DB-backed skill.
	SourceOrganization Source = "org"
	// SourceProject identifies a chat-project-owned, DB-backed skill.
	SourceProject Source = "project"
	// SourceWorkspace identifies a filesystem-discovered workspace skill.
	SourceWorkspace Source = "workspace"
)

var (
	// ErrInvalidSkillName indicates that a skill name is missing, not valid
	// kebab-case, or exceeds the maximum length.
	ErrInvalidSkillName = xerrors.New("invalid skill name")
	// ErrSkillBodyRequired indicates that the skill has no body after frontmatter.
	ErrSkillBodyRequired = xerrors.New("skill body is required")
	// ErrSkillTooLarge indicates that the raw skill Markdown is too large.
	ErrSkillTooLarge = xerrors.New("skill is too large")
	// ErrSkillDescriptionTooLarge indicates that the description is too large.
	ErrSkillDescriptionTooLarge = xerrors.New("skill description is too large")
	// ErrSkillNotFound indicates that a skill lookup did not match any alias.
	ErrSkillNotFound = xerrors.New("skill not found")
	// ErrSkillAmbiguous indicates that a skill lookup matched multiple sources.
	ErrSkillAmbiguous = xerrors.New("skill lookup is ambiguous")
)

// Skill is the source-aware metadata needed to list and resolve a skill.
type Skill struct {
	Name        string
	Description string
	Source      Source
}

// ParsedSkill is a parsed skill with the Markdown body after frontmatter.
// Body has HTML comments stripped and surrounding whitespace trimmed.
type ParsedSkill struct {
	Skill
	Body string
}

// ResolvedSkill is a skill with the alias exposed to chat tools.
type ResolvedSkill struct {
	Skill
	Alias string
}

// ParsePersonalSkillMarkdown parses raw personal skill Markdown and enforces
// the personal skill contract. The raw size must not exceed
// MaxPersonalSkillSizeBytes, frontmatter must contain a valid kebab-case name,
// the skill name must not exceed MaxPersonalSkillNameBytes, the description must
// not exceed MaxPersonalSkillDescriptionBytes, and the body after frontmatter
// must be non-empty.
func ParsePersonalSkillMarkdown(raw []byte) (ParsedSkill, error) {
	if len(raw) > MaxPersonalSkillSizeBytes {
		return ParsedSkill{}, xerrors.Errorf(
			"got %d bytes, maximum is %d bytes: %w",
			len(raw),
			MaxPersonalSkillSizeBytes,
			ErrSkillTooLarge,
		)
	}

	name, description, body, err := workspacesdk.ParseSkillFrontmatter(string(raw))
	if err != nil {
		if xerrors.Is(err, workspacesdk.ErrFrontmatterNameRequired) {
			return ParsedSkill{}, xerrors.Errorf("frontmatter must contain a 'name' field: %w", ErrInvalidSkillName)
		}
		return ParsedSkill{}, xerrors.Errorf("parse skill frontmatter: %w", err)
	}
	if !workspacesdk.SkillNamePattern.MatchString(name) {
		return ParsedSkill{}, xerrors.Errorf(
			"%q must match %s: %w",
			name,
			workspacesdk.SkillNameRegex,
			ErrInvalidSkillName,
		)
	}
	nameBytes := len(name)
	if nameBytes > MaxPersonalSkillNameBytes {
		return ParsedSkill{}, xerrors.Errorf(
			"%q is %d bytes, maximum is %d bytes: %w",
			name,
			nameBytes,
			MaxPersonalSkillNameBytes,
			ErrInvalidSkillName,
		)
	}
	descriptionBytes := len(description)
	if descriptionBytes > MaxPersonalSkillDescriptionBytes {
		return ParsedSkill{}, xerrors.Errorf(
			"got %d bytes, maximum is %d bytes: %w",
			descriptionBytes,
			MaxPersonalSkillDescriptionBytes,
			ErrSkillDescriptionTooLarge,
		)
	}
	if strings.TrimSpace(body) == "" {
		return ParsedSkill{}, xerrors.Errorf(
			"skill %q has no content after frontmatter: %w",
			name,
			ErrSkillBodyRequired,
		)
	}

	return ParsedSkill{
		Skill: Skill{
			Name:        name,
			Description: description,
			Source:      SourcePersonal,
		},
		Body: body,
	}, nil
}

// disableModelInvocationKey is the SKILL.md frontmatter key that Claude Code,
// Cursor, and VS Code use to mark a skill that only users may invoke.
const disableModelInvocationKey = "disable-model-invocation"

// RemoveDisableModelInvocation removes the top-level disable-model-invocation
// key from the frontmatter of raw skill Markdown, and reports whether the key
// was the YAML boolean true. Stored skills keep that setting in their
// model_invocable column, so removing the key keeps the stored file from
// disagreeing with the column. raw should already have passed
// ParsePersonalSkillMarkdown. When the key is absent, or cannot be removed
// line by line (for example, a quoted key), content is raw unchanged.
func RemoveDisableModelInvocation(raw string) (content string, disabled bool) {
	const bom = "\xef\xbb\xbf"
	prefix := ""
	rest := raw
	if strings.HasPrefix(raw, bom) {
		prefix, rest = bom, raw[len(bom):]
	}
	// SplitAfter keeps each line's newline, so joining the lines restores
	// the original bytes, including CRLF line endings.
	lines := strings.SplitAfter(rest, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return raw, false
	}
	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx < 0 {
		return raw, false
	}

	frontmatter := lines[1:closingIdx]
	value, ok, err := frontmatterValue(frontmatter, disableModelInvocationKey)
	if err != nil || !ok {
		return raw, false
	}
	disabled, _ = value.(bool)

	kept := make([]string, 0, len(frontmatter))
	for i := 0; i < len(frontmatter); i++ {
		if !strings.HasPrefix(frontmatter[i], disableModelInvocationKey+":") {
			kept = append(kept, frontmatter[i])
			continue
		}
		// Skip the indented continuation lines of a multi-line value.
		for i+1 < len(frontmatter) && isYAMLContinuationLine(frontmatter[i+1]) {
			i++
		}
	}
	if _, stillSet, err := frontmatterValue(kept, disableModelInvocationKey); err != nil || stillSet {
		return raw, disabled
	}

	return prefix + lines[0] + strings.Join(kept, "") + strings.Join(lines[closingIdx:], ""), disabled
}

// frontmatterValue decodes frontmatter lines as a YAML mapping and returns
// the value of key.
func frontmatterValue(lines []string, key string) (value any, ok bool, err error) {
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "")), &frontmatter); err != nil {
		return nil, false, xerrors.Errorf("parse frontmatter YAML: %w", err)
	}
	value, ok = frontmatter[key]
	return value, ok, nil
}

// isYAMLContinuationLine reports whether line continues the value of the
// previous top-level key: it is blank or indented.
func isYAMLContinuationLine(line string) bool {
	return strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

// SourceSkills holds the skills found in one source.
type SourceSkills struct {
	Source Source
	Skills []Skill
}

// MergeSkills combines skills from several sources into a deterministic list
// with aliases for chat tool display and lookup. Skills are sorted by name,
// then by the order of sources as passed. A name found in more than one source
// gets a qualified alias in every source that has it, so no source overrides
// another; a name found in one source keeps its bare alias. Skill names must
// already be valid kebab-case names because qualified aliases use / as a
// separator. If a source contains duplicate names, the first skill for that
// source wins.
func MergeSkills(sourceSkills ...SourceSkills) []ResolvedSkill {
	sources := make([]map[string]Skill, 0, len(sourceSkills))
	for _, source := range sourceSkills {
		sources = append(sources, skillsByName(source.Skills, source.Source))
	}

	names := make(map[string]struct{})
	for _, byName := range sources {
		for name := range byName {
			names[name] = struct{}{}
		}
	}

	resolved := make([]ResolvedSkill, 0, len(names))
	for _, name := range slices.Sorted(maps.Keys(names)) {
		var matches []Skill
		for _, byName := range sources {
			if skill, ok := byName[name]; ok {
				matches = append(matches, skill)
			}
		}
		for _, skill := range matches {
			alias := name
			if len(matches) > 1 {
				alias = QualifiedAlias(skill.Source, name)
			}
			resolved = append(resolved, ResolvedSkill{Skill: skill, Alias: alias})
		}
	}
	return resolved
}

// Lookup finds a resolved skill by bare alias or qualified source alias. It
// returns ErrSkillNotFound if no alias matches, or ErrSkillAmbiguous if a bare
// name matches skills from multiple sources.
func Lookup(resolved []ResolvedSkill, lookup string) (ResolvedSkill, error) {
	var (
		bareNameMatch ResolvedSkill
		matches       []string
	)
	for _, skill := range resolved {
		qualifiedAlias := QualifiedAlias(skill.Source, skill.Name)
		if lookup == skill.Alias || lookup == qualifiedAlias {
			return skill, nil
		}
		if lookup == skill.Name {
			bareNameMatch = skill
			matches = append(matches, qualifiedAlias)
		}
	}
	switch len(matches) {
	case 0:
		return ResolvedSkill{}, xerrors.Errorf("%q: %w", lookup, ErrSkillNotFound)
	case 1:
		return bareNameMatch, nil
	default:
		return ResolvedSkill{}, xerrors.Errorf(
			"%q matches %s: %w",
			lookup,
			strings.Join(matches, ", "),
			ErrSkillAmbiguous,
		)
	}
}

// QualifiedAlias returns the stable source-qualified alias for a skill name.
func QualifiedAlias(source Source, name string) string {
	return string(source) + "/" + name
}

func skillsByName(skills []Skill, source Source) map[string]Skill {
	byName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		if _, ok := byName[skill.Name]; ok {
			continue
		}
		skill.Source = source
		byName[skill.Name] = skill
	}
	return byName
}
