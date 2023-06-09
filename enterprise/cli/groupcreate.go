package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) groupCreate() *clibase.Cmd {
	var avatarURL string
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "create <name>",
		Short: "Create a user group",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()

			org, err := agpl.CurrentOrganization(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			group, err := client.CreateGroup(ctx, org.ID, codersdk.CreateGroupRequest{
				Name:      inv.Args[0],
				AvatarURL: avatarURL,
			})
			if err != nil {
				return xerrors.Errorf("create group: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully created group %s!\n", cliui.DefaultStyles.Keyword.Render(group.Name))
			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:          "avatar-url",
			Description:   `Set an avatar for a group.`,
			FlagShorthand: "u",
			Env:           "CODER_AVATAR_URL",
			Value:         clibase.StringOf(&avatarURL),
		},
	}

	return cmd
}
