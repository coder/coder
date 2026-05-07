package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentcontextconfig"
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
	return &serpent.Command{
		Use:   "context",
		Short: "Manage chat context",
		Long:  "Add or clear context files and skills for an active chat session.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.chatContextAddCommand(),
			r.chatContextClearCommand(),
		},
	}
}

func (*RootCmd) chatContextAddCommand() *serpent.Command {
	var (
		dir    string
		chatID string
	)
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "add",
		Short: "Add context to an active chat",
		Long: "Read instruction files and discover skills from a directory, then add " +
			"them as context to an active chat session. Multiple calls " +
			"are additive.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			if dir == "" && inv.Environ.Get("CODER") != "true" {
				return xerrors.New("this command must be run inside a Coder workspace (set --dir to override)")
			}

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			resolvedDir := dir
			if resolvedDir == "" {
				resolvedDir, err = os.Getwd()
				if err != nil {
					return xerrors.Errorf("get working directory: %w", err)
				}
			}
			resolvedDir, err = filepath.Abs(resolvedDir)
			if err != nil {
				return xerrors.Errorf("resolve directory: %w", err)
			}
			info, err := os.Stat(resolvedDir)
			if err != nil {
				return xerrors.Errorf("cannot read directory %q: %w", resolvedDir, err)
			}
			if !info.IsDir() {
				return xerrors.Errorf("%q is not a directory", resolvedDir)
			}

			parts := agentcontextconfig.ContextPartsFromDir(resolvedDir)
			if len(parts) == 0 {
				_, _ = fmt.Fprintln(inv.Stderr, "No context files or skills found in "+resolvedDir)
				return nil
			}

			// Resolve chat ID from flag or auto-detect.
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
		},
		Options: serpent.OptionSet{
			{
				Name:        "Directory",
				Flag:        "dir",
				Description: "Directory to read context files and skills from. Defaults to the current working directory.",
				Value:       serpent.StringOf(&dir),
			},
			{
				Name:        "Chat ID",
				Flag:        "chat",
				Env:         "CODER_CHAT_ID",
				Description: "Chat ID to add context to. Auto-detected from CODER_CHAT_ID, the only active chat, or the only top-level active chat.",
				Value:       serpent.StringOf(&chatID),
			},
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
