package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
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
		Short: "Manage chat context",
		Long: "Inspect, refresh, add, or clear the workspace context (instruction " +
			"files and skills) for a chat.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatContextShowCommand(),
			r.chatContextRefreshCommand(),
			r.chatContextAddCommand(),
			r.chatContextClearCommand(),
		},
		Options: serpent.OptionSet{{
			Flag:        "socket-path",
			Env:         "CODER_AGENT_SOCKET_PATH",
			Description: "Path to the agent socket used by the in-workspace source commands.",
			Value:       serpent.StringOf(&socketPath),
		}},
	}
}

// chatContextResourceRow is the table view of a pinned context resource.
type chatContextResourceRow struct {
	Source string `table:"source,default_sort"`
	Kind   string `table:"kind"`
	Size   int64  `table:"size bytes"`
	Skill  string `table:"skill"`
}

// chatContextChangeRow is the table view of one source-level context change.
type chatContextChangeRow struct {
	Status string `table:"status,default_sort"`
	Kind   string `table:"kind"`
	Source string `table:"source"`
	Skill  string `table:"skill"`
}

func (r *RootCmd) chatContextShowCommand() *serpent.Command {
	var outputFormat string
	cmd := &serpent.Command{
		Use:   "show <chat>",
		Short: "Show a chat's pinned workspace context and any drift",
		Long: "Display the workspace context a chat is pinned to (instruction files " +
			"and skills), whether it has drifted from the agent's latest snapshot, " +
			"and the per-source changes when it has.",
		Middleware: serpent.Chain(serpent.RequireNArgs(1)),
		Options: serpent.OptionSet{{
			Name:          "output",
			Flag:          "output",
			FlagShorthand: "o",
			Default:       "text",
			Description:   "Output format. Supported values: text, json.",
			Value:         serpent.EnumOf(&outputFormat, "text", "json"),
		}},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			chatID, err := uuid.Parse(inv.Args[0])
			if err != nil {
				return xerrors.Errorf("invalid chat ID %q: %w", inv.Args[0], err)
			}

			exp := codersdk.NewExperimentalClient(client)
			chat, err := exp.GetChat(ctx, chatID)
			if err != nil {
				return xerrors.Errorf("get chat: %w", err)
			}

			if outputFormat == "json" {
				// Emit the context object directly; it is null when the chat
				// has no pinned context yet.
				out, err := json.MarshalIndent(chat.Context, "", "  ")
				if err != nil {
					return xerrors.Errorf("marshal chat context: %w", err)
				}
				_, _ = fmt.Fprintln(inv.Stdout, string(out))
				return nil
			}
			return renderChatContextText(inv.Stdout, chat)
		},
	}
	return cmd
}

func renderChatContextText(out io.Writer, chat codersdk.Chat) error {
	if chat.Context == nil {
		_, _ = fmt.Fprintf(out, "Chat %s has no pinned workspace context.\n", chat.ID)
		return nil
	}
	cc := chat.Context

	status := "clean"
	if cc.Dirty {
		status = "drifted"
		if cc.DirtySince != nil {
			status = fmt.Sprintf("drifted (since %s)", cc.DirtySince.Format(time.RFC3339))
		}
	}
	_, _ = fmt.Fprintf(out, "Context for chat %s\n", chat.ID)
	_, _ = fmt.Fprintf(out, "  Status: %s\n", status)
	if cc.Error != "" {
		_, _ = fmt.Fprintf(out, "  Error:  %s\n", cc.Error)
	}

	resourceRows := make([]chatContextResourceRow, 0, len(cc.Resources))
	for _, res := range cc.Resources {
		resourceRows = append(resourceRows, chatContextResourceRow{
			Source: res.Source,
			Kind:   string(res.Kind),
			Size:   res.SizeBytes,
			Skill:  res.SkillName,
		})
	}
	_, _ = fmt.Fprintf(out, "\nPinned resources (%d)\n", len(resourceRows))
	if len(resourceRows) == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	} else {
		tbl, err := cliui.DisplayTable(resourceRows, "source", nil)
		if err != nil {
			return xerrors.Errorf("render resources: %w", err)
		}
		_, _ = fmt.Fprintln(out, tbl)
	}

	if !cc.Dirty {
		return nil
	}

	changeRows := make([]chatContextChangeRow, 0, len(cc.Changes))
	for _, change := range cc.Changes {
		changeRows = append(changeRows, chatContextChangeRow{
			Status: string(change.Status),
			Kind:   string(change.Kind),
			Source: change.Source,
			Skill:  change.SkillName,
		})
	}
	_, _ = fmt.Fprintf(out, "\nChanges vs latest snapshot (%d)\n", len(changeRows))
	if len(changeRows) == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	} else {
		tbl, err := cliui.DisplayTable(changeRows, "status", nil)
		if err != nil {
			return xerrors.Errorf("render changes: %w", err)
		}
		_, _ = fmt.Fprintln(out, tbl)
	}
	_, _ = fmt.Fprintf(out, "Run 'coder chat context refresh %s' to adopt the latest context.\n", chat.ID)
	return nil
}

func (r *RootCmd) chatContextRefreshCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "refresh <chat>",
		Short: "Refresh a chat's workspace context to the latest snapshot",
		Long: "Re-pin a chat to the workspace agent's latest context snapshot and " +
			"clear the drift marker. The chat's next turn uses the refreshed context.",
		Middleware: serpent.Chain(serpent.RequireNArgs(1)),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			chatID, err := uuid.Parse(inv.Args[0])
			if err != nil {
				return xerrors.Errorf("invalid chat ID %q: %w", inv.Args[0], err)
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
		},
	}
	return cmd
}

func (*RootCmd) chatContextAddCommand() *serpent.Command {
	var (
		dir    string
		chatID string
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
	var chatID string
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "add <path>",
		Short: "Register a workspace context source",
		Long: "Register a path as an additional context source on this workspace's agent. " +
			"The agent treats it as an extra scan root, applying the same discovery rules " +
			"it uses for the working directory: AGENTS.md / CLAUDE.md / .cursorrules, " +
			".agents/skills/<name>/SKILL.md, and .mcp.json are picked up now and as they " +
			"appear. Any change to a recognized file dirties this workspace's chats until " +
			"you refresh.\n\nA path may be a file or a directory. Must be run from inside " +
			"the workspace.\n\nPass --chat <chat> to keep the legacy one-shot behavior: read " +
			"context from the path once and inject it into a single chat without " +
			"registering a source.",
		Middleware: serpent.RequireNArgs(1),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			// Legacy one-shot inject into a single chat.
			if chatID != "" {
				return addChatContextOneShot(ctx, inv, agentAuth, inv.Args[0], chatID)
			}

			// Source registration (default).
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
		Options: serpent.OptionSet{{
			Name:        "Chat ID",
			Flag:        "chat",
			Env:         "CODER_CHAT_ID",
			Description: "Inject context from <path> into a single chat (legacy one-shot) instead of registering a source. Auto-detected from CODER_CHAT_ID, the only active chat, or the only top-level active chat.",
			Value:       serpent.StringOf(&chatID),
		}},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

// addChatContextOneShot preserves the legacy `add --chat` behavior: read
// context files and skills from a directory and inject them into a single
// chat via coderd, without registering a persistent source.
func addChatContextOneShot(ctx context.Context, inv *serpent.Invocation, agentAuth *AgentAuth, dir, chatID string) error {
	client, err := agentAuth.CreateClient()
	if err != nil {
		return xerrors.Errorf("create agent client: %w", err)
	}

	resolvedDir, err := filepath.Abs(dir)
	if err != nil {
		return xerrors.Errorf("resolve directory: %w", err)
	}
	info, err := os.Stat(resolvedDir)
	if err != nil {
		return xerrors.Errorf("cannot read directory %q: %w", resolvedDir, err)
	}
	if !info.IsDir() {
		return xerrors.Errorf("--chat one-shot inject requires a directory, but %q is a file", resolvedDir)
	}

	parts := agentcontextconfig.ContextPartsFromDir(resolvedDir)
	if len(parts) == 0 {
		_, _ = fmt.Fprintln(inv.Stderr, "No context files or skills found in "+resolvedDir)
		return nil
	}

	resolvedChatID, err := parseChatID(chatID)
	if err != nil {
		return err
	}

	resp, err := client.AddChatContext(ctx, agentsdk.AddChatContextRequest{
		ChatID: resolvedChatID,
		Parts:  parts,
	})
	if err != nil {
		return xerrors.Errorf("add chat context: %w", err)
	}

	_, _ = fmt.Fprintf(inv.Stdout, "Added %d context part(s) to chat %s\n", resp.Count, resp.ChatID)
	return nil
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

func (*RootCmd) chatContextClearCommand() *serpent.Command {
	var chatID string
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "clear",
		Short: "Clear context from an active chat",
		Long: "Soft-delete all context-file and skill messages from an active chat. " +
			"The next turn will re-fetch default context from the agent.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			resolvedChatID, err := parseChatID(chatID)
			if err != nil {
				return err
			}

			resp, err := client.ClearChatContext(ctx, agentsdk.ClearChatContextRequest{
				ChatID: resolvedChatID,
			})
			if err != nil {
				return xerrors.Errorf("clear chat context: %w", err)
			}

			if resp.ChatID == uuid.Nil {
				_, _ = fmt.Fprintln(inv.Stdout, "No active chats to clear.")
			} else {
				_, _ = fmt.Fprintf(inv.Stdout, "Cleared context from chat %s\n", resp.ChatID)
			}
			return nil
		},
		Options: serpent.OptionSet{{
			Name:        "Chat ID",
			Flag:        "chat",
			Env:         "CODER_CHAT_ID",
			Description: "Chat ID to clear context from. Auto-detected from CODER_CHAT_ID, the only active chat, or the only top-level active chat.",
			Value:       serpent.StringOf(&chatID),
		}},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

// parseChatID returns the chat UUID from the flag value (which
// serpent already populates from --chat or CODER_CHAT_ID). Returns
// uuid.Nil if empty (the server will auto-detect).
func parseChatID(flagValue string) (uuid.UUID, error) {
	if flagValue == "" {
		return uuid.Nil, nil
	}
	parsed, err := uuid.Parse(flagValue)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("invalid chat ID %q: %w", flagValue, err)
	}
	return parsed, nil
}
