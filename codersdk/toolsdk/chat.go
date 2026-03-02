package toolsdk

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Chat tool name constants.
const (
	ToolNameChatListTemplates   = "chat_list_templates"
	ToolNameChatReadTemplate    = "chat_read_template"
	ToolNameChatCreateWorkspace = "chat_create_workspace"
	ToolNameChatReadFile        = "chat_read_file"
	ToolNameChatWriteFile       = "chat_write_file"
	ToolNameChatEditFiles       = "chat_edit_files"
	ToolNameChatExecute         = "chat_execute"
	ToolNameChatProcessOutput   = "chat_process_output"
	ToolNameChatProcessList     = "chat_process_list"
	ToolNameChatProcessSignal   = "chat_process_signal"
)

// Chat tool configuration constants.
const (
	// chatListTemplatesPageSize is the number of templates
	// returned per page by the ChatListTemplates tool.
	chatListTemplatesPageSize = 10

	// chatDefaultTimeout is the default timeout for foreground
	// command execution.
	chatDefaultTimeout = 10 * time.Second

	// chatPollInterval is how often we check for process
	// completion in foreground mode.
	chatPollInterval = 200 * time.Millisecond

	// chatMaxOutputRunes is the maximum number of runes kept
	// from command output. We keep the last 32KB worth of runes.
	chatMaxOutputRunes = 32 * 1024
)

// chatNonInteractiveEnvVars are set on every process to prevent
// interactive prompts that would hang a headless execution.
var chatNonInteractiveEnvVars = map[string]string{
	"GIT_EDITOR":          "true",
	"TERM":                "dumb",
	"NO_COLOR":            "1",
	"PAGER":               "cat",
	"GIT_TERMINAL_PROMPT": "0",
	"CODER_CHAT_AGENT":    "true",
}

// --- ChatListTemplates ---

// ChatListTemplatesArgs are the parameters accepted by the
// ChatListTemplates tool.
type ChatListTemplatesArgs struct {
	Query string `json:"query,omitempty"`
	Page  int    `json:"page,omitempty"`
}

// ChatListTemplatesResult is the response from the
// ChatListTemplates tool.
type ChatListTemplatesResult struct {
	Templates  []MinimalTemplate `json:"templates"`
	Count      int               `json:"count"`
	Page       int               `json:"page"`
	TotalPages int               `json:"total_pages"`
	TotalCount int               `json:"total_count"`
}

// ChatListTemplates lists available workspace templates with
// pagination and optional filtering. Results are sorted by active
// user count (popularity) in descending order.
var ChatListTemplates = Tool[ChatListTemplatesArgs, ChatListTemplatesResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatListTemplates,
		Description: "List available workspace templates. Optionally filter by a " +
			"search query matching template name or description. " +
			"Use this to find a template before creating a workspace. " +
			"Results are ordered by number of active developers (most popular first). " +
			"Returns 10 per page. Use the page parameter to paginate through results.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Optional search query to filter templates by name or description.",
				},
				"page": map[string]any{
					"type":        "integer",
					"description": "Page number for pagination (default: 1).",
				},
			},
			Required: []string{},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatListTemplatesArgs) (ChatListTemplatesResult, error) {
		templates, err := deps.coderClient.Templates(ctx, codersdk.TemplateFilter{
			SearchQuery: args.Query,
		})
		if err != nil {
			return ChatListTemplatesResult{}, err
		}

		// Sort by active user count descending.
		sort.SliceStable(templates, func(i, j int) bool {
			return templates[i].ActiveUserCount > templates[j].ActiveUserCount
		})

		// Paginate.
		page := args.Page
		if page < 1 {
			page = 1
		}
		totalCount := len(templates)
		totalPages := (totalCount + chatListTemplatesPageSize - 1) / chatListTemplatesPageSize
		if totalPages == 0 {
			totalPages = 1
		}
		start := (page - 1) * chatListTemplatesPageSize
		end := start + chatListTemplatesPageSize
		if start > totalCount {
			start = totalCount
		}
		if end > totalCount {
			end = totalCount
		}
		pageTemplates := templates[start:end]

		minimal := make([]MinimalTemplate, len(pageTemplates))
		for i, t := range pageTemplates {
			minimal[i] = MinimalTemplate{
				ID:              t.ID.String(),
				Name:            t.Name,
				DisplayName:     t.DisplayName,
				Description:     t.Description,
				ActiveVersionID: t.ActiveVersionID,
				ActiveUserCount: t.ActiveUserCount,
			}
		}

		return ChatListTemplatesResult{
			Templates:  minimal,
			Count:      len(minimal),
			Page:       page,
			TotalPages: totalPages,
			TotalCount: totalCount,
		}, nil
	},
}

// --- ChatReadTemplate ---

// ChatReadTemplateArgs are the parameters accepted by the
// ChatReadTemplate tool.
type ChatReadTemplateArgs struct {
	TemplateID string `json:"template_id"`
}

// ChatReadTemplateResult is the response from the
// ChatReadTemplate tool.
type ChatReadTemplateResult struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	DisplayName     string                    `json:"display_name"`
	Description     string                    `json:"description"`
	ActiveVersionID string                    `json:"active_version_id"`
	Parameters      []ChatTemplateVersionParam `json:"parameters"`
}

// ChatTemplateVersionParam is a minimal representation of a
// template version parameter for the chat UI.
type ChatTemplateVersionParam struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name,omitempty"`
	Description  string `json:"description,omitempty"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value,omitempty"`
	Required     bool   `json:"required"`
}

// ChatReadTemplate returns details about a workspace template
// including its configurable rich parameters.
var ChatReadTemplate = Tool[ChatReadTemplateArgs, ChatReadTemplateResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatReadTemplate,
		Description: "Get details about a workspace template, including its " +
			"configurable parameters. Use this after finding a " +
			"template with list_templates and before creating a " +
			"workspace with create_workspace.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"template_id": map[string]any{
					"type":        "string",
					"description": "The ID of the template to read.",
				},
			},
			Required: []string{"template_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatReadTemplateArgs) (ChatReadTemplateResult, error) {
		templateID, err := uuid.Parse(args.TemplateID)
		if err != nil {
			return ChatReadTemplateResult{}, xerrors.Errorf("invalid template_id: %w", err)
		}

		tmpl, err := deps.coderClient.Template(ctx, templateID)
		if err != nil {
			return ChatReadTemplateResult{}, xerrors.Errorf("template not found: %w", err)
		}

		params, err := deps.coderClient.TemplateVersionRichParameters(ctx, tmpl.ActiveVersionID)
		if err != nil {
			return ChatReadTemplateResult{}, xerrors.Errorf("failed to get template parameters: %w", err)
		}

		chatParams := make([]ChatTemplateVersionParam, len(params))
		for i, p := range params {
			chatParams[i] = ChatTemplateVersionParam{
				Name:         p.Name,
				DisplayName:  p.DisplayName,
				Description:  p.Description,
				Type:         p.Type,
				DefaultValue: p.DefaultValue,
				Required:     p.Required,
			}
		}

		return ChatReadTemplateResult{
			ID:              tmpl.ID.String(),
			Name:            tmpl.Name,
			DisplayName:     tmpl.DisplayName,
			Description:     tmpl.Description,
			ActiveVersionID: tmpl.ActiveVersionID.String(),
			Parameters:      chatParams,
		}, nil
	},
}

// --- ChatCreateWorkspace ---

// ChatCreateWorkspaceArgs are the parameters accepted by the
// ChatCreateWorkspace tool.
type ChatCreateWorkspaceArgs struct {
	TemplateID string            `json:"template_id"`
	Name       string            `json:"name,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// ChatCreateWorkspaceResult is the response from the
// ChatCreateWorkspace tool.
type ChatCreateWorkspaceResult struct {
	Created       bool   `json:"created"`
	WorkspaceName string `json:"workspace_name"`
}

// ChatCreateWorkspace creates a new workspace from a template.
var ChatCreateWorkspace = Tool[ChatCreateWorkspaceArgs, ChatCreateWorkspaceResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatCreateWorkspace,
		Description: "Create a new workspace from a template. Requires a " +
			"template_id (from list_templates). Optionally provide " +
			"a name and parameter values (from read_template). " +
			"If no name is given, one will be generated.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"template_id": map[string]any{
					"type":        "string",
					"description": "The ID of the template to create the workspace from.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Optional name for the workspace. If not provided, one will be generated.",
				},
				"parameters": map[string]any{
					"type":        "object",
					"description": "Optional key/value pairs of template parameters.",
				},
			},
			Required: []string{"template_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatCreateWorkspaceArgs) (ChatCreateWorkspaceResult, error) {
		templateID, err := uuid.Parse(args.TemplateID)
		if err != nil {
			return ChatCreateWorkspaceResult{}, xerrors.Errorf("invalid template_id: %w", err)
		}

		// Fetch the template to get its active version.
		tmpl, err := deps.coderClient.Template(ctx, templateID)
		if err != nil {
			return ChatCreateWorkspaceResult{}, xerrors.Errorf("get template: %w", err)
		}

		name := strings.TrimSpace(args.Name)
		if name == "" {
			name = chatGenerateWorkspaceName(tmpl.Name)
		}

		var buildParams []codersdk.WorkspaceBuildParameter
		for k, v := range args.Parameters {
			buildParams = append(buildParams, codersdk.WorkspaceBuildParameter{
				Name:  k,
				Value: v,
			})
		}

		workspace, err := deps.coderClient.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
			TemplateVersionID:   tmpl.ActiveVersionID,
			Name:                name,
			RichParameterValues: buildParams,
		})
		if err != nil {
			return ChatCreateWorkspaceResult{}, err
		}

		return ChatCreateWorkspaceResult{
			Created:       true,
			WorkspaceName: workspace.Name,
		}, nil
	},
}

// chatGenerateWorkspaceName produces a unique workspace name
// seeded from the template name.
func chatGenerateWorkspaceName(seed string) string {
	base := codersdk.UsernameFrom(strings.TrimSpace(strings.ToLower(seed)))
	if base == "" {
		base = "workspace"
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	if len(base) > 27 {
		base = strings.Trim(base[:27], "-")
	}
	if base == "" {
		base = "workspace"
	}

	name := fmt.Sprintf("%s-%s", base, suffix)
	if err := codersdk.NameValid(name); err == nil {
		return name
	}
	return codersdk.UsernameFrom(name)
}

// --- ChatReadFile ---

// ChatReadFileArgs are the parameters accepted by the
// ChatReadFile tool.
type ChatReadFileArgs struct {
	Path   string `json:"path"`
	Offset *int64 `json:"offset,omitempty"`
	Limit  *int64 `json:"limit,omitempty"`
}

// ChatReadFileResult is the response from the ChatReadFile tool.
type ChatReadFileResult struct {
	Content    string `json:"content"`
	FileSize   int64  `json:"file_size"`
	TotalLines int64  `json:"total_lines"`
	LinesRead  int64  `json:"lines_read"`
}

// ChatReadFile reads a file from the workspace via the agent
// connection with line-based offset and limit support.
var ChatReadFile = Tool[ChatReadFileArgs, ChatReadFileResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatReadFile,
		Description: "Read a file from the workspace. Returns line-numbered content. " +
			"The offset parameter is a 1-based line number (default: 1). " +
			"The limit parameter is the number of lines to return (default: 2000). " +
			"For large files, use offset and limit to paginate.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to read.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "1-based line number to start reading from (default: 1).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Number of lines to return (default: 2000).",
				},
			},
			Required: []string{"path"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatReadFileArgs) (ChatReadFileResult, error) {
		conn := deps.AgentConn()
		if conn == nil {
			return ChatReadFileResult{}, xerrors.New("agent connection not available")
		}

		offset := int64(1) // 1-based default.
		limit := int64(0)  // 0 means server default (2000 lines).
		if args.Offset != nil {
			offset = *args.Offset
		}
		if args.Limit != nil {
			limit = *args.Limit
		}

		resp, err := conn.ReadFileLines(ctx, args.Path, offset, limit, workspacesdk.DefaultReadFileLinesLimits())
		if err != nil {
			return ChatReadFileResult{}, xerrors.Errorf("read file: %w", err)
		}
		if !resp.Success {
			return ChatReadFileResult{}, xerrors.New(resp.Error)
		}

		return ChatReadFileResult{
			Content:    resp.Content,
			FileSize:   resp.FileSize,
			TotalLines: int64(resp.TotalLines),
			LinesRead:  int64(resp.LinesRead),
		}, nil
	},
}

// --- ChatWriteFile ---

// ChatWriteFileArgs are the parameters accepted by the
// ChatWriteFile tool.
type ChatWriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ChatWriteFileResult is the response from the ChatWriteFile
// tool.
type ChatWriteFileResult struct {
	OK bool `json:"ok"`
}

// ChatWriteFile writes a file to the workspace via the agent
// connection.
var ChatWriteFile = Tool[ChatWriteFileArgs, ChatWriteFileResult]{
	Tool: aisdk.Tool{
		Name:        ToolNameChatWriteFile,
		Description: "Write a file to the workspace.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute path to the file to write.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file.",
				},
			},
			Required: []string{"path", "content"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatWriteFileArgs) (ChatWriteFileResult, error) {
		conn := deps.AgentConn()
		if conn == nil {
			return ChatWriteFileResult{}, xerrors.New("agent connection not available")
		}

		if err := conn.WriteFile(ctx, args.Path, strings.NewReader(args.Content)); err != nil {
			return ChatWriteFileResult{}, xerrors.Errorf("write file: %w", err)
		}

		return ChatWriteFileResult{OK: true}, nil
	},
}

// --- ChatEditFiles ---

// ChatEditFilesArgs are the parameters accepted by the
// ChatEditFiles tool.
type ChatEditFilesArgs struct {
	Files []workspacesdk.FileEdits `json:"files"`
}

// ChatEditFilesResult is the response from the ChatEditFiles
// tool.
type ChatEditFilesResult struct {
	OK bool `json:"ok"`
}

// ChatEditFiles performs search-and-replace edits on one or more
// files in the workspace via the agent connection.
var ChatEditFiles = Tool[ChatEditFilesArgs, ChatEditFilesResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatEditFiles,
		Description: "Perform search-and-replace edits on one or more files in the workspace." +
			" Each file can have multiple edits applied atomically.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"files": map[string]any{
					"type":        "array",
					"description": "List of files with their search-and-replace edits.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{
								"type":        "string",
								"description": "Path to the file to edit.",
							},
							"edits": map[string]any{
								"type":        "array",
								"description": "List of search/replace pairs.",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"search": map[string]any{
											"type":        "string",
											"description": "Text to search for.",
										},
										"replace": map[string]any{
											"type":        "string",
											"description": "Text to replace with.",
										},
									},
									"required": []string{"search", "replace"},
								},
							},
						},
						"required": []string{"path", "edits"},
					},
				},
			},
			Required: []string{"files"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatEditFilesArgs) (ChatEditFilesResult, error) {
		if len(args.Files) == 0 {
			return ChatEditFilesResult{}, xerrors.New("files is required")
		}

		conn := deps.AgentConn()
		if conn == nil {
			return ChatEditFilesResult{}, xerrors.New("agent connection not available")
		}

		if err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files}); err != nil {
			return ChatEditFilesResult{}, xerrors.Errorf("edit files: %w", err)
		}

		return ChatEditFilesResult{OK: true}, nil
	},
}

// --- ChatExecute ---

// ChatExecuteArgs are the parameters accepted by the ChatExecute
// tool.
type ChatExecuteArgs struct {
	Command         string  `json:"command"`
	Timeout         *string `json:"timeout,omitempty"`
	WorkDir         *string `json:"workdir,omitempty"`
	RunInBackground *bool   `json:"run_in_background,omitempty"`
}

// ChatExecuteResult is the structured response from the
// ChatExecute tool. It is also reused by the ChatProcessOutput
// tool.
type ChatExecuteResult struct {
	Success             bool   `json:"success"`
	Output              string `json:"output,omitempty"`
	ExitCode            int    `json:"exit_code"`
	WallDurationMs      int64  `json:"wall_duration_ms"`
	Error               string `json:"error,omitempty"`
	BackgroundProcessID string `json:"background_process_id,omitempty"`
}

// ChatExecute runs a shell command in the workspace via the agent
// HTTP API. It supports foreground (blocking) and background
// execution modes.
var ChatExecute = Tool[ChatExecuteArgs, ChatExecuteResult]{
	Tool: aisdk.Tool{
		Name:        ToolNameChatExecute,
		Description: "Execute a shell command in the workspace.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to execute.",
				},
				"timeout": map[string]any{
					"type":        "string",
					"description": "Optional timeout as a Go duration string (e.g. \"10s\", \"5m\"). Default: 10s.",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Optional working directory for the command.",
				},
				"run_in_background": map[string]any{
					"type":        "boolean",
					"description": "If true, run the command in the background and return a process ID.",
				},
			},
			Required: []string{"command"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatExecuteArgs) (ChatExecuteResult, error) {
		conn := deps.AgentConn()
		if conn == nil {
			return ChatExecuteResult{}, xerrors.New("agent connection not available")
		}

		// Build the environment map.
		env := make(map[string]string, len(chatNonInteractiveEnvVars))
		for k, v := range chatNonInteractiveEnvVars {
			env[k] = v
		}

		background := args.RunInBackground != nil && *args.RunInBackground

		var workDir string
		if args.WorkDir != nil {
			workDir = *args.WorkDir
		}

		if background {
			return chatExecuteBackground(ctx, conn, args.Command, workDir, env)
		}
		return chatExecuteForeground(ctx, conn, args, workDir, env)
	},
}

// chatExecuteBackground starts a process in the background and
// returns immediately with the process ID.
func chatExecuteBackground(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	command string,
	workDir string,
	env map[string]string,
) (ChatExecuteResult, error) {
	resp, err := conn.StartProcess(ctx, workspacesdk.StartProcessRequest{
		Command:    command,
		WorkDir:    workDir,
		Env:        env,
		Background: true,
	})
	if err != nil {
		return ChatExecuteResult{}, xerrors.Errorf("start background process: %w", err)
	}

	return ChatExecuteResult{
		Success:             true,
		BackgroundProcessID: resp.ID,
	}, nil
}

// chatExecuteForeground starts a process and polls for its
// completion, enforcing the configured timeout.
func chatExecuteForeground(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ChatExecuteArgs,
	workDir string,
	env map[string]string,
) (ChatExecuteResult, error) {
	timeout := chatDefaultTimeout
	if args.Timeout != nil {
		parsed, err := time.ParseDuration(*args.Timeout)
		if err != nil {
			return ChatExecuteResult{}, xerrors.Errorf("invalid timeout %q: %w", *args.Timeout, err)
		}
		timeout = parsed
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	resp, err := conn.StartProcess(cmdCtx, workspacesdk.StartProcessRequest{
		Command:    args.Command,
		WorkDir:    workDir,
		Env:        env,
		Background: false,
	})
	if err != nil {
		return ChatExecuteResult{}, xerrors.Errorf("start process: %w", err)
	}

	result := chatPollProcess(cmdCtx, conn, resp.ID, timeout)
	result.WallDurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// chatPollProcess polls for process output until the process
// exits or the context times out.
func chatPollProcess(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	processID string,
	timeout time.Duration,
) ChatExecuteResult {
	ticker := time.NewTicker(chatPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Timeout — get whatever output we have. Use a
			// fresh context since cmdCtx is already canceled.
			bgCtx, bgCancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			outputResp, _ := conn.ProcessOutput(bgCtx, processID)
			bgCancel()
			output := chatTruncateOutput(outputResp.Output)
			return ChatExecuteResult{
				Success:  false,
				Output:   output,
				ExitCode: -1,
				Error:    fmt.Sprintf("command timed out after %s", timeout),
			}
		case <-ticker.C:
			outputResp, err := conn.ProcessOutput(ctx, processID)
			if err != nil {
				return ChatExecuteResult{
					Success: false,
					Error:   fmt.Sprintf("get process output: %v", err),
				}
			}
			if !outputResp.Running {
				exitCode := 0
				if outputResp.ExitCode != nil {
					exitCode = *outputResp.ExitCode
				}
				output := chatTruncateOutput(outputResp.Output)
				return ChatExecuteResult{
					Success:  exitCode == 0,
					Output:   output,
					ExitCode: exitCode,
				}
			}
		}
	}
}

// chatTruncateOutput keeps the last chatMaxOutputRunes runes of
// the output string.
func chatTruncateOutput(output string) string {
	runes := []rune(output)
	if len(runes) > chatMaxOutputRunes {
		runes = runes[len(runes)-chatMaxOutputRunes:]
	}
	return string(runes)
}

// --- ChatProcessOutput ---

// ChatProcessOutputArgs are the parameters accepted by the
// ChatProcessOutput tool.
type ChatProcessOutputArgs struct {
	ProcessID string `json:"process_id"`
}

// ChatProcessOutput retrieves output from a background process
// by its ID.
var ChatProcessOutput = Tool[ChatProcessOutputArgs, ChatExecuteResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatProcessOutput,
		Description: "Retrieve output from a background process. " +
			"Use the process_id returned by execute with " +
			"run_in_background=true. Returns the current output, " +
			"whether the process is still running, and the exit " +
			"code if it has finished.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"process_id": map[string]any{
					"type":        "string",
					"description": "The ID of the background process.",
				},
			},
			Required: []string{"process_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatProcessOutputArgs) (ChatExecuteResult, error) {
		conn := deps.AgentConn()
		if conn == nil {
			return ChatExecuteResult{}, xerrors.New("agent connection not available")
		}

		resp, err := conn.ProcessOutput(ctx, args.ProcessID)
		if err != nil {
			return ChatExecuteResult{}, xerrors.Errorf("get process output: %w", err)
		}

		output := chatTruncateOutput(resp.Output)
		exitCode := 0
		if resp.ExitCode != nil {
			exitCode = *resp.ExitCode
		}

		result := ChatExecuteResult{
			Output:   output,
			ExitCode: exitCode,
		}

		if resp.Running {
			result.Success = true
		} else {
			result.Success = exitCode == 0
		}

		return result, nil
	},
}

// --- ChatProcessList ---

// ChatProcessListArgs is the (empty) parameter struct for the
// ChatProcessList tool.
type ChatProcessListArgs struct{}

// ChatProcessList lists all tracked processes on the workspace
// agent.
var ChatProcessList = Tool[ChatProcessListArgs, workspacesdk.ListProcessesResponse]{
	Tool: aisdk.Tool{
		Name: ToolNameChatProcessList,
		Description: "List all tracked processes in the workspace. " +
			"Returns process IDs, commands, status (running or " +
			"exited), and exit codes. Use this to discover " +
			"background processes or check which processes are " +
			"still running.",
		Schema: aisdk.Schema{
			Properties: map[string]any{},
			Required:   []string{},
		},
	},
	Handler: func(ctx context.Context, deps Deps, _ ChatProcessListArgs) (workspacesdk.ListProcessesResponse, error) {
		conn := deps.AgentConn()
		if conn == nil {
			return workspacesdk.ListProcessesResponse{}, xerrors.New("agent connection not available")
		}

		resp, err := conn.ListProcesses(ctx)
		if err != nil {
			return workspacesdk.ListProcessesResponse{}, xerrors.Errorf("list processes: %w", err)
		}

		return resp, nil
	},
}

// --- ChatProcessSignal ---

// ChatProcessSignalArgs are the parameters accepted by the
// ChatProcessSignal tool.
type ChatProcessSignalArgs struct {
	ProcessID string `json:"process_id"`
	Signal    string `json:"signal"`
}

// ChatProcessSignalResult is the response from the
// ChatProcessSignal tool.
type ChatProcessSignalResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ChatProcessSignal sends a signal to a tracked process on the
// workspace agent.
var ChatProcessSignal = Tool[ChatProcessSignalArgs, ChatProcessSignalResult]{
	Tool: aisdk.Tool{
		Name: ToolNameChatProcessSignal,
		Description: "Send a signal to a background process. " +
			"Use \"terminate\" (SIGTERM) for graceful shutdown " +
			"or \"kill\" (SIGKILL) to force stop. Use the " +
			"process_id returned by execute with " +
			"run_in_background=true or from process_list.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"process_id": map[string]any{
					"type":        "string",
					"description": "The ID of the process to signal.",
				},
				"signal": map[string]any{
					"type":        "string",
					"description": `The signal to send: "terminate" (SIGTERM) or "kill" (SIGKILL).`,
					"enum":        []string{"terminate", "kill"},
				},
			},
			Required: []string{"process_id", "signal"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ChatProcessSignalArgs) (ChatProcessSignalResult, error) {
		if args.Signal != "terminate" && args.Signal != "kill" {
			return ChatProcessSignalResult{}, xerrors.Errorf("invalid signal %q: must be \"terminate\" or \"kill\"", args.Signal)
		}

		conn := deps.AgentConn()
		if conn == nil {
			return ChatProcessSignalResult{}, xerrors.New("agent connection not available")
		}

		if err := conn.SignalProcess(ctx, args.ProcessID, args.Signal); err != nil {
			return ChatProcessSignalResult{}, xerrors.Errorf("signal process: %w", err)
		}

		return ChatProcessSignalResult{
			Success: true,
			Message: fmt.Sprintf("signal %q sent to process %s", args.Signal, args.ProcessID),
		}, nil
	},
}

// ChatTools is the list of all chat-specific tools.
var ChatTools = []GenericTool{
	ChatListTemplates.Generic(),
	ChatReadTemplate.Generic(),
	ChatCreateWorkspace.Generic(),
	ChatReadFile.Generic(),
	ChatWriteFile.Generic(),
	ChatEditFiles.Generic(),
	ChatExecute.Generic(),
	ChatProcessOutput.Generic(),
	ChatProcessList.Generic(),
	ChatProcessSignal.Generic(),
}
