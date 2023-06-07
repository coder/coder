package cli

import (
	"fmt"
	"net/mail"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) groupEdit() *clibase.Cmd {
	var (
		avatarURL string
		name      string
		addUsers  []string
		rmUsers   []string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "edit <name>",
		Short: "Edit a user group",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			var (
				ctx       = inv.Context()
				groupName = inv.Args[0]
			)

			org, err := agpl.CurrentOrganization(inv, client)
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

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully patched group %s!\n", cliui.DefaultStyles.Keyword.Render(group.Name))
			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:          "name",
			FlagShorthand: "n",
			Description:   "Update the group name.",
			Value:         clibase.StringOf(&name),
		},
		{
			Flag:          "avatar-url",
			FlagShorthand: "u",
			Description:   "Update the group avatar.",
			Value:         clibase.StringOf(&avatarURL),
		},
		{
			Flag:          "add-users",
			FlagShorthand: "a",
			Description:   "Add users to the group. Accepts emails or IDs.",
			Value:         clibase.StringArrayOf(&addUsers),
		},
		{
			Flag:          "rm-users",
			FlagShorthand: "r",
			Description:   "Remove users to the group. Accepts emails or IDs.",
			Value:         clibase.StringArrayOf(&rmUsers),
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
