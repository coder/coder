package cli

import (
	"fmt"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

func (r *RootCmd) transferWorkspace() *clibase.Cmd {
	var (
		workspaceID string
		toUserID    string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "transfer --workspace <workspaceId> --owner <userId>",
		Short: "Will transfer ownership of a workspace to a different user.",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			if workspaceID == "" {
				w, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Workspace Id:",
				})
				if err != nil {
					return err
				}
				workspaceID = w
			}
			if toUserID == "" {
				u, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text: "To User Id:",
				})
				if err != nil {
					return err
				}
				toUserID = u
			}

			uid, err := uuid.Parse(toUserID)
			if err != nil {
				return xerrors.New("That's not a valid user id!")
			}

			wid, err := uuid.Parse(workspaceID)
			if err != nil {
				return xerrors.New("That's not a valid workspace id!")
			}

			err = client.UpdateWorkspaceOwnerByID(inv.Context(), wid, codersdk.TransferWorkspaceOwnerRequest{
				OwnerID: uid,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stderr, `Workspace owner updated successfully.`)
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:          "workspace",
			FlagShorthand: "w",
			Description:   "Specifies the workspace to move.",
			Value:         clibase.StringOf(&workspaceID),
		},
		{
			Flag:          "owner",
			FlagShorthand: "o",
			Description:   "Specifies the new user to own this workspace.",
			Value:         clibase.StringOf(&toUserID),
		},
	}
	return cmd
}
