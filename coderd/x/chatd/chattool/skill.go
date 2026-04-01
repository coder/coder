package chattool

import (
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	maxSkillMetaBytes = 64 * 1024
	maxSkillFileBytes = 512 * 1024
)

// skillNamePattern validates kebab-case skill names. Each segment
// must start with a lowercase letter or digit, and segments are
// separated by single hyphens.
var skillNamePattern = regexp.MustCompile(
	`^[a-z0-9]+(-[a-z0-9]+)*$`,
)

// markdownCommentRe strips HTML comments from skill bodies so
// they don't leak into the prompt. Matches the same pattern
// used by instruction.go in the parent package.
var markdownCommentRe = regexp.MustCompile(`<!--[\s\S]*?-->`)

// SkillMeta is the frontmatter from a skill meta file discovered in a
// workspace. It carries just enough information to list the skill
// in the prompt index without reading the full body.
type SkillMeta struct {
	Name        string
	Description string
	// Dir is the absolute path to the skill directory inside
	// the workspace filesystem.
	Dir string
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

// DiscoverSkills walks the given skills directories and returns
// metadata for every valid skill it finds. Missing directories
// or individual read errors are silently skipped so that a
// partially broken skills tree never blocks the conversation.
// Skill names must be unique across directories; first
// occurrence wins.
func DiscoverSkills(
	ctx context.Context,
	logger slog.Logger,
	conn workspacesdk.AgentConn,
	skillsDirs []string,
	metaFile string,
) ([]SkillMeta, error) {
	seen := make(map[string]struct{})
	var skills []SkillMeta

	for _, skillsDirPath := range skillsDirs {
		lsResp, err := conn.LS(ctx, "", workspacesdk.LSRequest{
			Path:       []string{skillsDirPath},
			Relativity: workspacesdk.LSRelativityRoot,
		})
		if err != nil {
			// The skills directory is entirely optional.
			// Skip on any error.
			continue
		}

		for _, entry := range lsResp.Contents {
			if !entry.IsDir {
				continue
			}

			metaPath := path.Join(
				entry.AbsolutePathString, metaFile,
			)
			reader, _, err := conn.ReadFile(
				ctx, metaPath, 0, maxSkillMetaBytes+1,
			)
			if err != nil {
				// The directory may have been removed between
				// the LS and this read, or it simply lacks the
				// meta file. Any error is non-fatal.
				continue
			}
			raw, err := io.ReadAll(io.LimitReader(reader, maxSkillMetaBytes+1))
			reader.Close()
			if err != nil {
				logger.Debug(ctx, "failed to read skill meta file",
					slog.F("path", metaPath), slog.Error(err))
				continue
			}

			// Silently truncate oversized metadata files so
			// a single large file cannot exhaust memory.
			if int64(len(raw)) > maxSkillMetaBytes {
				raw = raw[:maxSkillMetaBytes]
			}

			name, description, _, err := parseSkillFrontmatter(
				string(raw),
			)
			if err != nil {
				logger.Debug(ctx, "failed to parse skill frontmatter",
					slog.F("path", metaPath), slog.Error(err))
				continue
			}

			// The directory name must match the declared name
			// so skill references are unambiguous.
			if name != entry.Name {
				logger.Debug(ctx, "skill name does not match directory",
					slog.F("dir", entry.Name), slog.F("declared_name", name))
				continue
			}
			if !skillNamePattern.MatchString(name) {
				logger.Debug(ctx, "skill name does not match pattern",
					slog.F("name", name))
				continue
			}

			// First occurrence wins across directories.
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}

			skills = append(skills, SkillMeta{
				Name:        name,
				Description: description,
				Dir:         entry.AbsolutePathString,
			})
		}
	}

	return skills, nil
}

// parseSkillFrontmatter extracts name, description, and the
// markdown body from a skill meta file. The frontmatter uses a
// simple `key: value` format between `---` delimiters, and no
// full YAML parser is needed.
func parseSkillFrontmatter(
	content string,
) (name, description, body string, err error) {
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
		// Strip surrounding quotes from YAML string values.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		switch strings.ToLower(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}

	if name == "" {
		return "", "", "", xerrors.New(
			"frontmatter missing required 'name' field",
		)
	}

	// Everything after the closing delimiter is the body.
	body = strings.Join(lines[closingIdx+1:], "\n")
	body = markdownCommentRe.ReplaceAllString(body, "")
	body = strings.TrimSpace(body)

	return name, description, body, nil
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

// LoadSkillBody reads the full skill meta file for a discovered skill
// and lists the supporting files in its directory. The caller
// should have already obtained the SkillMeta from DiscoverSkills.
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

	_, _, body, err := parseSkillFrontmatter(string(raw))
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

// ReadSkillOptions configures the read_skill and read_skill_file
// tools.
type ReadSkillOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	GetSkills        func() []SkillMeta
	SkillMetaFile    string
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

			content, err := LoadSkillBody(ctx, conn, skill, options.SkillMetaFile)
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
