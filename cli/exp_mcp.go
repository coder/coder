package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/serpent"
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
			r.mcpConfigureClaudeCode(),
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

func (*RootCmd) mcpConfigureClaudeCode() *serpent.Command {
	var (
		claudeAPIKey     string
		claudeConfigPath string
		claudeMDPath     string
		systemPrompt     string
		appStatusSlug    string
		testBinaryName   string

		deprecatedCoderMCPClaudeAPIKey string
	)
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
			agentToken, err := getAgentToken(fs)
			if err != nil {
				cliui.Warnf(inv.Stderr, "failed to get agent token: %s", err)
			} else {
				configureClaudeEnv["CODER_AGENT_TOKEN"] = agentToken
			}
			if claudeAPIKey == "" {
				if deprecatedCoderMCPClaudeAPIKey == "" {
					cliui.Warnf(inv.Stderr, "CLAUDE_API_KEY is not set.")
				} else {
					cliui.Warnf(inv.Stderr, "CODER_MCP_CLAUDE_API_KEY is deprecated, use CLAUDE_API_KEY instead")
					claudeAPIKey = deprecatedCoderMCPClaudeAPIKey
				}
			}
			if appStatusSlug != "" {
				configureClaudeEnv["CODER_MCP_APP_STATUS_SLUG"] = appStatusSlug
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

			// We also write the system prompt to the CLAUDE.md file.
			if err := injectClaudeMD(fs, systemPrompt, claudeMDPath); err != nil {
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
				Name:        "app-status-slug",
				Description: "The app status slug to use when running the Coder MCP server.",
				Env:         "CODER_MCP_CLAUDE_APP_STATUS_SLUG",
				Flag:        "claude-app-status-slug",
				Value:       serpent.StringOf(&appStatusSlug),
			},
			{
				Name:        "test-binary-name",
				Description: "Only used for testing.",
				Env:         "CODER_MCP_CLAUDE_TEST_BINARY_NAME",
				Flag:        "claude-test-binary-name",
				Value:       serpent.StringOf(&testBinaryName),
				Hidden:      true,
			},
		},
	}
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

func (r *RootCmd) mcpServer() *serpent.Command {
	var (
		client        = new(codersdk.Client)
		instructions  string
		allowedTools  []string
		appStatusSlug string
	)
	return &serpent.Command{
		Use: "server",
		Handler: func(inv *serpent.Invocation) error {
			return mcpServerHandler(inv, client, instructions, allowedTools, appStatusSlug)
		},
		Short: "Start the Coder MCP server.",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
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
				Env:         "CODER_MCP_APP_STATUS_SLUG",
				Value:       serpent.StringOf(&appStatusSlug),
				Default:     "",
			},
		},
	}
}

func mcpServerHandler(inv *serpent.Invocation, client *codersdk.Client, instructions string, allowedTools []string, appStatusSlug string) error {
	ctx, cancel := context.WithCancel(inv.Context())
	defer cancel()

	fs := afero.NewOsFs()

	me, err := client.User(ctx, codersdk.Me)
	if err != nil {
		cliui.Errorf(inv.Stderr, "Failed to log in to the Coder deployment.")
		cliui.Errorf(inv.Stderr, "Please check your URL and credentials.")
		cliui.Errorf(inv.Stderr, "Tip: Run `coder whoami` to check your credentials.")
		return err
	}
	cliui.Infof(inv.Stderr, "Starting MCP server")
	cliui.Infof(inv.Stderr, "User          : %s", me.Username)
	cliui.Infof(inv.Stderr, "URL           : %s", client.URL)
	cliui.Infof(inv.Stderr, "Instructions  : %q", instructions)
	if len(allowedTools) > 0 {
		cliui.Infof(inv.Stderr, "Allowed Tools : %v", allowedTools)
	}
	cliui.Infof(inv.Stderr, "Press Ctrl+C to stop the server")

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

	// Create a new context for the tools with all relevant information.
	tb := toolsdk.Deps{
		CoderClient: client,
	}
	// Get the workspace agent token from the environment.
	var hasAgentClient bool
	if agentToken, err := getAgentToken(fs); err == nil && agentToken != "" {
		hasAgentClient = true
		agentClient := agentsdk.New(client.URL)
		agentClient.SetSessionToken(agentToken)
		tb.AgentClient = agentClient
	} else {
		cliui.Warnf(inv.Stderr, "CODER_AGENT_TOKEN is not set, task reporting will not be available")
	}
	if appStatusSlug == "" {
		cliui.Warnf(inv.Stderr, "CODER_MCP_APP_STATUS_SLUG is not set, task reporting will not be available.")
	} else {
		tb.AppStatusSlug = appStatusSlug
	}

	// Register tools based on the allowlist (if specified)
	for _, tool := range toolsdk.All {
		// Skip adding the coder_report_task tool if there is no agent client
		if !hasAgentClient && tool.Tool.Name == "coder_report_task" {
			cliui.Warnf(inv.Stderr, "Task reporting not available")
			continue
		}
		if len(allowedTools) == 0 || slices.ContainsFunc(allowedTools, func(t string) bool {
			return t == tool.Tool.Name
		}) {
			mcpSrv.AddTools(mcpFromSDK(tool, tb))
		}
	}

	srv := server.NewStdioServer(mcpSrv)
	done := make(chan error)
	go func() {
		defer close(done)
		srvErr := srv.Listen(ctx, invStdin, invStdout)
		done <- srvErr
	}()

	if err := <-done; err != nil {
		if !errors.Is(err, context.Canceled) {
			cliui.Errorf(inv.Stderr, "Failed to start the MCP server: %s", err)
			return err
		}
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
	coderPrompt = `YOU MUST REPORT YOUR STATUS IMMEDIATELY AFTER EACH USER MESSAGE.
INTERRUPT READING FILES OR ANY OTHER TOOL CALL IF YOU HAVE NOT REPORTED A STATUS YET.
You MUST use the mcp__coder__coder_report_task function with all required parameters:
- summary: Short description of what you're doing
- link: A relevant link for the status
- done: Boolean indicating if the task is complete (true/false)
- emoji: Relevant emoji for the status
- need_user_attention: Boolean indicating if the task needs user attention (true/false)
WHEN TO REPORT (MANDATORY):
1. IMMEDIATELY after receiving ANY user message, before any other actions
2. After completing any task
3. When making significant progress
4. When encountering roadblocks
5. When asking questions
6. Before and after using search tools or making code changes
FAILING TO REPORT STATUS PROPERLY WILL RESULT IN INCORRECT BEHAVIOR.`

	// Define the guard strings
	coderPromptStartGuard  = "<coder-prompt>"
	coderPromptEndGuard    = "</coder-prompt>"
	systemPromptStartGuard = "<system-prompt>"
	systemPromptEndGuard   = "</system-prompt>"
)

func injectClaudeMD(fs afero.Fs, systemPrompt string, claudeMDPath string) error {
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

func getAgentToken(fs afero.Fs) (string, error) {
	token, ok := os.LookupEnv("CODER_AGENT_TOKEN")
	if ok && token != "" {
		return token, nil
	}
	tokenFile, ok := os.LookupEnv("CODER_AGENT_TOKEN_FILE")
	if !ok {
		return "", xerrors.Errorf("CODER_AGENT_TOKEN or CODER_AGENT_TOKEN_FILE must be set for token auth")
	}
	bs, err := afero.ReadFile(fs, tokenFile)
	if err != nil {
		return "", xerrors.Errorf("failed to read agent token file: %w", err)
	}
	return string(bs), nil
}

// mcpFromSDK adapts a toolsdk.Tool to go-mcp's server.ServerTool.
// It assumes that the tool responds with a valid JSON object.
func mcpFromSDK(sdkTool toolsdk.Tool[any, any], tb toolsdk.Deps) server.ServerTool {
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
			result, err := sdkTool.Handler(ctx, tb, request.Params.Arguments)
			if err != nil {
				return nil, err
			}
			var sb strings.Builder
			if err := json.NewEncoder(&sb).Encode(result); err == nil {
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						mcp.NewTextContent(sb.String()),
					},
				}, nil
			}
			// If the result is not JSON, return it as a string.
			// This is a fallback for tools that return non-JSON data.
			resultStr, ok := result.(string)
			if !ok {
				return nil, xerrors.Errorf("tool call result is neither valid JSON or a string, got: %T", result)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(resultStr),
				},
			}, nil
		},
	}
}
