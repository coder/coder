package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func groupCreate() *clibase.Cmd {
	var avatarURL string
	cmd := &clibase.Cmd{
		Use:        "create <name>",
		Short:      "Create a user group",
		Middleware: clibase.RequireNArgs(1),
		Handler: func(inv *clibase.Invokation) error {
			ctx := inv.Context()

			client, err := agpl.CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			org, err := agpl.CurrentOrganization(cmd, client)
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

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully created group %s!\n", cliui.Styles.Keyword.Render(group.Name))
			return nil
		},
	}

	cliflag.StringVarP(cmd.Flags(), &avatarURL, "avatar-url", "u", "CODER_AVATAR_URL", "", "set an avatar for a group")

	return cmd
}
