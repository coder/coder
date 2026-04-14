package chattool

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type EditFilesOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	ResolvePlanPath  func(context.Context) (chatPath string, home string, err error)
	IsPlanTurn       bool
}

// editFileEntry is the LLM-facing per-file argument for the ed-script
// tool. It is intentionally separate from the wire type so the tool
// schema exposes only ed_script, not the legacy search/replace fields.
type editFileEntry struct {
	Path     string `json:"path"`
	EdScript string `json:"ed_script"`
}

type EditFilesArgs struct {
	Files []editFileEntry `json:"files"`
}

const editFilesDescription = `Edit one or more files in the workspace using ed commands.
Each file entry contains a path and an ed script.

ed quick reference:
  [line]a    append text after line (end input with "." alone on a line)
  [line]i    insert text before line (end input with "." alone on a line)
  [from,to]c change lines (end input with "." alone on a line)
  [from,to]d delete lines
  [line]s/old/new/   substitute first match on line
  [line]s/old/new/g  substitute all matches on line
  /regex/    address by regex match instead of line number
  1,$        address range: all lines

Do NOT include w (write) or q (quit) commands; they are added
automatically. Lines are 1-indexed. A bare "." on its own line
ends input mode for a/i/c commands, so you cannot insert a line
containing only ".".

Example - delete lines 10-20:
  {"files": [{"path": "/abs/path", "ed_script": "10,20d"}]}
Example - insert after line 5:
  {"files": [{"path": "/abs/path", "ed_script": "5a\nnew line\n."}]}
Example - substitute all occurrences:
  {"files": [{"path": "/abs/path", "ed_script": "1,$s/oldName/newName/g"}]}`

func EditFiles(options EditFilesOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"edit_files",
		editFilesDescription,
		func(ctx context.Context, args EditFilesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var planPath string
			if options.IsPlanTurn && len(args.Files) > 0 {
				resolvedPlanPath, err := resolvePlanTurnPath(ctx, options.ResolvePlanPath)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				for i := range args.Files {
					args.Files[i].Path = strings.TrimSpace(args.Files[i].Path)
					if args.Files[i].Path != resolvedPlanPath {
						return fantasy.NewTextErrorResponse("during plan turns, edit_files is restricted to " + resolvedPlanPath), nil
					}
				}
				planPath = resolvedPlanPath
			}
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if planPath != "" {
				if err := ensurePlanPathResolvesToItself(ctx, conn, planPath); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
			}
			return executeEditFilesTool(ctx, conn, args, options.ResolvePlanPath)
		},
	)
}

func executeEditFilesTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args EditFilesArgs,
	resolvePlanPath func(context.Context) (chatPath string, home string, err error),
) (fantasy.ToolResponse, error) {
	if len(args.Files) == 0 {
		return fantasy.NewTextErrorResponse("files is required"), nil
	}

	// Validate files and build the wire request.
	var (
		chatPath       string
		home           string
		planPathErr    error
		planPathLoaded bool
	)
	wireFiles := make([]workspacesdk.FileEdits, 0, len(args.Files))
	for _, file := range args.Files {
		file.Path = strings.TrimSpace(file.Path)
		if file.Path == "" {
			return fantasy.NewTextErrorResponse("each file must have a non-empty path"), nil
		}
		if file.EdScript == "" {
			return fantasy.NewTextErrorResponse(
				fmt.Sprintf("%s: ed_script is required", file.Path),
			), nil
		}

		hasPlanFileName := looksLikePlanFileName(file.Path)
		if hasPlanFileName && !isAbsolutePath(file.Path) {
			return fantasy.NewTextErrorResponse(
				"plan files must use absolute paths; use the chat-specific absolute plan path; no files in this batch were applied",
			), nil
		}
		if resolvePlanPath != nil && hasPlanFileName {
			if !planPathLoaded {
				chatPath, home, planPathErr = resolvePlanPath(ctx)
				planPathLoaded = true
			}
			if resp, rejected := rejectSharedPlanPath(file.Path, home, chatPath, planPathErr); rejected {
				return fantasy.NewTextErrorResponse(
					resp.Content + "; no files in this batch were applied",
				), nil
			}
		}

		wireFiles = append(wireFiles, workspacesdk.FileEdits{
			Path:     file.Path,
			EdScript: file.EdScript,
		})
	}

	resp, err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: wireFiles})
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	// Return raw unified diffs, one per file. The tool call
	// args already carry the file paths.
	var result strings.Builder
	for _, d := range resp.Diffs {
		if d.Diff == "" {
			continue
		}
		if result.Len() > 0 {
			_, _ = result.WriteString("\n")
		}
		_, _ = result.WriteString(d.Diff)
	}
	if result.Len() == 0 {
		return fantasy.NewTextResponse("Files edited successfully (no diff)."), nil
	}
	return fantasy.NewTextResponse(result.String()), nil
}
