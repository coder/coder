package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) favorite() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Aliases:     []string{"fav", "favou" + "rite"},
		Annotations: workspaceCommand,
		Use:         "favorite <workspace>",
		Short:       "Add a workspace to your favorites",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			if err := client.FavoriteWorkspace(inv.Context(), ws.ID); err != nil {
				return xerrors.Errorf("favorite workspace: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Workspace %q added to favorites.\n", ws.Name)
			return nil
		},
	}
	return cmd
}

func (r *RootCmd) unfavorite() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Aliases:     []string{"unfav", "unfavou" + "rite"},
		Annotations: workspaceCommand,
		Use:         "unfavorite <workspace>",
		Short:       "Remove a workspace from your favorites",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ws, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			if err := client.UnfavoriteWorkspace(inv.Context(), ws.ID); err != nil {
				return xerrors.Errorf("unfavorite workspace: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Workspace %q removed from favorites.\n", ws.Name)
			return nil
		},
	}
	return cmd
}
