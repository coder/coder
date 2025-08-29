package cli

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

const defaultGroupDisplay = "-"

func (r *RootCmd) sharing() *serpent.Command {
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use:     "sharing [subcommand]",
		Short:   "Commands for managing shared workspaces",
		Aliases: []string{"share"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.shareWorkspace(orgContext),
			r.showWorkspaceSharing(),
		},
		Hidden: true,
	}

	orgContext.AttachOptions(cmd)
	return cmd
}

func (r *RootCmd) showWorkspaceSharing() *serpent.Command {
	client := new(codersdk.Client)

	cmd := &serpent.Command{
		Use:     "show <workspace>",
		Short:   "Show all users and groups the given Workspace is shared with.",
		Aliases: []string{"list"},
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			worksace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("unable to fetch Workspace %s: %w", inv.Args[0], err)
			}

			acl, err := client.WorkspaceACL(inv.Context(), worksace.ID)
			if err != nil {
				return xerrors.Errorf("unable to fetch ACL for Workspace: %w", err)
			}

			out, err := workspaceACLToTable(inv.Context(), &acl)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	return cmd
}

func (r *RootCmd) shareWorkspace(orgContext *OrganizationContext) *serpent.Command {
	var (
		// Username regex taken from codersdk/name.go
		nameRoleRegex = regexp.MustCompile(`(^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*)+(?::([A-Za-z0-9-]+))?`)
		client        = new(codersdk.Client)
		users         []string
		groups        []string
	)

	cmd := &serpent.Command{
		Use:   "share <workspace> --user <user>:<role> --group <group>:<role>",
		Short: "Share a workspace with a user or group.",
		Options: serpent.OptionSet{
			{
				Name:        "user",
				Description: "A comma separated list of users to share the workspace with.",
				Flag:        "user",
				Value:       serpent.StringArrayOf(&users),
			}, {
				Name:        "group",
				Description: "A comma separated list of groups to share the workspace with.",
				Flag:        "group",
				Value:       serpent.StringArrayOf(&groups),
			},
		},
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			if len(users) == 0 && len(groups) == 0 {
				return xerrors.New("at least one user or group must be provided")
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("could not fetch the workspace %s: %w", inv.Args[0], err)
			}

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			userRoles := make(map[string]codersdk.WorkspaceRole, len(users))
			if len(users) > 0 {
				orgMembers, err := client.OrganizationMembers(inv.Context(), org.ID)
				if err != nil {
					return err
				}

				for _, user := range users {
					userAndRole := nameRoleRegex.FindStringSubmatch(user)
					username := userAndRole[1]
					role := userAndRole[2]

					userID := ""
					for _, member := range orgMembers {
						if member.Username == username {
							userID = member.UserID.String()
							break
						}
					}
					if userID == "" {
						return xerrors.Errorf("could not find user %s in the organization %s", username, org.Name)
					}

					workspaceRole, err := stringToWorkspaceRole(role)
					if err != nil {
						return err
					}

					userRoles[userID] = workspaceRole
				}
			}

			groupRoles := make(map[string]codersdk.WorkspaceRole)
			if len(groups) > 0 {
				orgGroups, err := client.Groups(inv.Context(), codersdk.GroupArguments{
					Organization: org.ID.String(),
				})
				if err != nil {
					return err
				}

				for _, group := range groups {
					groupAndRole := nameRoleRegex.FindStringSubmatch(group)
					groupName := groupAndRole[1]
					role := groupAndRole[2]

					var orgGroup *codersdk.Group
					for _, group := range orgGroups {
						if group.Name == groupName {
							orgGroup = &group
							break
						}
					}

					if orgGroup == nil {
						return xerrors.Errorf("could not find group named %s belonging to the organization %s", groupName, org.Name)
					}

					workspaceRole, err := stringToWorkspaceRole(role)
					if err != nil {
						return err
					}

					groupRoles[orgGroup.ID.String()] = workspaceRole
				}
			}

			err = client.UpdateWorkspaceACL(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceACL{
				UserRoles:  userRoles,
				GroupRoles: groupRoles,
			})
			if err != nil {
				return err
			}

			acl, err := client.WorkspaceACL(inv.Context(), workspace.ID)
			if err != nil {
				return xerrors.Errorf("could not fetch current workspace ACL after sharing %w", err)
			}

			out, err := workspaceACLToTable(inv.Context(), &acl)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	return cmd
}

func stringToWorkspaceRole(role string) (codersdk.WorkspaceRole, error) {
	switch role {
	case "", string(codersdk.WorkspaceRoleUse):
		return codersdk.WorkspaceRoleUse, nil
	case string(codersdk.WorkspaceRoleAdmin):
		return codersdk.WorkspaceRoleAdmin, nil
	default:
		return "", xerrors.Errorf("invalid role %q: expected %q or %q",
			role, codersdk.WorkspaceRoleAdmin, codersdk.WorkspaceRoleUse)
	}
}

func workspaceACLToTable(ctx context.Context, acl *codersdk.WorkspaceACL) (string, error) {
	type workspaceShareRow struct {
		User  string                 `table:"user"`
		Group string                 `table:"group,default_sort"`
		Role  codersdk.WorkspaceRole `table:"role"`
	}

	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]workspaceShareRow{}, []string{"User", "Group", "Role"}),
		cliui.JSONFormat())

	outputRows := make([]workspaceShareRow, 0)
	for _, user := range acl.Users {
		if user.Role == codersdk.WorkspaceRoleDeleted {
			continue
		}

		outputRows = append(outputRows, workspaceShareRow{
			User:  user.Username,
			Group: defaultGroupDisplay,
			Role:  user.Role,
		})
	}
	for _, group := range acl.Groups {
		if group.Role == codersdk.WorkspaceRoleDeleted {
			continue
		}

		for _, user := range group.Members {
			outputRows = append(outputRows, workspaceShareRow{
				User:  user.Username,
				Group: group.Name,
				Role:  group.Role,
			})
		}
	}
	out, err := formatter.Format(ctx, outputRows)
	if err != nil {
		return "", err
	}

	return out, nil
}
