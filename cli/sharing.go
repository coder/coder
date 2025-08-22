package cli

import (
	"regexp"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
	"golang.org/x/xerrors"
)

func (r *RootCmd) sharing() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "sharing [subcommand]",
		Short:   "Commands for managing shared workspaces",
		Aliases: []string{"share"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{r.shareWorkspace()},
	}

	return cmd
}

func (r *RootCmd) shareWorkspace() *serpent.Command {
	var (
		client            = new(codersdk.Client)
		userAndGroupRegex = regexp.MustCompile(`([A-Za-z/0-9]+)(?::([A-Za-z/0-9]+))?`)
		users             []string
		groups            []string
	)

	cmd := &serpent.Command{
		Use:   "share <workspace> --user <user>:<role> --group <group>:<role>",
		Short: "Share a workspace with a user or group.",
		Long:  FormatExamples(Example{Description: "", Command: ""}),
		Options: serpent.OptionSet{
			{
				Name:        "user",
				Description: "TODO.",
				Flag:        "user",
				Value:       serpent.StringArrayOf(&users),
			}, {
				Name:        "group",
				Description: "TODO.",
				Flag:        "group",
				Value:       serpent.StringArrayOf(&groups),
			},
		},
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireRangeArgs(1, -1),
		),
		Handler: func(inv *serpent.Invocation) error {
			workspaceName := inv.Args[0]
			workspace, err := client.WorkspaceByOwnerAndName(inv.Context(), codersdk.Me, workspaceName, codersdk.WorkspaceOptions{
				IncludeDeleted: false,
			})
			if err != nil {
				return xerrors.Errorf("could not fetch workspace %s.", workspaceName)
			}

			userRoles := make(map[string]codersdk.WorkspaceRole, len(users))
			for _, user := range users {
				userAndRole := userAndGroupRegex.FindStringSubmatch(user)
				username := userAndRole[1]
				role := userAndRole[2]

				user, err := client.User(inv.Context(), username)
				if err != nil {
					return err
				}

				workspaceRole, err := stringToWorkspaceRole(role)
				if err != nil {
					return err
				}

				userRoles[user.ID.String()] = workspaceRole
			}

			// groupRoles := make(map[string]codersdk.WorkspaceRole)
			// for _, group := range groups {
			// 	groupAndRole := userAndGroupRegex.FindStringSubmatch(group)
			// 	groupname := groupAndRole[1]
			// 	role := groupAndRole[2]

			// 	group, err := client.Groups()

			// 	workspaceRole, err := stringToWorkspaceRole(role)
			// 	if err != nil {
			// 		return err
			// 	}

			// 	groupRoles[groupname] = workspaceRole

			// }

			err = client.UpdateWorkspaceACL(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceACL{
				UserRoles: userRoles,
				// GroupRoles: groupRoles,
			})
			if err != nil {
				return err
			}

			return nil
		},
	}

	return cmd
}

func stringToWorkspaceRole(role string) (codersdk.WorkspaceRole, error) {

	if role != "" && role != string(codersdk.WorkspaceRoleAdmin) && role != string(codersdk.WorkspaceRoleUse) {
		return "", xerrors.Errorf("invalid role %s. Expected %s, or %s", role, codersdk.WorkspaceRoleAdmin, codersdk.WorkspaceRoleUse)
	}

	workspaceRole := codersdk.WorkspaceRoleUse
	if role == string(codersdk.WorkspaceRoleAdmin) {
		workspaceRole = codersdk.WorkspaceRoleAdmin
	}

	return workspaceRole, nil
}
