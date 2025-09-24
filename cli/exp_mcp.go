package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	agentapi "github.com/coder/agentapi-sdk-go"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/serpent"
)

const (
	envAppStatusSlug = "CODER_MCP_APP_STATUS_SLUG"
	envAIAgentAPIURL = "CODER_MCP_AI_AGENTAPI_URL"
)

func (r *RootCmd) mcpCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "mcp",
		Short: "Run the Coder MCP server and configure it to work with AI tools.",
		Long:  "The Coder MCP server allows you to automatically create workspaces with parameters.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.mcpConfigure(),
			r.mcpServer(),
		},
	}
	return cmd
}

func (r *RootCmd) mcpConfigure() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "configure",
		Short: "Automatically configure the MCP server.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.mcpConfigureClaudeDesktop(),
			mcpConfigureClaudeCode(),
			r.mcpConfigureCursor(),
		},
	}
	return cmd
}

func (*RootCmd) mcpConfigureClaudeDesktop() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "claude-desktop",
		Short: "Configure the Claude Desktop server.",
		Handler: func(_ *serpent.Invocation) error {
			configPath, err := os.UserConfigDir()
			if err != nil {
				return err
			}
			configPath = filepath.Join(configPath, "Claude")
			err = os.MkdirAll(configPath, 0o755)
			if err != nil {
				return err
			}
			configPath = filepath.Join(configPath, "claude_desktop_config.json")
			_, err = os.Stat(configPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
			}
			contents := map[string]any{}
			data, err := os.ReadFile(configPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
			} else {
				err = json.Unmarshal(data, &contents)
				if err != nil {
					return err
				}
			}
			binPath, err := os.Executable()
			if err != nil {
				return err
			}
			contents["mcpServers"] = map[string]any{
				"coder": map[string]any{"command": binPath, "args": []string{"exp", "mcp", "server"}},
			}
			data, err = json.MarshalIndent(contents, "", "  ")
			if err != nil {
				return err
			}
			err = os.WriteFile(configPath, data, 0o600)
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

func mcpConfigureClaudeCode() *serpent.Command {
	var (
		claudeAPIKey     string
		claudeConfigPath string
		claudeMDPath     string
		systemPrompt     string
		coderPrompt      string
		appStatusSlug    string
		testBinaryName   string
		aiAgentAPIURL    url.URL
		claudeUseBedrock string

		deprecatedCoderMCPClaudeAPIKey string
	)
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "claude-code <project-directory>",
		Short: "Configure the Claude Code server. You will need to run this command for each project you want to use. Specify the project directory as the first argument.",
		Handler: func(inv *serpent.Invocation) error {
			if len(inv.Args) == 0 {
				return xerrors.Errorf("project directory is required")
			}
			projectDirectory := inv.Args[0]
			fs := afero.NewOsFs()
			binPath, err := os.Executable()
			if err != nil {
				return xerrors.Errorf("failed to get executable path: %w", err)
			}
			if testBinaryName != "" {
				binPath = testBinaryName
			}
			configureClaudeEnv := map[string]string{}
			agentClient, err := agentAuth.CreateClient()
			if err != nil {
				cliui.Warnf(inv.Stderr, "failed to create agent client: %s", err)
			} else {
				configureClaudeEnv[envAgentURL] = agentClient.SDK.URL.String()
				configureClaudeEnv[envAgentToken] = agentClient.SDK.SessionToken()
			}

			if deprecatedCoderMCPClaudeAPIKey != "" {
				cliui.Warnf(inv.Stderr, "CODER_MCP_CLAUDE_API_KEY is deprecated, use CLAUDE_API_KEY instead")
				claudeAPIKey = deprecatedCoderMCPClaudeAPIKey
			}
			if claudeAPIKey == "" && claudeUseBedrock != "1" {
				cliui.Warnf(inv.Stderr, "CLAUDE_API_KEY is not set.")
			}

			if appStatusSlug != "" {
				configureClaudeEnv[envAppStatusSlug] = appStatusSlug
			}
			if aiAgentAPIURL.String() != "" {
				configureClaudeEnv[envAIAgentAPIURL] = aiAgentAPIURL.String()
			}
			if deprecatedSystemPromptEnv, ok := os.LookupEnv("SYSTEM_PROMPT"); ok {
				cliui.Warnf(inv.Stderr, "SYSTEM_PROMPT is deprecated, use CODER_MCP_CLAUDE_SYSTEM_PROMPT instead")
				systemPrompt = deprecatedSystemPromptEnv
			}

			if err := configureClaude(fs, ClaudeConfig{
				// TODO: will this always be stable?
				AllowedTools:     []string{`mcp__coder__coder_report_task`},
				APIKey:           claudeAPIKey,
				ConfigPath:       claudeConfigPath,
				ProjectDirectory: projectDirectory,
				MCPServers: map[string]ClaudeConfigMCP{
					"coder": {
						Command: binPath,
						Args:    []string{"exp", "mcp", "server"},
						Env:     configureClaudeEnv,
					},
				},
			}); err != nil {
				return xerrors.Errorf("failed to modify claude.json: %w", err)
			}
			cliui.Infof(inv.Stderr, "Wrote config to %s", claudeConfigPath)

			// Determine if we should include the reportTaskPrompt
			var reportTaskPrompt string
			if agentClient != nil && appStatusSlug != "" {
				// Only include the report task prompt if both the agent client and app
				// status slug are defined. Otherwise, reporting a task will fail and
				// confuse the agent (and by extension, the user).
				reportTaskPrompt = defaultReportTaskPrompt
			}

			// The Coder Prompt just allows users to extend our
			if coderPrompt != "" {
				reportTaskPrompt += "\n\n" + coderPrompt
			}

			// We also write the system prompt to the CLAUDE.md file.
			if err := injectClaudeMD(fs, reportTaskPrompt, systemPrompt, claudeMDPath); err != nil {
				return xerrors.Errorf("failed to modify CLAUDE.md: %w", err)
			}
			cliui.Infof(inv.Stderr, "Wrote CLAUDE.md to %s", claudeMDPath)
			return nil
		},
		Options: []serpent.Option{
			{
				Name:        "claude-config-path",
				Description: "The path to the Claude config file.",
				Env:         "CODER_MCP_CLAUDE_CONFIG_PATH",
				Flag:        "claude-config-path",
				Value:       serpent.StringOf(&claudeConfigPath),
				Default:     filepath.Join(os.Getenv("HOME"), ".claude.json"),
			},
			{
				Name:        "claude-md-path",
				Description: "The path to CLAUDE.md.",
				Env:         "CODER_MCP_CLAUDE_MD_PATH",
				Flag:        "claude-md-path",
				Value:       serpent.StringOf(&claudeMDPath),
				Default:     filepath.Join(os.Getenv("HOME"), ".claude", "CLAUDE.md"),
			},
			{
				Name:        "claude-api-key",
				Description: "The API key to use for the Claude Code server. This is also read from CLAUDE_API_KEY.",
				Env:         "CLAUDE_API_KEY",
				Flag:        "claude-api-key",
				Value:       serpent.StringOf(&claudeAPIKey),
			},
			{
				Name:        "mcp-claude-api-key",
				Description: "Hidden alias for CLAUDE_API_KEY. This will be removed in a future version.",
				Env:         "CODER_MCP_CLAUDE_API_KEY",
				Value:       serpent.StringOf(&deprecatedCoderMCPClaudeAPIKey),
				Hidden:      true,
			},
			{
				Name:        "system-prompt",
				Description: "The system prompt to use for the Claude Code server.",
				Env:         "CODER_MCP_CLAUDE_SYSTEM_PROMPT",
				Flag:        "claude-system-prompt",
				Value:       serpent.StringOf(&systemPrompt),
				Default:     "Send a task status update to notify the user that you are ready for input, and then wait for user input.",
			},
			{
				Name:        "coder-prompt",
				Description: "The coder prompt to use for the Claude Code server.",
				Env:         "CODER_MCP_CLAUDE_CODER_PROMPT",
				Flag:        "claude-coder-prompt",
				Value:       serpent.StringOf(&coderPrompt),
				Default:     "", // Empty default means we'll use defaultCoderPrompt from the variable
			},
			{
				Name:        "app-status-slug",
				Description: "The app status slug to use when running the Coder MCP server.",
				Env:         envAppStatusSlug,
				Flag:        "claude-app-status-slug",
				Value:       serpent.StringOf(&appStatusSlug),
			},
			{
				Flag:        "ai-agentapi-url",
				Description: "The URL of the AI AgentAPI, used to listen for status updates.",
				Env:         envAIAgentAPIURL,
				Value:       serpent.URLOf(&aiAgentAPIURL),
			},
			{
				Name:        "test-binary-name",
				Description: "Only used for testing.",
				Env:         "CODER_MCP_CLAUDE_TEST_BINARY_NAME",
				Flag:        "claude-test-binary-name",
				Value:       serpent.StringOf(&testBinaryName),
				Hidden:      true,
			},
			{
				Name:        "claude-code-use-bedrock",
				Description: "Use Amazon Bedrock.",
				Env:         "CLAUDE_CODE_USE_BEDROCK",
				Flag:        "claude-code-use-bedrock",
				Value:       serpent.StringOf(&claudeUseBedrock),
				Hidden:      true,
			},
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

func (*RootCmd) mcpConfigureCursor() *serpent.Command {
	var project bool
	cmd := &serpent.Command{
		Use:   "cursor",
		Short: "Configure Cursor to use Coder MCP.",
		Options: serpent.OptionSet{
			serpent.Option{
				Flag:        "project",
				Env:         "CODER_MCP_CURSOR_PROJECT",
				Description: "Use to configure a local project to use the Cursor MCP.",
				Value:       serpent.BoolOf(&project),
			},
		},
		Handler: func(_ *serpent.Invocation) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			if !project {
				dir, err = os.UserHomeDir()
				if err != nil {
					return err
				}
			}
			cursorDir := filepath.Join(dir, ".cursor")
			err = os.MkdirAll(cursorDir, 0o755)
			if err != nil {
				return err
			}
			mcpConfig := filepath.Join(cursorDir, "mcp.json")
			_, err = os.Stat(mcpConfig)
			contents := map[string]any{}
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
			} else {
				data, err := os.ReadFile(mcpConfig)
				if err != nil {
					return err
				}
				// The config can be empty, so we don't want to return an error if it is.
				if len(data) > 0 {
					err = json.Unmarshal(data, &contents)
					if err != nil {
						return err
					}
				}
			}
			mcpServers, ok := contents["mcpServers"].(map[string]any)
			if !ok {
				mcpServers = map[string]any{}
			}
			binPath, err := os.Executable()
			if err != nil {
				return err
			}
			mcpServers["coder"] = map[string]any{
				"command": binPath,
				"args":    []string{"exp", "mcp", "server"},
			}
			contents["mcpServers"] = mcpServers
			data, err := json.MarshalIndent(contents, "", "  ")
			if err != nil {
				return err
			}
			err = os.WriteFile(mcpConfig, data, 0o600)
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

type taskReport struct {
	// link is optional.
	link string
	// messageID must be set if this update is from a *user* message. A user
	// message only happens when interacting via the AI AgentAPI (as opposed to
	// interacting with the terminal directly).
	messageID *int64
	// selfReported must be set if the update is directly from the AI agent
	// (as opposed to the screen watcher).
	selfReported bool
	// state must always be set.
	state codersdk.WorkspaceAppStatusState
	// summary is optional.
	summary string
}

type mcpServer struct {
	agentClient      *agentsdk.Client
	appStatusSlug    string
	client           *codersdk.Client
	aiAgentAPIClient *agentapi.Client
	queue            *cliutil.Queue[taskReport]
}

func (r *RootCmd) mcpServer() *serpent.Command {
	var (
		instructions  string
		allowedTools  []string
		appStatusSlug string
		aiAgentAPIURL url.URL
	)
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use: "server",
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.TryInitClient(inv)
			if err != nil {
				return err
			}

			var lastReport taskReport
			// Create a queue that skips duplicates and preserves summaries.
			queue := cliutil.NewQueue[taskReport](512).WithPredicate(func(report taskReport) (taskReport, bool) {
				// Avoid queuing empty statuses (this would probably indicate a
				// developer error)
				if report.state == "" {
					return report, false
				}
				// If this is a user message, discard if it is not new.
				if report.messageID != nil && lastReport.messageID != nil &&
					*lastReport.messageID >= *report.messageID {
					return report, false
				}
				// If this is not a user message, and the status is "working" and not
				// self-reported (meaning it came from the screen watcher), then it
				// means one of two things:
				//
				// 1. The AI agent is not working; the user is interacting with the
				//    terminal directly.
				// 2. The AI agent is working.
				//
				// At the moment, we have no way to tell the difference between these
				// two states. In the future, if we can reliably distinguish between
				// user and AI agent activity, we can change this.
				//
				// If this is our first update, we assume it is the AI agent working and
				// accept the update.
				//
				// Otherwise we discard the update.  This risks missing cases where the
				// user manually submits a new prompt and the AI agent becomes active
				// (and does not update itself), but it avoids spamming useless status
				// updates as the user is typing, so the tradeoff is worth it.
				if report.messageID == nil &&
					report.state == codersdk.WorkspaceAppStatusStateWorking &&
					!report.selfReported && lastReport.state != "" {
					return report, false
				}
				// Keep track of the last message ID so we can tell when a message is
				// new or if it has been re-emitted.
				if report.messageID == nil {
					report.messageID = lastReport.messageID
				}
				// Preserve previous message and URI if there was no message.
				if report.summary == "" {
					report.summary = lastReport.summary
					if report.link == "" {
						report.link = lastReport.link
					}
				}
				// Avoid queueing duplicate updates.
				if report.state == lastReport.state &&
					report.link == lastReport.link &&
					report.summary == lastReport.summary {
					return report, false
				}
				lastReport = report
				return report, true
			})

			srv := &mcpServer{
				appStatusSlug: appStatusSlug,
				queue:         queue,
			}

			// Display client URL separately from authentication status.
			if client != nil && client.URL != nil {
				cliui.Infof(inv.Stderr, "URL            : %s", client.URL.String())
			} else {
				cliui.Infof(inv.Stderr, "URL            : Not configured")
			}

			// Validate the client.
			if client != nil && client.URL != nil && client.SessionToken() != "" {
				me, err := client.User(inv.Context(), codersdk.Me)
				if err == nil {
					username := me.Username
					cliui.Infof(inv.Stderr, "Authentication : Successful")
					cliui.Infof(inv.Stderr, "User           : %s", username)
					srv.client = client
				} else {
					cliui.Infof(inv.Stderr, "Authentication : Failed (%s)", err)
					cliui.Warnf(inv.Stderr, "Some tools that require authentication will not be available.")
				}
			} else {
				cliui.Infof(inv.Stderr, "Authentication : None")
			}

			// Try to create an agent client for status reporting.  Not validated.
			agentClient, err := agentAuth.CreateClient()
			if err == nil {
				cliui.Infof(inv.Stderr, "Agent URL      : %s", agentClient.SDK.URL.String())
				srv.agentClient = agentClient
			}
			if err != nil || appStatusSlug == "" {
				cliui.Infof(inv.Stderr, "Task reporter  : Disabled")
				if err != nil {
					cliui.Warnf(inv.Stderr, "%s", err)
				}
				if appStatusSlug == "" {
					cliui.Warnf(inv.Stderr, "%s must be set", envAppStatusSlug)
				}
			} else {
				cliui.Infof(inv.Stderr, "Task reporter  : Enabled")
			}

			// Try to create a client for the AI AgentAPI, which is used to get the
			// screen status to make the status reporting more robust.  No auth
			// needed, so no validation.
			if aiAgentAPIURL.String() == "" {
				cliui.Infof(inv.Stderr, "AI AgentAPI URL  : Not configured")
			} else {
				cliui.Infof(inv.Stderr, "AI AgentAPI URL  : %s", aiAgentAPIURL.String())
				aiAgentAPIClient, err := agentapi.NewClient(aiAgentAPIURL.String())
				if err != nil {
					cliui.Infof(inv.Stderr, "Screen events  : Disabled")
					cliui.Warnf(inv.Stderr, "%s must be set", envAIAgentAPIURL)
				} else {
					cliui.Infof(inv.Stderr, "Screen events  : Enabled")
					srv.aiAgentAPIClient = aiAgentAPIClient
				}
			}

			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			defer srv.queue.Close()

			cliui.Infof(inv.Stderr, "Failed to watch screen events")
			// Start the reporter, watcher, and server.  These are all tied to the
			// lifetime of the MCP server, which is itself tied to the lifetime of the
			// AI agent.
			if srv.agentClient != nil && appStatusSlug != "" {
				srv.startReporter(ctx, inv)
				if srv.aiAgentAPIClient != nil {
					srv.startWatcher(ctx, inv)
				}
			}
			return srv.startServer(ctx, inv, instructions, allowedTools)
		},
		Short: "Start the Coder MCP server.",
		Options: []serpent.Option{
			{
				Name:        "instructions",
				Description: "The instructions to pass to the MCP server.",
				Flag:        "instructions",
				Env:         "CODER_MCP_INSTRUCTIONS",
				Value:       serpent.StringOf(&instructions),
			},
			{
				Name:        "allowed-tools",
				Description: "Comma-separated list of allowed tools. If not specified, all tools are allowed.",
				Flag:        "allowed-tools",
				Env:         "CODER_MCP_ALLOWED_TOOLS",
				Value:       serpent.StringArrayOf(&allowedTools),
			},
			{
				Name:        "app-status-slug",
				Description: "When reporting a task, the coder_app slug under which to report the task.",
				Flag:        "app-status-slug",
				Env:         envAppStatusSlug,
				Value:       serpent.StringOf(&appStatusSlug),
				Default:     "",
			},
			{
				Flag:        "ai-agentapi-url",
				Description: "The URL of the AI AgentAPI, used to listen for status updates.",
				Env:         envAIAgentAPIURL,
				Value:       serpent.URLOf(&aiAgentAPIURL),
			},
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

func (s *mcpServer) startReporter(ctx context.Context, inv *serpent.Invocation) {
	go func() {
		for {
			// TODO: Even with the queue, there is still the potential that a message
			// from the screen watcher and a message from the AI agent could arrive
			// out of order if the timing is just right.  We might want to wait a bit,
			// then check if the status has changed before committing.
			item, ok := s.queue.Pop()
			if !ok {
				return
			}

			err := s.agentClient.PatchAppStatus(ctx, agentsdk.PatchAppStatus{
				AppSlug: s.appStatusSlug,
				Message: item.summary,
				URI:     item.link,
				State:   item.state,
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				cliui.Warnf(inv.Stderr, "Failed to report task status: %s", err)
			}
		}
	}()
}

func (s *mcpServer) startWatcher(ctx context.Context, inv *serpent.Invocation) {
	eventsCh, errCh, err := s.aiAgentAPIClient.SubscribeEvents(ctx)
	if err != nil {
		cliui.Warnf(inv.Stderr, "Failed to watch screen events: %s", err)
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventsCh:
				switch ev := event.(type) {
				case agentapi.EventStatusChange:
					// If the screen is stable, report idle.
					state := codersdk.WorkspaceAppStatusStateWorking
					if ev.Status == agentapi.StatusStable {
						state = codersdk.WorkspaceAppStatusStateIdle
					}
					err := s.queue.Push(taskReport{
						state: state,
					})
					if err != nil {
						cliui.Warnf(inv.Stderr, "Failed to queue update: %s", err)
						return
					}
				case agentapi.EventMessageUpdate:
					if ev.Role == agentapi.RoleUser {
						err := s.queue.Push(taskReport{
							messageID: &ev.Id,
							state:     codersdk.WorkspaceAppStatusStateWorking,
						})
						if err != nil {
							cliui.Warnf(inv.Stderr, "Failed to queue update: %s", err)
							return
						}
					}
				}
			case err := <-errCh:
				if !errors.Is(err, context.Canceled) {
					cliui.Warnf(inv.Stderr, "Received error from screen event watcher: %s", err)
				}
				return
			}
		}
	}()
}

func (s *mcpServer) startServer(ctx context.Context, inv *serpent.Invocation, instructions string, allowedTools []string) error {
	cliui.Infof(inv.Stderr, "Starting MCP server")

	cliui.Infof(inv.Stderr, "Instructions   : %q", instructions)
	if len(allowedTools) > 0 {
		cliui.Infof(inv.Stderr, "Allowed Tools  : %v", allowedTools)
	}

	// Capture the original stdin, stdout, and stderr.
	invStdin := inv.Stdin
	invStdout := inv.Stdout
	invStderr := inv.Stderr
	defer func() {
		inv.Stdin = invStdin
		inv.Stdout = invStdout
		inv.Stderr = invStderr
	}()

	mcpSrv := server.NewMCPServer(
		"Coder Agent",
		buildinfo.Version(),
		server.WithInstructions(instructions),
	)

	// If both clients are unauthorized, there are no tools we can enable.
	if s.client == nil && s.agentClient == nil {
		return xerrors.New(notLoggedInMessage)
	}

	// Add tool dependencies.
	toolOpts := []func(*toolsdk.Deps){
		toolsdk.WithTaskReporter(func(args toolsdk.ReportTaskArgs) error {
			// The agent does not reliably report its status correctly.  If AgentAPI
			// is enabled, we will always set the status to "working" when we get an
			// MCP message, and rely on the screen watcher to eventually catch the
			// idle state.
			state := codersdk.WorkspaceAppStatusStateWorking
			if s.aiAgentAPIClient == nil {
				state = codersdk.WorkspaceAppStatusState(args.State)
			}
			return s.queue.Push(taskReport{
				link:         args.Link,
				selfReported: true,
				state:        state,
				summary:      args.Summary,
			})
		}),
	}

	toolDeps, err := toolsdk.NewDeps(s.client, toolOpts...)
	if err != nil {
		return xerrors.Errorf("failed to initialize tool dependencies: %w", err)
	}

	// Register tools based on the allowlist.  Zero length means allow everything.
	for _, tool := range toolsdk.All {
		// Skip if not allowed.
		if len(allowedTools) > 0 && !slices.ContainsFunc(allowedTools, func(t string) bool {
			return t == tool.Tool.Name
		}) {
			continue
		}

		// Skip user-dependent tools if no authenticated user client.
		if !tool.UserClientOptional && s.client == nil {
			cliui.Warnf(inv.Stderr, "Tool %q requires authentication and will not be available", tool.Tool.Name)
			continue
		}

		// Skip the coder_report_task tool if there is no agent client or slug.
		if tool.Tool.Name == "coder_report_task" && (s.agentClient == nil || s.appStatusSlug == "") {
			cliui.Warnf(inv.Stderr, "Tool %q requires the task reporter and will not be available", tool.Tool.Name)
			continue
		}

		mcpSrv.AddTools(mcpFromSDK(tool, toolDeps))
	}

	srv := server.NewStdioServer(mcpSrv)
	done := make(chan error)
	go func() {
		defer close(done)
		srvErr := srv.Listen(ctx, invStdin, invStdout)
		done <- srvErr
	}()

	cliui.Infof(inv.Stderr, "Press Ctrl+C to stop the server")

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		cliui.Errorf(inv.Stderr, "Failed to start the MCP server: %s", err)
		return err
	}

	return nil
}

type ClaudeConfig struct {
	ConfigPath       string
	ProjectDirectory string
	APIKey           string
	AllowedTools     []string
	MCPServers       map[string]ClaudeConfigMCP
}

type ClaudeConfigMCP struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func configureClaude(fs afero.Fs, cfg ClaudeConfig) error {
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = filepath.Join(os.Getenv("HOME"), ".claude.json")
	}
	var config map[string]any
	_, err := fs.Stat(cfg.ConfigPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return xerrors.Errorf("failed to stat claude config: %w", err)
		}
		// Touch the file to create it if it doesn't exist.
		if err = afero.WriteFile(fs, cfg.ConfigPath, []byte(`{}`), 0o600); err != nil {
			return xerrors.Errorf("failed to touch claude config: %w", err)
		}
	}
	oldConfigBytes, err := afero.ReadFile(fs, cfg.ConfigPath)
	if err != nil {
		return xerrors.Errorf("failed to read claude config: %w", err)
	}
	err = json.Unmarshal(oldConfigBytes, &config)
	if err != nil {
		return xerrors.Errorf("failed to unmarshal claude config: %w", err)
	}

	if cfg.APIKey != "" {
		// Stops Claude from requiring the user to generate
		// a Claude-specific API key.
		config["primaryApiKey"] = cfg.APIKey
	}
	// Stops Claude from asking for onboarding.
	config["hasCompletedOnboarding"] = true
	// Stops Claude from asking for permissions.
	config["bypassPermissionsModeAccepted"] = true
	config["autoUpdaterStatus"] = "disabled"
	// Stops Claude from asking for cost threshold.
	config["hasAcknowledgedCostThreshold"] = true

	projects, ok := config["projects"].(map[string]any)
	if !ok {
		projects = make(map[string]any)
	}

	project, ok := projects[cfg.ProjectDirectory].(map[string]any)
	if !ok {
		project = make(map[string]any)
	}

	allowedTools, ok := project["allowedTools"].([]string)
	if !ok {
		allowedTools = []string{}
	}

	// Add cfg.AllowedTools to the list if they're not already present.
	for _, tool := range cfg.AllowedTools {
		for _, existingTool := range allowedTools {
			if tool == existingTool {
				continue
			}
		}
		allowedTools = append(allowedTools, tool)
	}
	project["allowedTools"] = allowedTools
	project["hasTrustDialogAccepted"] = true
	project["hasCompletedProjectOnboarding"] = true

	mcpServers, ok := project["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
	}
	for name, cfgmcp := range cfg.MCPServers {
		mcpServers[name] = cfgmcp
	}
	project["mcpServers"] = mcpServers
	// Prevents Claude from asking the user to complete the project onboarding.
	project["hasCompletedProjectOnboarding"] = true

	history, ok := project["history"].([]string)
	injectedHistoryLine := "make sure to read claude.md and report tasks properly"

	if !ok || len(history) == 0 {
		// History doesn't exist or is empty, create it with our injected line
		history = []string{injectedHistoryLine}
	} else if history[0] != injectedHistoryLine {
		// Check if our line is already the first item
		// Prepend our line to the existing history
		history = append([]string{injectedHistoryLine}, history...)
	}
	project["history"] = history

	projects[cfg.ProjectDirectory] = project
	config["projects"] = projects

	newConfigBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return xerrors.Errorf("failed to marshal claude config: %w", err)
	}
	err = afero.WriteFile(fs, cfg.ConfigPath, newConfigBytes, 0o644)
	if err != nil {
		return xerrors.Errorf("failed to write claude config: %w", err)
	}
	return nil
}

var (
	defaultReportTaskPrompt = `Respect the requirements of the "coder_report_task" tool. It is pertinent to provide a fantastic user-experience.`

	// Define the guard strings
	coderPromptStartGuard  = "<coder-prompt>"
	coderPromptEndGuard    = "</coder-prompt>"
	systemPromptStartGuard = "<system-prompt>"
	systemPromptEndGuard   = "</system-prompt>"
)

func injectClaudeMD(fs afero.Fs, coderPrompt, systemPrompt, claudeMDPath string) error {
	_, err := fs.Stat(claudeMDPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return xerrors.Errorf("failed to stat claude config: %w", err)
		}
		// Write a new file with the system prompt.
		if err = fs.MkdirAll(filepath.Dir(claudeMDPath), 0o700); err != nil {
			return xerrors.Errorf("failed to create claude config directory: %w", err)
		}

		return afero.WriteFile(fs, claudeMDPath, []byte(promptsBlock(coderPrompt, systemPrompt, "")), 0o600)
	}

	bs, err := afero.ReadFile(fs, claudeMDPath)
	if err != nil {
		return xerrors.Errorf("failed to read claude config: %w", err)
	}

	// Extract the content without the guarded sections
	cleanContent := string(bs)

	// Remove existing coder prompt section if it exists
	coderStartIdx := indexOf(cleanContent, coderPromptStartGuard)
	coderEndIdx := indexOf(cleanContent, coderPromptEndGuard)
	if coderStartIdx != -1 && coderEndIdx != -1 && coderStartIdx < coderEndIdx {
		beforeCoderPrompt := cleanContent[:coderStartIdx]
		afterCoderPrompt := cleanContent[coderEndIdx+len(coderPromptEndGuard):]
		cleanContent = beforeCoderPrompt + afterCoderPrompt
	}

	// Remove existing system prompt section if it exists
	systemStartIdx := indexOf(cleanContent, systemPromptStartGuard)
	systemEndIdx := indexOf(cleanContent, systemPromptEndGuard)
	if systemStartIdx != -1 && systemEndIdx != -1 && systemStartIdx < systemEndIdx {
		beforeSystemPrompt := cleanContent[:systemStartIdx]
		afterSystemPrompt := cleanContent[systemEndIdx+len(systemPromptEndGuard):]
		cleanContent = beforeSystemPrompt + afterSystemPrompt
	}

	// Trim any leading whitespace from the clean content
	cleanContent = strings.TrimSpace(cleanContent)

	// Create the new content with coder and system prompt prepended
	newContent := promptsBlock(coderPrompt, systemPrompt, cleanContent)

	// Write the updated content back to the file
	err = afero.WriteFile(fs, claudeMDPath, []byte(newContent), 0o600)
	if err != nil {
		return xerrors.Errorf("failed to write claude config: %w", err)
	}

	return nil
}

func promptsBlock(coderPrompt, systemPrompt, existingContent string) string {
	var newContent strings.Builder
	_, _ = newContent.WriteString(coderPromptStartGuard)
	_, _ = newContent.WriteRune('\n')
	_, _ = newContent.WriteString(coderPrompt)
	_, _ = newContent.WriteRune('\n')
	_, _ = newContent.WriteString(coderPromptEndGuard)
	_, _ = newContent.WriteRune('\n')
	_, _ = newContent.WriteString(systemPromptStartGuard)
	_, _ = newContent.WriteRune('\n')
	_, _ = newContent.WriteString(systemPrompt)
	_, _ = newContent.WriteRune('\n')
	_, _ = newContent.WriteString(systemPromptEndGuard)
	_, _ = newContent.WriteRune('\n')
	if existingContent != "" {
		_, _ = newContent.WriteString(existingContent)
	}
	return newContent.String()
}

// indexOf returns the index of the first instance of substr in s,
// or -1 if substr is not present in s.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// mcpFromSDK adapts a toolsdk.Tool to go-mcp's server.ServerTool.
// It assumes that the tool responds with a valid JSON object.
func mcpFromSDK(sdkTool toolsdk.GenericTool, tb toolsdk.Deps) server.ServerTool {
	// NOTE: some clients will silently refuse to use tools if there is an issue
	// with the tool's schema or configuration.
	if sdkTool.Schema.Properties == nil {
		panic("developer error: schema properties cannot be nil")
	}
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        sdkTool.Tool.Name,
			Description: sdkTool.Description,
			InputSchema: mcp.ToolInputSchema{
				Type:       "object", // Default of mcp.NewTool()
				Properties: sdkTool.Schema.Properties,
				Required:   sdkTool.Schema.Required,
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(request.Params.Arguments); err != nil {
				return nil, xerrors.Errorf("failed to encode request arguments: %w", err)
			}
			result, err := sdkTool.Handler(ctx, tb, buf.Bytes())
			if err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(string(result)),
				},
			}, nil
		},
	}
}
