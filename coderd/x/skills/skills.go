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
	// SourceWorkspace identifies a filesystem-discovered workspace skill.
	SourceWorkspace Source = "workspace"
	// SourcePlugin identifies a workspace skill shipped inside an Agent
	// Plugin's skills/ directory. Its identity is the (plugin, name) pair.
	SourcePlugin Source = "plugin"
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
	// Plugin is the owning plugin name for SourcePlugin skills and empty
	// otherwise.
	Plugin string
}

// QualifiedAlias returns the stable source-qualified alias for the skill:
// personal/<name>, workspace/<name>, or plugin/<plugin>/<name>. The fixed
// source token keeps plugin names such as "personal" from colliding with
// the other sources.
func (s Skill) QualifiedAlias() string {
	if s.Source == SourcePlugin {
		return string(SourcePlugin) + "/" + s.Plugin + "/" + s.Name
	}
	return string(s.Source) + "/" + s.Name
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
			"%w: got %d bytes, maximum is %d bytes",
			ErrSkillTooLarge,
			len(raw),
			MaxPersonalSkillSizeBytes,
		)
	}

	name, description, body, err := workspacesdk.ParseSkillFrontmatter(string(raw))
	if err != nil {
		if xerrors.Is(err, workspacesdk.ErrFrontmatterNameRequired) {
			return ParsedSkill{}, xerrors.Errorf("%w: frontmatter must contain a 'name' field", ErrInvalidSkillName)
		}
		return ParsedSkill{}, xerrors.Errorf("parse skill frontmatter: %w", err)
	}
	if !workspacesdk.SkillNamePattern.MatchString(name) {
		return ParsedSkill{}, xerrors.Errorf(
			"%w: %q must match %s",
			ErrInvalidSkillName,
			name,
			workspacesdk.SkillNameRegex,
		)
	}
	nameBytes := len(name)
	if nameBytes > MaxPersonalSkillNameBytes {
		return ParsedSkill{}, xerrors.Errorf(
			"%w: %q is %d bytes, maximum is %d bytes",
			ErrInvalidSkillName,
			name,
			nameBytes,
			MaxPersonalSkillNameBytes,
		)
	}
	descriptionBytes := len(description)
	if descriptionBytes > MaxPersonalSkillDescriptionBytes {
		return ParsedSkill{}, xerrors.Errorf(
			"%w: got %d bytes, maximum is %d bytes",
			ErrSkillDescriptionTooLarge,
			descriptionBytes,
			MaxPersonalSkillDescriptionBytes,
		)
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

// MergeSkills combines personal, workspace, and plugin skills into a
// deterministic list with aliases for chat tool display and lookup. Skill
// names must already be valid kebab-case names because qualified aliases use
// / as a separator. A name held by exactly one skill gets a bare alias; when
// several skills share a name, every holder gets its qualified alias so none
// is silently selected. Holders of a shared name are ordered personal,
// workspace, then plugins by plugin name. Within the personal and workspace
// sources duplicate names keep the first skill; within plugins the identity
// is (plugin, name), so distinct plugins may each ship a skill of the same
// name.
func MergeSkills(personalSkills, workspaceSkills, pluginSkills []Skill) []ResolvedSkill {
	personalByName := skillsByName(personalSkills, SourcePersonal)
	workspaceByName := skillsByName(workspaceSkills, SourceWorkspace)
	pluginsByName := pluginSkillsByName(pluginSkills)

	names := make(map[string]struct{}, len(personalByName)+len(workspaceByName)+len(pluginsByName))
	for name := range personalByName {
		names[name] = struct{}{}
	}
	for name := range workspaceByName {
		names[name] = struct{}{}
	}
	for name := range pluginsByName {
		names[name] = struct{}{}
	}

	resolved := make([]ResolvedSkill, 0, len(names))
	for _, name := range slices.Sorted(maps.Keys(names)) {
		holders := make([]Skill, 0, 2+len(pluginsByName[name]))
		if personal, ok := personalByName[name]; ok {
			holders = append(holders, personal)
		}
		if workspace, ok := workspaceByName[name]; ok {
			holders = append(holders, workspace)
		}
		holders = append(holders, pluginsByName[name]...)

		if len(holders) == 1 {
			resolved = append(resolved, ResolvedSkill{Skill: holders[0], Alias: name})
			continue
		}
		for _, holder := range holders {
			resolved = append(resolved, ResolvedSkill{Skill: holder, Alias: holder.QualifiedAlias()})
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
		qualifiedAlias := skill.QualifiedAlias()
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
		return ResolvedSkill{}, xerrors.Errorf("%w: %q", ErrSkillNotFound, lookup)
	case 1:
		return bareNameMatch, nil
	default:
		return ResolvedSkill{}, xerrors.Errorf(
			"%w: %q matches %s",
			ErrSkillAmbiguous,
			lookup,
			strings.Join(matches, ", "),
		)
	}
}

func skillsByName(skills []Skill, source Source) map[string]Skill {
	byName := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		if _, ok := byName[skill.Name]; ok {
			continue
		}
		skill.Source = source
		skill.Plugin = ""
		byName[skill.Name] = skill
	}
	return byName
}

// pluginSkillsByName groups plugin skills by skill name, keeping the first
// skill per (plugin, name) pair and ordering each group by plugin name.
// Skills without a plugin name are dropped because they cannot be aliased.
func pluginSkillsByName(skills []Skill) map[string][]Skill {
	seen := make(map[[2]string]struct{}, len(skills))
	byName := make(map[string][]Skill)
	for _, skill := range skills {
		if skill.Plugin == "" {
			continue
		}
		key := [2]string{skill.Plugin, skill.Name}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		skill.Source = SourcePlugin
		byName[skill.Name] = append(byName[skill.Name], skill)
	}
	for name := range byName {
		slices.SortFunc(byName[name], func(a, b Skill) int {
			return strings.Compare(a.Plugin, b.Plugin)
		})
	}
	return byName
}
