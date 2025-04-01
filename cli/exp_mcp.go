package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	codermcp "github.com/coder/coder/v2/mcp"
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
	cmd := &serpent.Command{
		Use:   "claude-code",
		Short: "Configure the Claude Code server.",
		Handler: func(_ *serpent.Invocation) error {
			return nil
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

//nolint:revive // control coupling
func mcpServerHandler(inv *serpent.Invocation, client *codersdk.Client, instructions string, allowedTools []string, appStatusSlug string) error {
	ctx, cancel := context.WithCancel(inv.Context())
	defer cancel()

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

	// Create a separate logger for the tools.
	toolLogger := slog.Make(sloghuman.Sink(invStderr))

	toolDeps := codermcp.ToolDeps{
		Client:        client,
		Logger:        &toolLogger,
		AppStatusSlug: appStatusSlug,
		AgentClient:   agentsdk.New(client.URL),
	}

	// Get the workspace agent token from the environment.
	agentToken, ok := os.LookupEnv("CODER_AGENT_TOKEN")
	if ok && agentToken != "" {
		toolDeps.AgentClient.SetSessionToken(agentToken)
	} else {
		cliui.Warnf(inv.Stderr, "CODER_AGENT_TOKEN is not set, task reporting will not be available")
	}

	// Register tools based on the allowlist (if specified)
	reg := codermcp.AllTools()
	if len(allowedTools) > 0 {
		reg = reg.WithOnlyAllowed(allowedTools...)
	}

	reg.Register(mcpSrv, toolDeps)

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
