package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) chatShareRemoveCommand() *serpent.Command {
	var users []string
	var groups []string

	return &serpent.Command{
		Use:   "remove <chat-id> --user <user> --group <group>",
		Short: "Remove shared access for users or groups from a chat.",
		Options: serpent.OptionSet{
			{
				Name:        "user",
				Description: "A comma separated list of users to remove shared chat access from.",
				Flag:        "user",
				Value:       serpent.StringArrayOf(&users),
			}, {
				Name:        "group",
				Description: "A comma separated list of groups to remove shared chat access from.",
				Flag:        "group",
				Value:       serpent.StringArrayOf(&groups),
			},
		},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			if len(users) == 0 && len(groups) == 0 {
				return xerrors.New("at least one user or group must be provided")
			}

			chatID, err := parseChatShareID(inv.Args[0])
			if err != nil {
				return err
			}

			userRoleStrings := make([][2]string, len(users))
			for i, user := range users {
				parsed, err := parseChatShareActor(user)
				if err != nil {
					return xerrors.Errorf("invalid user format %q: %w", user, err)
				}
				userRoleStrings[i] = [2]string{parsed, string(codersdk.ChatRoleDeleted)}
			}

			groupRoleStrings := make([][2]string, len(groups))
			for i, group := range groups {
				parsed, err := parseChatShareActor(group)
				if err != nil {
					return xerrors.Errorf("invalid group format %q: %w", group, err)
				}
				groupRoleStrings[i] = [2]string{parsed, string(codersdk.ChatRoleDeleted)}
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			experimentalClient := codersdk.NewExperimentalClient(client)

			chat, err := experimentalClient.GetChat(inv.Context(), chatID)
			if err != nil {
				return xerrors.Errorf("unable to fetch chat %s: %w", inv.Args[0], err)
			}

			userRoles, groupRoles, err := fetchChatUsersAndGroups(inv.Context(), chatRoleLookupParams{
				Client: client,
				OrgID:  chat.OrganizationID,
				Users:  userRoleStrings,
				Groups: groupRoleStrings,
			})
			if err != nil {
				return err
			}

			if err := experimentalClient.UpdateChatACL(inv.Context(), chat.ID, codersdk.UpdateChatACL{
				UserRoles:  userRoles,
				GroupRoles: groupRoles,
			}); err != nil {
				return err
			}

			acl, err := experimentalClient.GetChatACL(inv.Context(), chat.ID)
			if err != nil {
				return xerrors.Errorf("could not fetch current chat ACL after sharing: %w", err)
			}
			out, err := chatACLToTable(inv.Context(), &acl)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
}
