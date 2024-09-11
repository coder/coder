package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) groupCreate() *serpent.Command {
	var (
		avatarURL   string
		displayName string
		orgContext  = agpl.NewOrganizationContext()
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "create <name>",
		Short: "Create a user group",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			err = codersdk.GroupNameValid(inv.Args[0])
			if err != nil {
				return xerrors.Errorf("group name %q is invalid: %w", inv.Args[0], err)
			}

			group, err := client.CreateGroup(ctx, org.ID, codersdk.CreateGroupRequest{
				Name:        inv.Args[0],
				DisplayName: displayName,
				AvatarURL:   avatarURL,
			})
			if err != nil {
				return xerrors.Errorf("create group: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully created group %s!\n", pretty.Sprint(cliui.DefaultStyles.Keyword, group.Name))
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "avatar-url",
			Description:   `Set an avatar for a group.`,
			FlagShorthand: "u",
			Env:           "CODER_AVATAR_URL",
			Value:         serpent.StringOf(&avatarURL),
		},
		{
			Flag:        "display-name",
			Description: `Optional human friendly name for the group.`,
			Env:         "CODER_DISPLAY_NAME",
			Value: serpent.Validate(serpent.StringOf(&displayName), func(_displayName *serpent.String) error {
				displayName := _displayName.String()
				if displayName != "" {
					err := codersdk.DisplayNameValid(displayName)
					if err != nil {
						return xerrors.Errorf("group display name %q is invalid: %w", displayName, err)
					}
				}
				return nil
			}),
		},
	}
	orgContext.AttachOptions(cmd)

	return cmd
}
