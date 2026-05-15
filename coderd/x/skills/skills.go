package skills

import (
	"maps"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// MaxPersonalSkillSizeBytes is the maximum raw Markdown size accepted for a
// personal skill upload.
const MaxPersonalSkillSizeBytes = workspacesdk.MaxSkillMetaBytes

// MaxPersonalSkillNameBytes is the maximum skill name length accepted for a
// personal skill upload. Skill names are also used in URL paths.
const MaxPersonalSkillNameBytes = 256

// Source identifies where a skill came from.
type Source string

const (
	// SourcePersonal identifies a user-owned, DB-backed skill.
	SourcePersonal Source = "personal"
	// SourceWorkspace identifies a filesystem-discovered workspace skill.
	SourceWorkspace Source = "workspace"
)

var (
	// ErrInvalidSkillName indicates that a skill name is missing or not valid kebab-case.
	ErrInvalidSkillName = xerrors.New("invalid skill name")
	// ErrSkillBodyRequired indicates that the skill has no body after frontmatter.
	ErrSkillBodyRequired = xerrors.New("skill body is required")
	// ErrSkillTooLarge indicates that the raw skill Markdown is too large.
	ErrSkillTooLarge = xerrors.New("skill is too large")
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

// ParsePersonalSkillMarkdown parses raw personal skill Markdown. It returns
// source-aware metadata and the body after frontmatter.
func ParsePersonalSkillMarkdown(raw []byte) (ParsedSkill, error) {
	name, description, body, err := workspacesdk.ParseSkillFrontmatter(string(raw))
	if err != nil {
		if xerrors.Is(err, workspacesdk.ErrFrontmatterNameRequired) {
			return ParsedSkill{}, xerrors.Errorf("%w: frontmatter must contain a 'name' field", ErrInvalidSkillName)
		}
		return ParsedSkill{}, xerrors.Errorf("parse skill frontmatter: %w", err)
	}
	if strings.TrimSpace(body) == "" {
		return ParsedSkill{}, xerrors.Errorf(
			"%w: skill %q has no content after frontmatter",
			ErrSkillBodyRequired,
			name,
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

// ValidatePersonalSkillMarkdown parses raw personal skill Markdown and
// enforces upload constraints. The raw size must not exceed
// MaxPersonalSkillSizeBytes, frontmatter must contain a valid kebab-case name,
// the skill name must not exceed MaxPersonalSkillNameBytes, and the body after
// frontmatter must be non-empty.
func ValidatePersonalSkillMarkdown(raw []byte) (ParsedSkill, error) {
	if len(raw) > MaxPersonalSkillSizeBytes {
		return ParsedSkill{}, xerrors.Errorf(
			"%w: got %d bytes, maximum is %d bytes",
			ErrSkillTooLarge,
			len(raw),
			MaxPersonalSkillSizeBytes,
		)
	}

	parsed, err := ParsePersonalSkillMarkdown(raw)
	if err != nil {
		return ParsedSkill{}, err
	}
	if !workspacesdk.SkillNamePattern.MatchString(parsed.Name) {
		return ParsedSkill{}, xerrors.Errorf(
			"%w: %q must match %s",
			ErrInvalidSkillName,
			parsed.Name,
			workspacesdk.SkillNameRegex,
		)
	}
	nameBytes := len([]byte(parsed.Name))
	if nameBytes > MaxPersonalSkillNameBytes {
		return ParsedSkill{}, xerrors.Errorf(
			"%w: %q is %d bytes, maximum is %d bytes",
			ErrInvalidSkillName,
			parsed.Name,
			nameBytes,
			MaxPersonalSkillNameBytes,
		)
	}

	return parsed, nil
}

// MergeSkills combines personal and workspace skills into a deterministic list
// with aliases for chat tool display and lookup. Skill names must already be
// valid kebab-case names because qualified aliases use / as a separator. If a
// source contains duplicate names, the first skill for that source wins.
func MergeSkills(personalSkills, workspaceSkills []Skill) []ResolvedSkill {
	personalByName := skillsByName(personalSkills, SourcePersonal)
	workspaceByName := skillsByName(workspaceSkills, SourceWorkspace)

	names := make(map[string]struct{}, len(personalByName)+len(workspaceByName))
	for name := range personalByName {
		names[name] = struct{}{}
	}
	for name := range workspaceByName {
		names[name] = struct{}{}
	}

	resolved := make([]ResolvedSkill, 0, len(personalByName)+len(workspaceByName))
	for _, name := range slices.Sorted(maps.Keys(names)) {
		personal, hasPersonal := personalByName[name]
		workspace, hasWorkspace := workspaceByName[name]
		if hasPersonal && hasWorkspace {
			resolved = append(resolved,
				ResolvedSkill{
					Skill: personal,
					Alias: QualifiedAlias(SourcePersonal, name),
				},
				ResolvedSkill{
					Skill: workspace,
					Alias: QualifiedAlias(SourceWorkspace, name),
				},
			)
			continue
		}
		if hasPersonal {
			resolved = append(resolved, ResolvedSkill{
				Skill: personal,
				Alias: name,
			})
			continue
		}
		resolved = append(resolved, ResolvedSkill{
			Skill: workspace,
			Alias: name,
		})
	}
	return resolved
}

// Lookup finds a resolved skill by bare alias or qualified source alias.
func Lookup(resolved []ResolvedSkill, lookup string) (ResolvedSkill, error) {
	var matches []string
	for _, skill := range resolved {
		qualifiedAlias := QualifiedAlias(skill.Source, skill.Name)
		if lookup == skill.Alias || lookup == qualifiedAlias {
			return skill, nil
		}
		if lookup == skill.Name {
			matches = append(matches, qualifiedAlias)
		}
	}
	if len(matches) > 1 {
		return ResolvedSkill{}, xerrors.Errorf(
			"%w: %q matches %s",
			ErrSkillAmbiguous,
			lookup,
			strings.Join(matches, ", "),
		)
	}
	return ResolvedSkill{}, xerrors.Errorf("%w: %q", ErrSkillNotFound, lookup)
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
