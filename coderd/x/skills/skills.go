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
const MaxPersonalSkillSizeBytes = 64 * 1024

// MaxPersonalSkillsPerUser is the maximum number of personal skills a user may
// create.
const MaxPersonalSkillsPerUser = 100

// Source identifies where a skill came from.
type Source string

const (
	// SourcePersonal identifies a user-owned, DB-backed skill.
	SourcePersonal Source = "personal"
	// SourceWorkspace identifies a filesystem-discovered workspace skill.
	SourceWorkspace Source = "workspace"
)

var (
	// ErrInvalidSkillName indicates that a parsed skill name is not kebab-case.
	ErrInvalidSkillName = xerrors.New("invalid skill name")
	// ErrSkillBodyRequired indicates that the skill has no body after frontmatter.
	ErrSkillBodyRequired = xerrors.New("skill body is required")
	// ErrSkillTooLarge indicates that the raw skill Markdown is too large.
	ErrSkillTooLarge = xerrors.New("skill is too large")
	// ErrSkillNotFound indicates that a skill lookup did not match any alias.
	ErrSkillNotFound = xerrors.New("skill not found")
)

// Skill is the source-aware metadata needed to list and resolve a skill.
type Skill struct {
	Name        string
	Description string
	Source      Source
}

// ParsedSkill is a parsed skill with the Markdown body after frontmatter.
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
			return ParsedSkill{}, ErrInvalidSkillName
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

// ValidatePersonalSkillMarkdown parses and validates raw personal skill
// Markdown. It returns source-aware metadata and the body after frontmatter.
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

	return parsed, nil
}

// MergeSkills combines personal and workspace skills into a deterministic list
// with aliases for chat tool display and lookup.
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
	for _, skill := range resolved {
		if lookup == skill.Alias || lookup == QualifiedAlias(skill.Source, skill.Name) {
			return skill, nil
		}
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
