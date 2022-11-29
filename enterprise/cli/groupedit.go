package cli

import (
	"fmt"
	"net/mail"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func groupEdit() *cobra.Command {
	var (
		avatarURL string
		name      string
		addUsers  []string
		rmUsers   []string
	)
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a user group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx       = cmd.Context()
				groupName = args[0]
			)

			client, err := agpl.CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			org, err := agpl.CurrentOrganization(cmd, client)
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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Successfully patched group %s!\n", cliui.Styles.Keyword.Render(group.Name))
			return nil
		},
	}

	cliflag.StringVarP(cmd.Flags(), &name, "name", "n", "", "", "Update the group name")
	cliflag.StringVarP(cmd.Flags(), &avatarURL, "avatar-url", "u", "", "", "Update the group avatar")
	cliflag.StringArrayVarP(cmd.Flags(), &addUsers, "add-users", "a", "", nil, "Add users to the group. Accepts emails or IDs.")
	cliflag.StringArrayVarP(cmd.Flags(), &rmUsers, "rm-users", "r", "", nil, "Remove users to the group. Accepts emails or IDs.")
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
