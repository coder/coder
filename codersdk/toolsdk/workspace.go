package toolsdk

import (
	"context"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// The workspace file and process tools share their arguments, behavior,
// and results with the Coder Agents tools (see workspacetools). The MCP
// versions add a workspace argument because an MCP session is not bound
// to a single workspace, and they report invalid arguments and agent API
// failures as MCP tool errors. A parity test in coderd/mcp keeps the
// schemas aligned.

// workspaceToolNames are the MCP names of the workspace tools, used in
// shared descriptions and follow-up hints.
var workspaceToolNames = workspacetools.ToolNames{
	Execute:       ToolNameWorkspaceExecute,
	ProcessOutput: ToolNameWorkspaceProcessOutput,
	ProcessList:   ToolNameWorkspaceProcessList,
	ReadFile:      ToolNameWorkspaceReadFile,
}

// maxWorkspaceToolWait bounds how long an MCP call blocks on a command.
// Each MCP request must return before the client gives up, otherwise the
// background_process_id needed to follow up is lost. It matches the
// agent's maximum blocking wait for process output.
const maxWorkspaceToolWait = 5 * time.Minute

func workspaceProperty() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": workspaceAgentDescription,
	}
}

// workspaceToolError converts an agent API error to the tool error
// returned to the model, dropping transport metadata.
func workspaceToolError(err error) error {
	return xerrors.New(workspacetools.AgentAPIErrorMessage(err))
}

// WorkspaceReadFileArgs are the arguments of WorkspaceReadFile.
type WorkspaceReadFileArgs struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Offset    *int64 `json:"offset,omitempty"`
	Limit     *int64 `json:"limit,omitempty"`
}

// WorkspaceReadFile reads line-numbered content from a workspace file.
var WorkspaceReadFile = Tool[WorkspaceReadFileArgs, workspacetools.ReadFileResult]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceReadFile,
		Description: workspacetools.ReadFileDescription,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": workspaceProperty(),
				"path":      map[string]any{"type": "string"},
				"offset":    map[string]any{"type": "integer"},
				"limit":     map[string]any{"type": "integer"},
			},
			Required: []string{"workspace", "path"},
		},
	},
	MCPAnnotations:     mcpReadOnlyAnnotations,
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceReadFileArgs) (workspacetools.ReadFileResult, error) {
		conn, err := openAgentConn(ctx, deps, args.Workspace)
		if err != nil {
			return workspacetools.ReadFileResult{}, err
		}
		defer conn.Close()
		res, err := workspacetools.ReadFile(ctx, conn, args.Path, args.Offset, args.Limit)
		if err != nil {
			return workspacetools.ReadFileResult{}, workspaceToolError(err)
		}
		return res, nil
	},
}

// WorkspaceWriteFileArgs are the arguments of WorkspaceWriteFile.
type WorkspaceWriteFileArgs struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

// WorkspaceWriteFile creates or replaces a workspace file.
var WorkspaceWriteFile = Tool[WorkspaceWriteFileArgs, workspacetools.OKResult]{
	Tool: aisdk.Tool{
		Name: ToolNameWorkspaceWriteFile,
		Description: "Create a file in the workspace or overwrite an existing one with the given content. " +
			"Use " + ToolNameWorkspaceEditFiles + " for targeted changes to an existing file.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": workspaceProperty(),
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path of the file to write.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": workspacetools.WriteFileContentDescription,
				},
			},
			Required: []string{"workspace", "path", "content"},
		},
	},
	MCPAnnotations:     mcpDestructiveAnnotations,
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceWriteFileArgs) (workspacetools.OKResult, error) {
		path := strings.TrimSpace(args.Path)
		if path == "" {
			return workspacetools.OKResult{}, xerrors.New("path is required")
		}
		conn, err := openAgentConn(ctx, deps, args.Workspace)
		if err != nil {
			return workspacetools.OKResult{}, err
		}
		defer conn.Close()
		if err := conn.WriteFile(ctx, path, strings.NewReader(args.Content)); err != nil {
			return workspacetools.OKResult{}, workspaceToolError(err)
		}
		return workspacetools.OKResult{OK: true}, nil
	},
}

// WorkspaceEditFilesArgs are the arguments of WorkspaceEditFiles.
type WorkspaceEditFilesArgs struct {
	Workspace string                   `json:"workspace"`
	Files     []workspacesdk.FileEdits `json:"files"`
}

// WorkspaceEditFiles applies text replacements to workspace files.
var WorkspaceEditFiles = Tool[WorkspaceEditFilesArgs, workspacetools.EditFilesResult]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceEditFiles,
		Description: workspacetools.EditFilesDescription,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": workspaceProperty(),
				"files": map[string]any{
					"type":        "array",
					"description": workspacetools.EditFilesFilesDescription,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{
								"type":        "string",
								"description": "The absolute path of the file to edit, for example /home/coder/project/main.go.",
							},
							"edits": map[string]any{
								"type":        "array",
								"description": "Edits that replace old text with new text, applied to this file in order.",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"old_text": map[string]any{
											"type":        "string",
											"description": "Existing text in the file to replace. Matching is fuzzy: whitespace and indentation differences are tolerated.",
										},
										"new_text": map[string]any{
											"type":        "string",
											"description": "Text that replaces old_text.",
										},
										"replace_all": map[string]any{
											"type":        "boolean",
											"description": "Replace every match of old_text instead of erroring when it matches more than once.",
										},
									},
									"required": []string{"old_text", "new_text"},
								},
							},
						},
						"required": []string{"path", "edits"},
					},
				},
			},
			Required: []string{"workspace", "files"},
		},
	},
	MCPAnnotations:     mcpDestructiveAnnotations,
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceEditFilesArgs) (workspacetools.EditFilesResult, error) {
		if err := workspacetools.ValidateFileEdits(args.Files); err != nil {
			return workspacetools.EditFilesResult{}, err
		}
		conn, err := openAgentConn(ctx, deps, args.Workspace)
		if err != nil {
			return workspacetools.EditFilesResult{}, err
		}
		defer conn.Close()
		res, err := workspacetools.EditFiles(ctx, conn, args.Files)
		if err != nil {
			return workspacetools.EditFilesResult{}, workspaceToolError(err)
		}
		return res, nil
	},
}

// WorkspaceExecuteArgs are the arguments of WorkspaceExecute.
type WorkspaceExecuteArgs struct {
	Workspace       string  `json:"workspace"`
	Command         string  `json:"command"`
	Timeout         *string `json:"timeout,omitempty"`
	WorkDir         *string `json:"workdir,omitempty"`
	RunInBackground *bool   `json:"run_in_background,omitempty"`
}

// WorkspaceExecute runs a shell command in a workspace.
var WorkspaceExecute = Tool[WorkspaceExecuteArgs, workspacetools.ExecuteResult]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceExecute,
		Description: workspacetools.ExecuteDescription(workspaceToolNames),
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": workspaceProperty(),
				"command": map[string]any{
					"type":        "string",
					"description": workspacetools.ExecuteCommandDescription,
				},
				"timeout": map[string]any{
					"type":        "string",
					"description": workspacetools.ExecuteTimeoutDescription,
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": workspacetools.ExecuteWorkDirDescription,
				},
				"run_in_background": map[string]any{
					"type":        "boolean",
					"description": workspacetools.ExecuteRunInBackgroundDescription(workspaceToolNames),
				},
			},
			Required: []string{"workspace", "command"},
		},
	},
	MCPAnnotations: mcpDestructiveAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceExecuteArgs) (workspacetools.ExecuteResult, error) {
		if args.Command == "" {
			return workspacetools.ExecuteResult{}, xerrors.New("command is required")
		}
		conn, err := openAgentConn(ctx, deps, args.Workspace)
		if err != nil {
			return workspacetools.ExecuteResult{}, err
		}
		defer conn.Close()
		req := workspacetools.ExecuteRequest{
			Command:         args.Command,
			Timeout:         args.Timeout,
			RunInBackground: args.RunInBackground != nil && *args.RunInBackground,
			MaxTimeout:      maxWorkspaceToolWait,
			Names:           workspaceToolNames,
		}
		if args.WorkDir != nil {
			req.WorkDir = *args.WorkDir
		}
		return workspacetools.Execute(ctx, conn, req)
	},
}

// WorkspaceProcessOutputArgs are the arguments of WorkspaceProcessOutput.
type WorkspaceProcessOutputArgs struct {
	Workspace   string  `json:"workspace"`
	ProcessID   string  `json:"process_id"`
	WaitTimeout *string `json:"wait_timeout,omitempty"`
}

// WorkspaceProcessOutput retrieves the output of a workspace process.
var WorkspaceProcessOutput = Tool[WorkspaceProcessOutputArgs, workspacetools.ExecuteResult]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceProcessOutput,
		Description: workspacetools.ProcessOutputDescription(workspaceToolNames),
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace":  workspaceProperty(),
				"process_id": map[string]any{"type": "string"},
				"wait_timeout": map[string]any{
					"type":        "string",
					"description": workspacetools.ProcessOutputWaitTimeoutDescription,
				},
			},
			Required: []string{"workspace", "process_id"},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceProcessOutputArgs) (workspacetools.ExecuteResult, error) {
		if args.ProcessID == "" {
			return workspacetools.ExecuteResult{}, xerrors.New("process_id is required")
		}
		conn, err := openAgentConn(ctx, deps, args.Workspace)
		if err != nil {
			return workspacetools.ExecuteResult{}, err
		}
		defer conn.Close()
		return workspacetools.ProcessOutput(ctx, conn, workspacetools.ProcessOutputRequest{
			ProcessID:   args.ProcessID,
			WaitTimeout: args.WaitTimeout,
			MaxWait:     maxWorkspaceToolWait,
		})
	},
}

// WorkspaceProcessListArgs are the arguments of WorkspaceProcessList.
type WorkspaceProcessListArgs struct {
	Workspace string `json:"workspace"`
}

// WorkspaceProcessList lists processes started outside a chat in a
// workspace.
var WorkspaceProcessList = Tool[WorkspaceProcessListArgs, workspacesdk.ListProcessesResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceProcessList,
		Description: workspacetools.ProcessListDescription,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": workspaceProperty(),
			},
			Required: []string{"workspace"},
		},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceProcessListArgs) (workspacesdk.ListProcessesResponse, error) {
		conn, err := openAgentConn(ctx, deps, args.Workspace)
		if err != nil {
			return workspacesdk.ListProcessesResponse{}, err
		}
		defer conn.Close()
		resp, err := conn.ListProcesses(ctx)
		if err != nil {
			return workspacesdk.ListProcessesResponse{}, xerrors.Errorf("list processes: %s", workspacetools.AgentAPIErrorMessage(err))
		}
		return resp, nil
	},
}

// WorkspaceProcessSignalArgs are the arguments of WorkspaceProcessSignal.
type WorkspaceProcessSignalArgs struct {
	Workspace string `json:"workspace"`
	ProcessID string `json:"process_id"`
	Signal    string `json:"signal"`
}

// WorkspaceProcessSignal terminates or kills a workspace process.
var WorkspaceProcessSignal = Tool[WorkspaceProcessSignalArgs, workspacetools.SignalResult]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceProcessSignal,
		Description: workspacetools.ProcessSignalDescription(workspaceToolNames),
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace":  workspaceProperty(),
				"process_id": map[string]any{"type": "string"},
				"signal":     map[string]any{"type": "string"},
			},
			Required: []string{"workspace", "process_id", "signal"},
		},
	},
	MCPAnnotations: mcpDestructiveAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceProcessSignalArgs) (workspacetools.SignalResult, error) {
		if args.ProcessID == "" {
			return workspacetools.SignalResult{}, xerrors.New("process_id is required")
		}
		if err := workspacetools.ValidateSignal(args.Signal); err != nil {
			return workspacetools.SignalResult{}, err
		}
		conn, err := openAgentConn(ctx, deps, args.Workspace)
		if err != nil {
			return workspacetools.SignalResult{}, err
		}
		defer conn.Close()
		if err := conn.SignalProcess(ctx, args.ProcessID, args.Signal); err != nil {
			return workspacetools.SignalResult{}, xerrors.Errorf("signal process: %s", workspacetools.AgentAPIErrorMessage(err))
		}
		return workspacetools.SignalSent(args.ProcessID, args.Signal), nil
	},
}
