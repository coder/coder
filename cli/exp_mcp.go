package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	coderdmcp "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
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
		testBinaryName   string
		claudeUseBedrock string

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

			if deprecatedCoderMCPClaudeAPIKey != "" {
				cliui.Warnf(inv.Stderr, "CODER_MCP_CLAUDE_API_KEY is deprecated, use CLAUDE_API_KEY instead")
				claudeAPIKey = deprecatedCoderMCPClaudeAPIKey
			}
			if claudeAPIKey == "" && claudeUseBedrock != "1" {
				cliui.Warnf(inv.Stderr, "CLAUDE_API_KEY is not set.")
			}

			if deprecatedSystemPromptEnv, ok := os.LookupEnv("SYSTEM_PROMPT"); ok {
				cliui.Warnf(inv.Stderr, "SYSTEM_PROMPT is deprecated, use CODER_MCP_CLAUDE_SYSTEM_PROMPT instead")
				systemPrompt = deprecatedSystemPromptEnv
			}

			if err := configureClaude(fs, ClaudeConfig{
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
			if err := injectClaudeMD(fs, coderPrompt, systemPrompt, claudeMDPath); err != nil {
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
			},
			{
				Name:        "coder-prompt",
				Description: "The coder prompt to use for the Claude Code server.",
				Env:         "CODER_MCP_CLAUDE_CODER_PROMPT",
				Flag:        "claude-coder-prompt",
				Value:       serpent.StringOf(&coderPrompt),
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

type mcpServer struct {
	client *codersdk.Client
}

func (r *RootCmd) mcpServer() *serpent.Command {
	var (
		instructions string
		allowedTools []string
	)
	cmd := &serpent.Command{
		Use: "server",
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.TryInitClient(inv)
			if err != nil {
				return err
			}

			srv := &mcpServer{}

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
				}
			} else {
				cliui.Infof(inv.Stderr, "Authentication : None")
			}

			return srv.startServer(inv.Context(), inv, instructions, allowedTools)
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
		},
	}
	return cmd
}

func (s *mcpServer) startServer(ctx context.Context, inv *serpent.Invocation, instructions string, allowedTools []string) error {
	cliui.Infof(inv.Stderr, "Starting MCP server")

	cliui.Infof(inv.Stderr, "Instructions   : %q", instructions)
	if len(allowedTools) > 0 {
		cliui.Infof(inv.Stderr, "Allowed Tools  : %v", allowedTools)
	}

	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "Coder Agent",
		Version: buildinfo.Version(),
	}, &mcp.ServerOptions{
		Instructions: instructions,
	})

	// Every tool needs an authenticated user client.
	if s.client == nil {
		return xerrors.New(notLoggedInMessage)
	}

	toolDeps, err := toolsdk.NewDeps(s.client)
	if err != nil {
		return xerrors.Errorf("failed to initialize tool dependencies: %w", err)
	}

	// Register tools based on the allowlist.  Zero length means allow everything.
	registeredTools := make(map[string]bool, len(toolsdk.All))
	for _, tool := range toolsdk.All {
		// Skip if not allowed.
		if len(allowedTools) > 0 && !slices.ContainsFunc(allowedTools, func(t string) bool {
			return t == tool.Name
		}) {
			continue
		}

		coderdmcp.RegisterSDKTool(mcpSrv, tool, toolDeps)
		registeredTools[tool.Name] = true
	}

	// Skip prompts whose referenced tools are unavailable so clients are
	// not offered workflows they cannot run.
	for _, prompt := range toolsdk.AllPrompts {
		if slices.ContainsFunc(prompt.RequiredTools, func(name string) bool {
			return !registeredTools[name]
		}) {
			continue
		}
		coderdmcp.RegisterSDKPrompt(mcpSrv, prompt)
	}

	done := make(chan error)
	go func() {
		defer close(done)
		srvErr := mcpSrv.Run(ctx, &mcp.IOTransport{
			Reader: io.NopCloser(inv.Stdin),
			Writer: nopWriteCloser{inv.Stdout},
		})
		done <- srvErr
	}()

	cliui.Infof(inv.Stderr, "Press Ctrl+C to stop the server")

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		cliui.Errorf(inv.Stderr, "Failed to start the MCP server: %s", err)
		return err
	}

	return nil
}

// nopWriteCloser adapts the invocation's stdout to the WriteCloser
// the SDK transport requires without closing the underlying stream.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

type ClaudeConfig struct {
	ConfigPath       string
	ProjectDirectory string
	APIKey           string
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
