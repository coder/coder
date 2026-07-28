package cli

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) chatCommand() *serpent.Command {
	return &serpent.Command{
		Use:   "chat",
		Short: "Manage agent chats",
		Long:  "Commands for interacting with chats from within a workspace.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatContextCommand(),
		},
	}
}

func (r *RootCmd) chatContextCommand() *serpent.Command {
	// socketPath is shared by the in-workspace source commands (list, show,
	// add, remove) and the no-argument refresh, which all talk to the agent's
	// local IPC socket.
	var socketPath string
	return &serpent.Command{
		Use:   "context",
		Short: "Manage workspace context",
		Long: "Inspect and manage the workspace context sources (instruction files, " +
			"skills, and MCP configs) the agent resolves, and refresh a chat to the " +
			"agent's latest snapshot.\n\nThe list, show, add, and remove commands manage " +
			"agent-local sources and must be run from inside the workspace.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatContextListCommand(&socketPath),
			r.chatContextShowCommand(&socketPath),
			r.chatContextAddCommand(&socketPath),
			r.chatContextRemoveCommand(&socketPath),
			r.chatContextRefreshCommand(&socketPath),
		},
		Options: serpent.OptionSet{{
			Flag:        "socket-path",
			Env:         "CODER_AGENT_SOCKET_PATH",
			Description: "Path to the agent socket used by the in-workspace source commands.",
			Value:       serpent.StringOf(&socketPath),
		}},
	}
}

// resolveContextSourcePath makes a user-supplied source path absolute so the
// agent (which requires absolute, canonical paths) accepts it. A leading ~ is
// preserved for the agent to expand against its own home directory. A path that
// is already absolute on the agent's POSIX filesystem (a leading /) is cleaned
// and passed through; filepath.Abs is host-OS specific and would mangle such a
// path on a Windows CLI host, so it is reserved for resolving relative paths
// against the CLI's working directory, which shares the workspace filesystem
// with the agent.
func resolveContextSourcePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", xerrors.New("path is empty")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		return p, nil
	}
	if strings.HasPrefix(p, "/") {
		return path.Clean(p), nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", xerrors.Errorf("resolve path %q: %w", p, err)
	}
	return abs, nil
}

// dialAgentContextSocket connects to the workspace agent's local IPC socket.
// It is only reachable from inside the workspace.
func dialAgentContextSocket(ctx context.Context, socketPath string) (*agentsocket.Client, error) {
	opts := []agentsocket.Option{}
	if socketPath != "" {
		opts = append(opts, agentsocket.WithPath(socketPath))
	}
	client, err := agentsocket.NewClient(ctx, opts...)
	if err != nil {
		return nil, xerrors.Errorf("connect to agent socket (run this from inside the workspace): %w", err)
	}
	return client, nil
}

func (*RootCmd) chatContextListCommand(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]agentsocket.ContextSource{}, []string{"path"}),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Use:   "list",
		Short: "List the workspace context sources registered on the agent",
		Long: "List the additional scan roots registered on this workspace's agent. " +
			"Built-in defaults (the working directory, ~/.coder, ~/.claude) are always " +
			"scanned and are not shown here.\n\nMust be run from inside the workspace.",
		Middleware: serpent.RequireNArgs(0),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := dialAgentContextSocket(ctx, *socketPath)
			if err != nil {
				return err
			}
			defer client.Close()

			sources, err := client.ContextSources(ctx)
			if err != nil {
				return xerrors.Errorf("list context sources: %w", err)
			}
			if len(sources) == 0 && formatter.FormatID() == "table" {
				cliui.Info(inv.Stdout, "No context sources registered.")
				return nil
			}
			out, err := formatter.Format(ctx, sources)
			if err != nil {
				return xerrors.Errorf("format output: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (*RootCmd) chatContextShowCommand(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]agentsocket.ContextResource{},
			[]string{"kind", "name", "source", "status", "size bytes", "error"},
		),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Use:   "show <path>",
		Short: "Show a context source and the resources it contributes",
		Long: "Show a registered context source and the resources the agent currently " +
			"resolves from it (instruction files, skills, MCP configs), including any " +
			"that failed to read or parse.\n\nMust be run from inside the workspace.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := dialAgentContextSocket(ctx, *socketPath)
			if err != nil {
				return err
			}
			defer client.Close()

			path, err := resolveContextSourcePath(inv.Args[0])
			if err != nil {
				return err
			}
			src, err := client.GetContextSource(ctx, path)
			if err != nil {
				return xerrors.Errorf("get context source: %w", err)
			}
			snap, err := client.GetContextSnapshot(ctx)
			if err != nil {
				return xerrors.Errorf("get context snapshot: %w", err)
			}
			resources := make([]agentsocket.ContextResource, 0, len(snap.Resources))
			for _, res := range snap.Resources {
				if res.SourcePath == src.Path {
					resources = append(resources, res)
				}
			}

			if formatter.FormatID() == "table" {
				cliui.Infof(inv.Stdout, "Source: %s (%d resources)", src.Path, len(resources))
			}
			out, err := formatter.Format(ctx, resources)
			if err != nil {
				return xerrors.Errorf("format output: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (*RootCmd) chatContextAddCommand(socketPath *string) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "add <path>",
		Short: "Register a workspace context source",
		Long: "Register a path as an additional context source on this workspace's agent. " +
			"The agent treats it as an extra scan root, applying the same discovery rules " +
			"it uses for the working directory: AGENTS.md / CLAUDE.md / .cursorrules, " +
			".agents/skills/<name>/SKILL.md, and .mcp.json are picked up now and as they " +
			"appear. Any change to a recognized file dirties this workspace's chats until " +
			"you refresh.\n\nA path may be a file or a directory. Must be run from inside " +
			"the workspace.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			path, err := resolveContextSourcePath(inv.Args[0])
			if err != nil {
				return err
			}
			client, err := dialAgentContextSocket(ctx, *socketPath)
			if err != nil {
				return err
			}
			defer client.Close()

			src, err := client.AddContextSource(ctx, path)
			if err != nil {
				return xerrors.Errorf("add context source: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Registered context source %s\n", src.Path)
			return nil
		},
	}
	return cmd
}

func (*RootCmd) chatContextRemoveCommand(socketPath *string) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "remove <path>",
		Short: "Remove a workspace context source",
		Long: "Remove a previously-registered context source from this workspace's agent " +
			"and re-resolve. Built-in default scan roots cannot be removed.\n\nMust be run " +
			"from inside the workspace.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := dialAgentContextSocket(ctx, *socketPath)
			if err != nil {
				return err
			}
			defer client.Close()

			path, err := resolveContextSourcePath(inv.Args[0])
			if err != nil {
				return err
			}
			if err := client.RemoveContextSource(ctx, path); err != nil {
				return xerrors.Errorf("remove context source: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Removed context source %s\n", path)
			return nil
		},
	}
	return cmd
}

func (r *RootCmd) chatContextRefreshCommand(socketPath *string) *serpent.Command {
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "refresh [<chat>]",
		Short: "Refresh chat context to the agent's latest snapshot",
		Long: "Re-pin a chat to the workspace agent's latest context snapshot and clear " +
			"its drift marker. The chat's next turn uses the refreshed context.\n\nWith a " +
			"<chat> argument, refreshes that chat and works from anywhere.\n\nWith no " +
			"argument, run from inside the workspace: forces the agent to re-resolve its " +
			"sources (catching freshly-cloned repos and startup-script writes the watcher " +
			"has not seen yet), then refreshes every drifted chat. This path authenticates " +
			"with the agent token, so it does not require 'coder login'.",
		Middleware: serpent.RequireRangeArgs(0, 1),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			// With a <chat> argument: refresh that specific chat through the
			// user-facing API. Works from anywhere with a logged-in CLI.
			if len(inv.Args) == 1 {
				chatID, err := uuid.Parse(inv.Args[0])
				if err != nil {
					return xerrors.Errorf("invalid chat ID %q: %w", inv.Args[0], err)
				}
				client, err := r.InitClient(inv)
				if err != nil {
					return err
				}
				exp := codersdk.NewExperimentalClient(client)
				chat, err := exp.RefreshChatContext(ctx, chatID)
				if err != nil {
					return xerrors.Errorf("refresh chat context: %w", err)
				}
				_, _ = fmt.Fprintf(inv.Stdout, "Refreshed context for chat %s.\n", chatID)
				if chat.Context != nil && chat.Context.Error != "" {
					_, _ = fmt.Fprintf(inv.Stdout, "Snapshot reported an error: %s\n", chat.Context.Error)
				}
				return nil
			}

			// No argument: in-workspace. Re-resolve the agent's sources over
			// the local context socket, then ask the agent (using its own
			// token) to re-pin every drifted chat. Neither step needs a
			// logged-in user session.
			sock, err := dialAgentContextSocket(ctx, *socketPath)
			if err != nil {
				return xerrors.Errorf("connect to agent context socket "+
					"(run inside the workspace, or pass a <chat> ID): %w", err)
			}
			defer sock.Close()
			snap, err := sock.ResyncContext(ctx)
			if err != nil {
				return xerrors.Errorf("re-resolve agent context: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Re-resolved agent context (version %d, %d resources).\n", snap.Version, len(snap.Resources))
			if snap.SnapshotError != "" {
				_, _ = fmt.Fprintf(inv.Stdout, "Snapshot reported an error: %s\n", snap.SnapshotError)
			}

			agentClient, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}
			resp, err := agentClient.RefreshChatContext(ctx)
			if err != nil {
				return xerrors.Errorf("refresh chat context: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Refreshed %d drifted chat(s).\n", resp.Refreshed)
			return nil
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}
