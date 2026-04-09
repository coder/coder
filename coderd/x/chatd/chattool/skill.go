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

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	maxSkillMetaBytes = 64 * 1024
	maxSkillFileBytes = 512 * 1024
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

// FormatSkillIndex renders an XML block listing all discovered
// skills. This block is injected into the system prompt so the
// model knows which skills are available and how to load them.
func FormatSkillIndex(skills []SkillMeta) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	_, _ = b.WriteString("<available-skills>\n")
	_, _ = b.WriteString(
		"Use read_skill to load a skill's full instructions " +
			"before following them.\n" +
			"Use read_skill_file to read supporting files " +
			"referenced by a skill.\n\n",
	)
	for _, s := range skills {
		_, _ = b.WriteString("- ")
		_, _ = b.WriteString(s.Name)
		if s.Description != "" {
			_, _ = b.WriteString(": ")
			_, _ = b.WriteString(s.Description)
		}
		_, _ = b.WriteString("\n")
	}
	_, _ = b.WriteString("</available-skills>")
	return b.String()
}

// LoadSkillBody reads the full skill meta file for a discovered
// skill and lists the supporting files in its directory.
func LoadSkillBody(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	skill SkillMeta,
	metaFile string,
) (SkillContent, error) {
	metaPath := path.Join(skill.Dir, metaFile)

	reader, _, err := conn.ReadFile(
		ctx, metaPath, 0, maxSkillMetaBytes+1,
	)
	if err != nil {
		return SkillContent{}, xerrors.Errorf(
			"read skill body: %w", err,
		)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSkillMetaBytes+1))
	reader.Close()
	if err != nil {
		return SkillContent{}, xerrors.Errorf(
			"read skill body bytes: %w", err,
		)
	}

	if int64(len(raw)) > maxSkillMetaBytes {
		raw = raw[:maxSkillMetaBytes]
	}

	_, _, body, err := workspacesdk.ParseSkillFrontmatter(string(raw))
	if err != nil {
		return SkillContent{}, xerrors.Errorf(
			"parse skill frontmatter: %w", err,
		)
	}

	// List supporting files so the model knows what it can
	// request via read_skill_file.
	lsResp, err := conn.LS(ctx, "", workspacesdk.LSRequest{
		Path:       []string{skill.Dir},
		Relativity: workspacesdk.LSRelativityRoot,
	})
	if err != nil {
		return SkillContent{}, xerrors.Errorf(
			"list skill directory: %w", err,
		)
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

	return SkillContent{
		SkillMeta: skill,
		Body:      body,
		Files:     files,
	}, nil
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
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	GetSkills        func() []SkillMeta
}

// ReadSkillArgs are the parameters accepted by read_skill.
type ReadSkillArgs struct {
	Name string `json:"name" description:"The kebab-case name of the skill to read."`
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
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse(
					"workspace connection resolver is not configured",
				), nil
			}
			if args.Name == "" {
				return fantasy.NewTextErrorResponse(
					"name is required",
				), nil
			}

			skill, ok := findSkill(options.GetSkills, args.Name)
			if !ok {
				return fantasy.NewTextErrorResponse(
					fmt.Sprintf("skill %q not found", args.Name),
				), nil
			}

			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(
					err.Error(),
				), nil
			}

			// Load the skill body from the workspace agent,
			// respecting a custom meta file name if set.
			content, err := LoadSkillBody(ctx, conn, skill, cmp.Or(skill.MetaFile, DefaultSkillMetaFile))
			if err != nil {
				return fantasy.NewTextErrorResponse(
					err.Error(),
				), nil
			}
			return toolResponse(map[string]any{
				"name":  content.Name,
				"body":  content.Body,
				"files": content.Files,
			}), nil
		},
	)
}

// ReadSkillFileArgs are the parameters accepted by
// read_skill_file.
type ReadSkillFileArgs struct {
	Name string `json:"name" description:"The kebab-case name of the skill."`
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
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse(
					"workspace connection resolver is not configured",
				), nil
			}
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

			skill, ok := findSkill(options.GetSkills, args.Name)
			if !ok {
				return fantasy.NewTextErrorResponse(
					fmt.Sprintf("skill %q not found", args.Name),
				), nil
			}

			// Validate the path early so we reject bad
			// inputs before dialing the workspace agent.
			if err := validateSkillFilePath(args.Path); err != nil {
				return fantasy.NewTextErrorResponse(
					err.Error(),
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
