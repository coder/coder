package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type ChatModelRow struct {
	ID          string `json:"-" table:"id,nosort"`
	Provider    string `json:"-" table:"provider"`
	Model       string `json:"-" table:"model"`
	DisplayName string `json:"-" table:"display name"`
	Available   bool   `json:"-" table:"available"`
}

func chatModelRows(catalog codersdk.ChatModelsResponse) []ChatModelRow {
	rows := make([]ChatModelRow, 0)
	for _, provider := range catalog.Providers {
		for _, model := range provider.Models {
			rows = append(rows, ChatModelRow{
				ID:          model.ID,
				Provider:    model.Provider,
				Model:       model.Model,
				DisplayName: model.DisplayName,
				Available:   provider.Available,
			})
		}
	}
	return rows
}

func (r *RootCmd) chatsModels() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]ChatModelRow{}, []string{"id", "provider", "model", "display name", "available"}),
			func(data any) (any, error) {
				catalog, ok := data.(codersdk.ChatModelsResponse)
				if !ok {
					return nil, xerrors.Errorf("expected type %T, got %T", codersdk.ChatModelsResponse{}, data)
				}
				return chatModelRows(catalog), nil
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "models",
		Short: "List available chat models.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			expClient := codersdk.NewExperimentalClient(client)

			catalog, err := expClient.ListChatModels(inv.Context())
			if err != nil {
				return xerrors.Errorf("list chat models: %w", err)
			}

			out, err := formatter.Format(inv.Context(), catalog)
			if err != nil {
				return xerrors.Errorf("format chat models: %w", err)
			}
			if out == "" {
				cliui.Infof(inv.Stderr, "No chat models found.")
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
