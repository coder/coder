package cli

import (
	"context"
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
	return truncateText(candidates[len(candidates)-1], width, " •|│:", 1)
}

func truncateText(text string, width int, trimRightCutset string, ellipsisWidth int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(text) <= width {
		return text
	}
	if width <= ellipsisWidth {
		return "…"
	}
	for runes := []rune(text); len(runes) > 0; runes = runes[:len(runes)-1] {
		truncated := strings.TrimRight(string(runes), trimRightCutset) + "…"
		if lipgloss.Width(truncated) <= width {
			return truncated
		}
	}
	return "…"
}

func (r *RootCmd) agentsCommand() *serpent.Command {
	var (
		workspaceFlag string
		modelFlag     string
	)

	return &serpent.Command{
		Use:     "agents [chat-id]",
		Short:   "Interactive terminal UI for AI agents.",
		Aliases: []string{"agent"},
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

			orgs, err := client.OrganizationsByUser(inv.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("list organizations: %w", err)
			}
			if len(orgs) == 0 {
				return xerrors.New("no organizations found")
			}
			defaultOrgID := orgs[0].ID

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
				workspace, err := client.ResolveWorkspace(inv.Context(), workspaceFlag)
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
			renderer := lipgloss.NewRenderer(
				inv.Stdout,
				termenv.WithProfile(termenv.TrueColor),
			)
			renderer.SetHasDarkBackground(true)

			model := newExpChatsTUIModel(inv.Context(), expClient, initialChatID, workspaceID, modelID, defaultOrgID)
			model.setRenderer(renderer)
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

//nolint:nilnil // A nil string indicates that no model override was provided.
func resolveModel(ctx context.Context, client *codersdk.ExperimentalClient, modelFlag string) (*string, error) {
	if modelFlag == "" {
		return nil, nil
	}

	if _, err := uuid.Parse(modelFlag); err == nil {
		return &modelFlag, nil
	}

	catalog, err := client.ListChatModels(ctx)
	if err != nil {
		return nil, xerrors.Errorf("listing models: %w", err)
	}

	for _, provider := range catalog.Providers {
		for _, model := range provider.Models {
			if model.ID == modelFlag || model.Provider+"/"+model.Model == modelFlag || model.DisplayName == modelFlag {
				return &model.ID, nil
			}
		}
	}

	return nil, xerrors.Errorf("unknown model %q", modelFlag)
}
