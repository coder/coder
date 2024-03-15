package cli

import (
	"fmt"
	"net/mail"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) groupEdit() *serpent.Cmd {
	var (
		avatarURL   string
		name        string
		displayName string
		addUsers    []string
		rmUsers     []string
	)
	client := new(codersdk.Client)
	cmd := &serpent.Cmd{
		Use:   "edit <name>",
		Short: "Edit a user group",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			var (
				ctx       = inv.Context()
				groupName = inv.Args[0]
			)

			org, err := agpl.CurrentOrganization(&r.RootCmd, inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			group, err := client.GroupByOrgAndName(ctx, org.ID, groupName)
			if err != nil {
				return xerrors.Errorf("group by org and name: %w", err)
			}

			req := codersdk.PatchGroupRequest{
				Name: name,
			}

			if avatarURL != "" {
				req.AvatarURL = &avatarURL
			}

			if inv.ParsedFlags().Lookup("display-name").Changed {
				req.DisplayName = &displayName
			}

			userRes, err := client.Users(ctx, codersdk.UsersRequest{})
			if err != nil {
				return xerrors.Errorf("get users: %w", err)
			}

			req.AddUsers, err = convertToUserIDs(addUsers, userRes.Users)
			if err != nil {
				return xerrors.Errorf("parse add-users: %w", err)
			}

			req.RemoveUsers, err = convertToUserIDs(rmUsers, userRes.Users)
			if err != nil {
				return xerrors.Errorf("parse rm-users: %w", err)
			}

			group, err = client.PatchGroup(ctx, group.ID, req)
			if err != nil {
				return xerrors.Errorf("patch group: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully patched group %s!\n", pretty.Sprint(cliui.DefaultStyles.Keyword, group.Name))
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "name",
			FlagShorthand: "n",
			Description:   "Update the group name.",
			Value:         serpent.StringOf(&name),
		},
		{
			Flag:          "avatar-url",
			FlagShorthand: "u",
			Description:   "Update the group avatar.",
			Value:         serpent.StringOf(&avatarURL),
		},
		{
			Flag:        "display-name",
			Description: `Optional human friendly name for the group.`,
			Env:         "CODER_DISPLAY_NAME",
			Value:       serpent.StringOf(&displayName),
		},
		{
			Flag:          "add-users",
			FlagShorthand: "a",
			Description:   "Add users to the group. Accepts emails or IDs.",
			Value:         serpent.StringArrayOf(&addUsers),
		},
		{
			Flag:          "rm-users",
			FlagShorthand: "r",
			Description:   "Remove users to the group. Accepts emails or IDs.",
			Value:         serpent.StringArrayOf(&rmUsers),
		},
	}

	return cmd
}

// convertToUserIDs accepts a list of users in the form of IDs or email addresses
// and translates any emails to the matching user ID.
func convertToUserIDs(userList []string, users []codersdk.User) ([]string, error) {
	converted := make([]string, 0, len(userList))

	for _, user := range userList {
		if _, err := uuid.Parse(user); err == nil {
			converted = append(converted, user)
			continue
		}
		if _, err := mail.ParseAddress(user); err == nil {
			for _, u := range users {
				if u.Email == user {
					converted = append(converted, u.ID.String())
					break
				}
			}
			continue
		}

		return nil, xerrors.Errorf("%q must be a valid UUID or email address", user)
	}

	return converted, nil
}
