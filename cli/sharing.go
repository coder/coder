package cli

import (
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
	"golang.org/x/xerrors"
	"regexp"
)

func (r *RootCmd) sharing() *serpent.Command {
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use:     "sharing [subcommand]",
		Short:   "Commands for managing shared workspaces",
		Aliases: []string{"share"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{r.shareWorkspace(orgContext)},
	}

	orgContext.AttachOptions(cmd)
	return cmd
}

func (r *RootCmd) shareWorkspace(orgContext *OrganizationContext) *serpent.Command {
	var (
		client            = new(codersdk.Client)
		userAndGroupRegex = regexp.MustCompile(`([A-Za-z0-9]+)(?::([A-Za-z0-9]+))?`)
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
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("could not fetch the workspace %s.", inv.Args[0])
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

			groupRoles := make(map[string]codersdk.WorkspaceRole)

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}
			orgGroups, err := client.Groups(inv.Context(), codersdk.GroupArguments{
				Organization: org.ID.String(),
			})

			for _, group := range groups {
				groupAndRole := userAndGroupRegex.FindStringSubmatch(group)
				groupName := groupAndRole[1]
				role := groupAndRole[2]

				var orgGroup *codersdk.Group
				for _, g := range orgGroups {
					if g.Name == groupName {
						orgGroup = &g
						break
					}
				}

				if orgGroup == nil {
					return xerrors.Errorf("Could not find group named %s belonging to the organization %s", groupName, org.Name)
				}

				workspaceRole, err := stringToWorkspaceRole(role)
				if err != nil {
					return err
				}

				groupRoles[orgGroup.ID.String()] = workspaceRole

			}

			err = client.UpdateWorkspaceACL(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceACL{
				UserRoles:  userRoles,
				GroupRoles: groupRoles,
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
