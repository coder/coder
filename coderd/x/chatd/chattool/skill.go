package chattool

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	maxSkillMetaBytes = workspacesdk.MaxSkillMetaBytes
	maxSkillFileBytes = 512 * 1024

	// AvailableSkillsOpenTag is the XML start tag for the skill index block.
	AvailableSkillsOpenTag = "<available-skills>"
	// AvailableSkillsCloseTag is the XML end tag for the skill index block.
	AvailableSkillsCloseTag = "</available-skills>"
)

// SkillMeta is the frontmatter from a skill meta file discovered in a
// workspace. It carries just enough information to list the skill
// in the prompt index without reading the full body.
type SkillMeta struct {
	Name        string
	Description string
	// Dir is the absolute path to the skill directory inside
	// the workspace filesystem.
	Dir string
	// MetaFile is the basename of the skill meta file (e.g.
	// "SKILL.md"). When empty, DefaultSkillMetaFile is used.
	MetaFile string
	// Meta is the verbatim skill meta file (SKILL.md) content the
	// agent pushed in the workspace context snapshot: front-matter
	// plus body. When present, read_skill serves the body from it
	// instead of reading the file back over the workspace
	// connection, so a pinned chat keeps returning the same
	// instructions even when the workspace is unreachable. It is
	// empty on the legacy per-turn discovery path, where the body is
	// read live.
	Meta []byte
}

// SkillContent is the full body of a skill, loaded on demand
// when the model calls read_skill.
type SkillContent struct {
	SkillMeta
	// Body is the markdown content after the frontmatter
	// delimiters have been stripped.
	Body string
	// Files lists relative paths of supporting files in the
	// skill directory (everything except the skill meta file).
	Files []string
}

// FormatResolvedSkillIndex renders an XML block listing all source-aware
// skills. Aliases are the names the model should pass to the skill tools.
func FormatResolvedSkillIndex(resolved []skillspkg.ResolvedSkill) string {
	if len(resolved) == 0 {
		return ""
	}

	entries := make([]skillIndexEntry, 0, len(resolved))
	hasQualifiedAlias := false
	hasWorkspaceSkill := false
	for _, s := range resolved {
		entries = append(entries, skillIndexEntry{
			Alias:       s.Alias,
			Description: s.Description,
		})
		if s.Source == skillspkg.SourceWorkspace {
			hasWorkspaceSkill = true
		}
		if s.Alias == skillspkg.QualifiedAlias(s.Source, s.Name) {
			hasQualifiedAlias = true
		}
	}
	return renderSkillIndex(entries, skillIndexFormatOptions{
		includeQualifiedAliasInstruction: hasQualifiedAlias,
		includeReadSkillFileInstruction:  hasWorkspaceSkill,
	})
}

type skillIndexEntry struct {
	Alias       string
	Description string
}

type skillIndexFormatOptions struct {
	includeQualifiedAliasInstruction bool
	includeReadSkillFileInstruction  bool
}

func renderSkillIndex(entries []skillIndexEntry, opts skillIndexFormatOptions) string {
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	_, _ = b.WriteString(AvailableSkillsOpenTag + "\n")
	_, _ = b.WriteString(
		"Use read_skill to load a skill's full instructions " +
			"before following them.\n",
	)
	if opts.includeReadSkillFileInstruction {
		_, _ = b.WriteString(
			"Use read_skill_file to read supporting files " +
				"referenced by a workspace skill.\n",
		)
	}
	if opts.includeQualifiedAliasInstruction {
		_, _ = b.WriteString(
			"When a skill is listed as personal/name or workspace/name, " +
				"pass that qualified alias to read_skill.\n",
		)
	}
	_, _ = b.WriteString("\n")
	for _, s := range entries {
		_, _ = b.WriteString("- ")
		_, _ = b.WriteString(s.Alias)
		if s.Description != "" {
			_, _ = b.WriteString(": ")
			_, _ = b.WriteString(s.Description)
		}
		_, _ = b.WriteString("\n")
	}
	_, _ = b.WriteString(AvailableSkillsCloseTag)
	return b.String()
}

// listSkillFiles lists the supporting files in a skill directory,
// excluding the skill meta file itself. Directory entries are
// suffixed with "/" so the model can tell them apart from files.
func listSkillFiles(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	dir, metaFile string,
) ([]string, error) {
	lsResp, err := conn.LS(ctx, "", workspacesdk.LSRequest{
		Path:       []string{dir},
		Relativity: workspacesdk.LSRelativityRoot,
	})
	if err != nil {
		return nil, xerrors.Errorf("list skill directory: %w", err)
	}

	var files []string
	for _, entry := range lsResp.Contents {
		if entry.Name == metaFile {
			continue
		}
		name := entry.Name
		if entry.IsDir {
			name += "/"
		}
		files = append(files, name)
	}
	return files, nil
}

// loadPinnedWorkspaceSkillContent builds skill content from the
// SKILL.md the agent pushed in the workspace context snapshot. The
// body is parsed from the pinned bytes without dialing the workspace,
// so it keeps working when the workspace is unreachable. The
// supporting-file list is a best-effort live lookup, because the
// snapshot carries only the meta file (per the agent push contract):
// a missing connection or LS failure yields an empty file list rather
// than failing the read.
func loadPinnedWorkspaceSkillContent(
	ctx context.Context,
	options ReadSkillOptions,
	skill SkillMeta,
) (SkillContent, error) {
	raw := skill.Meta
	if int64(len(raw)) > maxSkillMetaBytes {
		raw = raw[:maxSkillMetaBytes]
	}

	_, _, body, err := workspacesdk.ParseSkillFrontmatter(string(raw))
	if err != nil {
		return SkillContent{}, xerrors.Errorf(
			"parse skill frontmatter: %w", err,
		)
	}

	return SkillContent{
		SkillMeta: skill,
		Body:      body,
		Files:     bestEffortSkillFiles(ctx, options, skill),
	}, nil
}

// bestEffortSkillFiles lists a pinned skill's supporting files over the
// workspace connection, returning nil when the connection or listing
// fails so an unreachable workspace never blocks read_skill from
// returning the pinned body.
func bestEffortSkillFiles(
	ctx context.Context,
	options ReadSkillOptions,
	skill SkillMeta,
) []string {
	if options.GetWorkspaceConn == nil {
		return nil
	}
	conn, err := options.GetWorkspaceConn(ctx)
	if err != nil {
		return nil
	}
	files, err := listSkillFiles(ctx, conn, skill.Dir, cmp.Or(skill.MetaFile, DefaultSkillMetaFile))
	if err != nil {
		return nil
	}
	return files
}

// LoadSkillFile reads a supporting file from a skill's directory.
// The relativePath is validated to prevent directory traversal and
// access to hidden files.
func LoadSkillFile(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	skill SkillMeta,
	relativePath string,
) (string, error) {
	if err := validateSkillFilePath(relativePath); err != nil {
		return "", err
	}

	fullPath := path.Join(skill.Dir, relativePath)

	reader, _, err := conn.ReadFile(
		ctx, fullPath, 0, maxSkillFileBytes+1,
	)
	if err != nil {
		return "", xerrors.Errorf(
			"read skill file: %w", err,
		)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSkillFileBytes+1))
	reader.Close()
	if err != nil {
		return "", xerrors.Errorf(
			"read skill file bytes: %w", err,
		)
	}

	if int64(len(raw)) > maxSkillFileBytes {
		raw = raw[:maxSkillFileBytes]
	}

	return string(raw), nil
}

// validateSkillFilePath rejects paths that could escape the skill
// directory or access hidden files. Only forward-relative,
// non-hidden paths are allowed.
func validateSkillFilePath(p string) error {
	if p == "" {
		return xerrors.New("path is required")
	}
	if strings.HasPrefix(p, "/") {
		return xerrors.New(
			"absolute paths are not allowed",
		)
	}
	for _, component := range strings.Split(p, "/") {
		if component == ".." {
			return xerrors.New(
				"path traversal is not allowed",
			)
		}
		if strings.HasPrefix(component, ".") {
			return xerrors.New(
				"hidden file components are not allowed",
			)
		}
	}
	return nil
}

// DefaultSkillMetaFile is the fallback skill meta file name used
// when loading skill bodies on demand from older agents.
const DefaultSkillMetaFile = "SKILL.md"

// ReadSkillOptions configures the read_skill and read_skill_file
// tools.
type ReadSkillOptions struct {
	GetWorkspaceConn      func(context.Context) (workspacesdk.AgentConn, error)
	GetSkills             func() []SkillMeta
	ResolveAlias          func(string) (skillspkg.ResolvedSkill, error)
	LoadPersonalSkillBody func(context.Context, string) (skillspkg.ParsedSkill, error)
}

// ReadSkillArgs are the parameters accepted by read_skill.
type ReadSkillArgs struct {
	Name string `json:"name" description:"The name or qualified alias of the skill to read."`
}

// ReadSkill returns an AgentTool that reads the full instructions
// for a skill by name. The model should call this before
// following any skill's instructions.
func ReadSkill(options ReadSkillOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_skill",
		"Read the full instructions for a skill by name. "+
			"Returns the skill meta file body and a list of "+
			"supporting files. Use read_skill before "+
			"following a skill's instructions.",
		func(ctx context.Context, args ReadSkillArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if args.Name == "" {
				return fantasy.NewTextErrorResponse(
					"name is required",
				), nil
			}

			resolved, err := resolveSkillAlias(options, args.Name)
			if err != nil {
				return skillResolveErrorResponse(args.Name, err), nil
			}

			switch resolved.Source {
			case skillspkg.SourcePersonal:
				if options.LoadPersonalSkillBody == nil {
					return fantasy.NewTextErrorResponse(
						"personal skill loader is not configured",
					), nil
				}
				content, err := options.LoadPersonalSkillBody(ctx, resolved.Name)
				if err != nil {
					if xerrors.Is(err, skillspkg.ErrSkillNotFound) {
						return skillNotFoundResponse(args.Name), nil
					}
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("failed to load personal skill %q", args.Name),
					), nil
				}
				return toolResponse(map[string]any{
					"name":  args.Name,
					"body":  content.Body,
					"files": []string{},
				}), nil
			case skillspkg.SourceWorkspace:
				content, response, ok := readWorkspaceSkillBody(ctx, options, args.Name, resolved.Name)
				if ok {
					return response, nil
				}
				// Include the absolute skill directory so the agent can
				// reach supporting files with read_file and execute.
				return toolResponse(map[string]any{
					"name":  args.Name,
					"dir":   content.Dir,
					"body":  content.Body,
					"files": nonNilFiles(content.Files),
				}), nil
			default:
				return skillNotFoundResponse(args.Name), nil
			}
		},
	)
}

// ReadSkillFileArgs are the parameters accepted by
// read_skill_file.
type ReadSkillFileArgs struct {
	Name string `json:"name" description:"The name or qualified alias of the skill to read."`
	Path string `json:"path" description:"Relative path to a file in the skill directory (e.g. roles/security-reviewer.md)."`
}

// ReadSkillFile returns an AgentTool that reads a supporting file
// from a skill's directory.
func ReadSkillFile(options ReadSkillOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_skill_file",
		"Read a supporting file from a skill's directory "+
			"(e.g. roles/security-reviewer.md).",
		func(ctx context.Context, args ReadSkillFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if args.Name == "" {
				return fantasy.NewTextErrorResponse(
					"name is required",
				), nil
			}
			if args.Path == "" {
				return fantasy.NewTextErrorResponse(
					"path is required",
				), nil
			}

			resolved, err := resolveSkillAlias(options, args.Name)
			if err != nil {
				return skillResolveErrorResponse(args.Name, err), nil
			}
			if resolved.Source == skillspkg.SourcePersonal {
				return fantasy.NewTextErrorResponse(
					"read_skill_file is not supported for personal skills (no supporting files)",
				), nil
			}
			if resolved.Source != skillspkg.SourceWorkspace {
				return skillNotFoundResponse(args.Name), nil
			}

			skill, ok := findSkill(options.GetSkills, resolved.Name)
			if !ok {
				return skillNotFoundResponse(args.Name), nil
			}

			// Validate the path early so we reject bad
			// inputs before dialing the workspace agent.
			if err := validateSkillFilePath(args.Path); err != nil {
				return fantasy.NewTextErrorResponse(
					err.Error(),
				), nil
			}

			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse(
					"workspace connection resolver is not configured",
				), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					err.Error(),
				), nil
			}

			content, err := LoadSkillFile(
				ctx, conn, skill, args.Path,
			)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					err.Error(),
				), nil
			}

			return toolResponse(map[string]any{
				"content": content,
			}), nil
		},
	)
}

func resolveSkillAlias(options ReadSkillOptions, name string) (skillspkg.ResolvedSkill, error) {
	if options.ResolveAlias != nil {
		return options.ResolveAlias(name)
	}

	skill, ok := findSkill(options.GetSkills, name)
	if !ok {
		return skillspkg.ResolvedSkill{}, skillspkg.ErrSkillNotFound
	}
	return skillspkg.ResolvedSkill{
		Skill: skillspkg.Skill{
			Name:        skill.Name,
			Description: skill.Description,
			Source:      skillspkg.SourceWorkspace,
		},
		Alias: skill.Name,
	}, nil
}

func readWorkspaceSkillBody(
	ctx context.Context,
	options ReadSkillOptions,
	requestedName string,
	canonicalName string,
) (SkillContent, fantasy.ToolResponse, bool) {
	skill, ok := findSkill(options.GetSkills, canonicalName)
	if !ok {
		return SkillContent{}, skillNotFoundResponse(requestedName), true
	}

	// The SKILL.md body travels in the workspace context snapshot, so it
	// is served from the pin without dialing the workspace. The supporting
	// file list is still a best-effort live lookup; see
	// loadPinnedWorkspaceSkillContent.
	content, err := loadPinnedWorkspaceSkillContent(ctx, options, skill)
	if err != nil {
		return SkillContent{}, fantasy.NewTextErrorResponse(err.Error()), true
	}
	return content, fantasy.ToolResponse{}, false
}

func skillResolveErrorResponse(name string, err error) fantasy.ToolResponse {
	if xerrors.Is(err, skillspkg.ErrSkillNotFound) {
		return skillNotFoundResponse(name)
	}
	if xerrors.Is(err, skillspkg.ErrSkillAmbiguous) {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextErrorResponse(
		fmt.Sprintf("failed to resolve skill %q", name),
	)
}

func skillNotFoundResponse(name string) fantasy.ToolResponse {
	return fantasy.NewTextErrorResponse(
		fmt.Sprintf("skill %q not found", name),
	)
}

func nonNilFiles(files []string) []string {
	if files == nil {
		return []string{}
	}
	return files
}

// findSkill looks up a skill by name in the current skill list.
func findSkill(
	getSkills func() []SkillMeta,
	name string,
) (SkillMeta, bool) {
	if getSkills == nil {
		return SkillMeta{}, false
	}
	for _, s := range getSkills() {
		if s.Name == name {
			return s, true
		}
	}
	return SkillMeta{}, false
}
