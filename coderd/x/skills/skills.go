package skills

import (
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

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
	// ErrSkillInvocationKeyInvalid indicates that the frontmatter spells
	// disable-model-invocation in a way that cannot be set to true or false.
	ErrSkillInvocationKeyInvalid = xerrors.New("invalid disable-model-invocation key")
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

// WithModelInvocable returns raw skill Markdown whose frontmatter agrees with
// modelInvocable: disable-model-invocation reads as true exactly when
// modelInvocable is false. workspacesdk.SkillDisableModelInvocationValue is
// the only judge of what a file says, so a stored skill and a workspace
// skill read the same file the same way.
//
// Stored skills keep the setting in their model_invocable column and the
// file as uploaded, so this runs on the file that leaves the server. Content
// that already agrees is returned unchanged. Otherwise the value of an
// existing top-level key is replaced, or the key is added before the first
// one, and every other byte is kept. ok is false, and content is raw, when
// no such edit leaves the other frontmatter fields as they were.
func WithModelInvocable(raw string, modelInvocable bool) (content string, ok bool) {
	disable := !modelInvocable
	if disabled, _ := workspacesdk.SkillDisableModelInvocationValue(raw); disabled == disable {
		return raw, true
	}

	start, end, ok := frontmatterSpan(raw)
	if !ok {
		return raw, false
	}
	frontmatter := raw[start:end]
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatter), &doc); err != nil {
		return raw, false
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return raw, false
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode || len(mapping.Content) == 0 {
		return raw, false
	}

	value := strconv.FormatBool(disable)
	var edited string
	if node := mappingValue(mapping, disableModelInvocationKey); node != nil {
		edited, ok = replaceScalar(frontmatter, node, value)
	} else {
		newline := "\n"
		if strings.HasSuffix(raw[:start], "\r\n") {
			newline = "\r\n"
		}
		edited, ok = insertFirstEntry(frontmatter, mapping, disableModelInvocationKey+": "+value, newline)
	}
	// The edit goes by the position YAML reported for a node, so check the
	// result instead of trusting the position: the other fields must decode
	// as before, and the file must now say what was asked.
	if !ok || !sameFieldsExcept(frontmatter, edited, disableModelInvocationKey) {
		return raw, false
	}
	content = raw[:start] + edited + raw[end:]
	if disabled, _ := workspacesdk.SkillDisableModelInvocationValue(content); disabled != disable {
		return raw, false
	}
	return content, true
}

// ValidateModelInvocationKey reports whether WithModelInvocable can make raw
// agree with either setting. Uploads that fail are rejected, so a later
// change to model_invocable never leaves the file saying the opposite. raw
// should already have passed ParsePersonalSkillMarkdown.
func ValidateModelInvocationKey(raw string) error {
	for _, modelInvocable := range []bool{true, false} {
		if _, ok := WithModelInvocable(raw, modelInvocable); !ok {
			return xerrors.Errorf(
				"frontmatter key %q must be a top-level true or false: %w",
				disableModelInvocationKey,
				ErrSkillInvocationKeyInvalid,
			)
		}
	}
	return nil
}

// frontmatterSpan returns the byte range of the YAML between the frontmatter
// delimiters of raw skill Markdown. It finds the delimiters the way
// workspacesdk.ParseSkillFrontmatter does.
func frontmatterSpan(raw string) (start, end int, ok bool) {
	offset := len(raw) - len(strings.TrimPrefix(raw, "\xef\xbb\xbf"))
	// SplitAfter keeps each line's newline, so line lengths add up to byte
	// offsets in raw.
	lines := strings.SplitAfter(raw[offset:], "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return 0, 0, false
	}
	offset += len(lines[0])
	start = offset
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return start, offset, true
		}
		offset += len(line)
	}
	return 0, 0, false
}

// mappingValue returns the value node of key in a YAML mapping node, or nil
// when the mapping does not spell the key out.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if k := mapping.Content[i]; k.Kind == yaml.ScalarNode && k.Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// replaceScalar replaces the source text of a scalar node in text with value.
// It handles a plain or quoted scalar on one line without escapes. ok is
// false for any other spelling, such as an empty or tagged value.
func replaceScalar(text string, node *yaml.Node, value string) (edited string, ok bool) {
	if node.Kind != yaml.ScalarNode || node.Value == "" {
		return "", false
	}
	offset, ok := yamlOffset(text, node.Line, node.Column)
	if !ok {
		return "", false
	}
	for _, quote := range []string{"", `"`, "'"} {
		if old := quote + node.Value + quote; strings.HasPrefix(text[offset:], old) {
			return text[:offset] + value + text[offset+len(old):], true
		}
	}
	return "", false
}

// insertFirstEntry adds entry in front of the first key of the top-level
// mapping in text. In a block mapping the entry takes its own line with the
// first key's indentation. In a flow mapping, such as {name: x}, it goes
// inside the braces.
func insertFirstEntry(text string, mapping *yaml.Node, entry, newline string) (edited string, ok bool) {
	first := mapping.Content[0]
	offset, ok := yamlOffset(text, first.Line, first.Column)
	if !ok {
		return "", false
	}
	if mapping.Style&yaml.FlowStyle != 0 {
		return text[:offset] + entry + ", " + text[offset:], true
	}
	indent := text[len(strings.TrimRight(text[:offset], " ")):offset]
	return text[:offset] + entry + newline + indent + text[offset:], true
}

// yamlOffset returns the byte offset in text of a yaml.Node position: a
// 1-based line and a 1-based column counted in characters. Lines end where
// the YAML scanner ends them, which includes a lone carriage return and the
// Unicode line breaks NEL, LS, and PS, so splitting on "\n" is not enough.
func yamlOffset(text string, line, column int) (offset int, ok bool) {
	atLine, atColumn := 1, 1
	for offset < len(text) {
		if atLine == line && atColumn == column {
			return offset, true
		}
		r, size := utf8.DecodeRuneInString(text[offset:])
		if r == '\r' && strings.HasPrefix(text[offset+size:], "\n") {
			size++
		}
		switch r {
		case '\n', '\r', '\u0085', '\u2028', '\u2029':
			atLine, atColumn = atLine+1, 1
		default:
			atColumn++
		}
		offset += size
	}
	return offset, atLine == line && atColumn == column
}

// sameFieldsExcept reports whether two frontmatter texts decode to the same
// YAML mapping once key is set aside.
func sameFieldsExcept(original, edited, key string) bool {
	var want, got map[string]any
	if err := yaml.Unmarshal([]byte(original), &want); err != nil {
		return false
	}
	if err := yaml.Unmarshal([]byte(edited), &got); err != nil {
		return false
	}
	delete(want, key)
	delete(got, key)
	return reflect.DeepEqual(want, got)
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
