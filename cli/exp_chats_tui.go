package cli

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/muesli/termenv"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

var (
	_ = createChatCmd
	_ = sendMessageCmd
	_ = interruptChatCmd
	_ = listModelsCmd
	_ = loadGitChangesCmd
	_ = loadDiffContentsCmd
	_ = listenToStream
	_ = renderToolCall
	_ = renderToolResult
	_ = renderCompaction
	_ = renderDiffDrawer
	_ = renderModelPicker
)

func installTUISignalHandler(p *tea.Program) func() {
	ch := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		defer func() {
			signal.Stop(sig)
			close(ch)
		}()
		for {
			select {
			case <-ch:
				return
			case <-sig:
				p.Send(terminateTUIMsg{})
			}
		}
	}()
	return func() {
		ch <- struct{}{}
	}
}

func fitHelpText(width int, candidates ...string) string {
	if len(candidates) == 0 {
		return ""
	}
	if width <= 0 {
		return candidates[0]
	}
	for _, candidate := range candidates {
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
	}
	return truncateHelpText(candidates[len(candidates)-1], width)
}

func truncateHelpText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}

	runes := []rune(text)
	for len(runes) > 0 {
		truncated := strings.TrimRight(string(runes), " •|│:") + "…"
		if lipgloss.Width(truncated) <= width {
			return truncated
		}
		runes = runes[:len(runes)-1]
	}
	return "…"
}

func (r *RootCmd) chatsTUI() *serpent.Command {
	var (
		workspaceFlag string
		modelFlag     string
	)

	return &serpent.Command{
		Use:   "tui [chat-id]",
		Short: "Interactive TUI for managing chats.",
		Options: serpent.OptionSet{
			{
				Name:        "workspace",
				Flag:        "workspace",
				Description: "Associate the chat with a workspace by name, owner/name, or UUID.",
				Value:       serpent.StringOf(&workspaceFlag),
			},
			{
				Name:        "model",
				Flag:        "model",
				Description: "Choose a model by ID, provider/model, or display name.",
				Value:       serpent.StringOf(&modelFlag),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			expClient := codersdk.NewExperimentalClient(client)

			if len(inv.Args) > 1 {
				return xerrors.New("expected zero or one chat ID")
			}

			var initialChatID *uuid.UUID
			if len(inv.Args) == 1 {
				chatID, err := uuid.Parse(inv.Args[0])
				if err != nil {
					return xerrors.Errorf("invalid chat ID %q: %w", inv.Args[0], err)
				}
				initialChatID = &chatID
			}

			var workspaceID *uuid.UUID
			if workspaceFlag != "" {
				workspace, err := namedWorkspace(inv.Context(), client, workspaceFlag)
				if err != nil {
					return xerrors.Errorf("resolve workspace %q: %w", workspaceFlag, err)
				}
				workspaceID = &workspace.ID
			}

			modelID, err := resolveModel(inv.Context(), expClient, modelFlag)
			if err != nil {
				return err
			}

			// Set an explicit color profile before Bubble Tea acquires the
			// terminal so lipgloss/termenv don't send OSC color queries that
			// can leak back into stdin as literal input in some terminals.
			lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(
				inv.Stdout,
				termenv.WithProfile(termenv.TrueColor),
			))

			model := newExpChatsTUIModel(inv.Context(), expClient, initialChatID, workspaceID, modelID)
			program := tea.NewProgram(
				model,
				tea.WithAltScreen(),
				tea.WithoutSignalHandler(),
				tea.WithContext(inv.Context()),
				tea.WithInput(inv.Stdin),
				tea.WithOutput(inv.Stdout),
			)

			closeSignalHandler := installTUISignalHandler(program)
			defer closeSignalHandler()

			runModel, err := program.Run()
			if err != nil {
				return err
			}

			if _, ok := runModel.(expChatsTUIModel); !ok {
				return xerrors.New(fmt.Sprintf("unknown model found %T (%+v)", runModel, runModel))
			}

			return nil
		},
	}
}
