package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) organizationRoles(orgContext *OrganizationContext) *serpent.Command {
	cmd := &serpent.Command{
		Use:     "roles",
		Short:   "Manage organization roles.",
		Aliases: []string{"role"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.showOrganizationRoles(orgContext),
			r.updateOrganizationRole(orgContext),
			r.createOrganizationRole(orgContext),
		},
	}
	return cmd
}

func (r *RootCmd) showOrganizationRoles(orgContext *OrganizationContext) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]roleTableRow{}, []string{"name", "display name", "site permissions", "organization permissions", "user permissions"}),
			func(data any) (any, error) {
				inputs, ok := data.([]codersdk.AssignableRoles)
				if !ok {
					return nil, xerrors.Errorf("expected []codersdk.AssignableRoles got %T", data)
				}

				tableRows := make([]roleTableRow, 0)
				for _, input := range inputs {
					tableRows = append(tableRows, roleToTableView(input.Role))
				}

				return tableRows, nil
			},
		),
		cliui.JSONFormat(),
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "show [role_names ...]",
		Short: "Show role(s)",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			roles, err := client.ListOrganizationRoles(ctx, org.ID)
			if err != nil {
				return xerrors.Errorf("listing roles: %w", err)
			}

			if len(inv.Args) > 0 {
				// filter roles
				filtered := make([]codersdk.AssignableRoles, 0)
				for _, role := range roles {
					if slices.ContainsFunc(inv.Args, func(s string) bool {
						return strings.EqualFold(s, role.Name)
					}) {
						filtered = append(filtered, role)
					}
				}
				roles = filtered
			}

			out, err := formatter.Format(inv.Context(), roles)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)

	return cmd
}

func (r *RootCmd) createOrganizationRole(orgContext *OrganizationContext) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]roleTableRow{}, []string{"name", "display name", "site permissions", "organization permissions", "user permissions"}),
			func(data any) (any, error) {
				typed, _ := data.(codersdk.Role)
				return []roleTableRow{roleToTableView(typed)}, nil
			},
		),
		cliui.JSONFormat(),
	)

	var (
		dryRun    bool
		jsonInput bool
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "create <role_name>",
		Short: "Create a new organization custom role",
		Long: FormatExamples(
			Example{
				Description: "Run with an input.json file",
				Command:     "coder organization -O <organization_name> roles create --stidin < role.json",
			},
		),
		Options: []serpent.Option{
			cliui.SkipPromptOption(),
			{
				Name:        "dry-run",
				Description: "Does all the work, but does not submit the final updated role.",
				Flag:        "dry-run",
				Value:       serpent.BoolOf(&dryRun),
			},
			{
				Name:        "stdin",
				Description: "Reads stdin for the json role definition to upload.",
				Flag:        "stdin",
				Value:       serpent.BoolOf(&jsonInput),
			},
		},
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			existingRoles, err := client.ListOrganizationRoles(ctx, org.ID)
			if err != nil {
				return xerrors.Errorf("listing existing roles: %w", err)
			}

			var customRole codersdk.Role
			if jsonInput {
				bytes, err := io.ReadAll(inv.Stdin)
				if err != nil {
					return xerrors.Errorf("reading stdin: %w", err)
				}

				err = json.Unmarshal(bytes, &customRole)
				if err != nil {
					return xerrors.Errorf("parsing stdin json: %w", err)
				}

				if customRole.Name == "" {
					arr := make([]json.RawMessage, 0)
					err = json.Unmarshal(bytes, &arr)
					if err == nil && len(arr) > 0 {
						return xerrors.Errorf("the input appears to be an array, only 1 role can be sent at a time")
					}
					return xerrors.Errorf("json input does not appear to be a valid role")
				}

				if role := existingRole(customRole.Name, existingRoles); role != nil {
					return xerrors.Errorf("The role %s already exists. If you'd like to edit this role use the update command instead", customRole.Name)
				}
			} else {
				if len(inv.Args) == 0 {
					return xerrors.Errorf("missing role name argument, usage: \"coder organizations roles create <role_name>\"")
				}

				if role := existingRole(inv.Args[0], existingRoles); role != nil {
					return xerrors.Errorf("The role %s already exists. If you'd like to edit this role use the update command instead", inv.Args[0])
				}

				interactiveRole, err := interactiveOrgRoleEdit(inv, org.ID, nil)
				if err != nil {
					return xerrors.Errorf("editing role: %w", err)
				}

				customRole = *interactiveRole
			}

			var updated codersdk.Role
			if dryRun {
				// Do not actually post
				updated = customRole
			} else {
				updated, err = client.CreateOrganizationRole(ctx, customRole)
				if err != nil {
					return xerrors.Errorf("patch role: %w", err)
				}
			}

			output, err := formatter.Format(ctx, updated)
			if err != nil {
				return xerrors.Errorf("formatting: %w", err)
			}

			_, err = fmt.Fprintln(inv.Stdout, output)
			return err
		},
	}

	return cmd
}

func (r *RootCmd) updateOrganizationRole(orgContext *OrganizationContext) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]roleTableRow{}, []string{"name", "display name", "site permissions", "organization permissions", "user permissions"}),
			func(data any) (any, error) {
				typed, _ := data.(codersdk.Role)
				return []roleTableRow{roleToTableView(typed)}, nil
			},
		),
		cliui.JSONFormat(),
	)

	var (
		dryRun    bool
		jsonInput bool
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "update <role_name>",
		Short: "Update an organization custom role",
		Long: FormatExamples(
			Example{
				Description: "Run with an input.json file",
				Command:     "coder roles update --stdin < role.json",
			},
		),
		Options: []serpent.Option{
			cliui.SkipPromptOption(),
			{
				Name:        "dry-run",
				Description: "Does all the work, but does not submit the final updated role.",
				Flag:        "dry-run",
				Value:       serpent.BoolOf(&dryRun),
			},
			{
				Name:        "stdin",
				Description: "Reads stdin for the json role definition to upload.",
				Flag:        "stdin",
				Value:       serpent.BoolOf(&jsonInput),
			},
		},
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			existingRoles, err := client.ListOrganizationRoles(ctx, org.ID)
			if err != nil {
				return xerrors.Errorf("listing existing roles: %w", err)
			}

			var customRole codersdk.Role
			if jsonInput {
				bytes, err := io.ReadAll(inv.Stdin)
				if err != nil {
					return xerrors.Errorf("reading stdin: %w", err)
				}

				err = json.Unmarshal(bytes, &customRole)
				if err != nil {
					return xerrors.Errorf("parsing stdin json: %w", err)
				}

				if customRole.Name == "" {
					arr := make([]json.RawMessage, 0)
					err = json.Unmarshal(bytes, &arr)
					if err == nil && len(arr) > 0 {
						return xerrors.Errorf("only 1 role can be sent at a time")
					}
					return xerrors.Errorf("json input does not appear to be a valid role")
				}

				if role := existingRole(customRole.Name, existingRoles); role == nil {
					return xerrors.Errorf("The role %s does not exist. If you'd like to create this role use the create command instead", customRole.Name)
				}
			} else {
				if len(inv.Args) == 0 {
					return xerrors.Errorf("missing role name argument, usage: \"coder organizations roles edit <role_name>\"")
				}

				role := existingRole(inv.Args[0], existingRoles)
				if role == nil {
					return xerrors.Errorf("The role %s does not exist. If you'd like to create this role use the create command instead", inv.Args[0])
				}

				interactiveRole, err := interactiveOrgRoleEdit(inv, org.ID, &role.Role)
				if err != nil {
					return xerrors.Errorf("editing role: %w", err)
				}

				customRole = *interactiveRole

				preview := fmt.Sprintf("permissions: %d site, %d org, %d user",
					len(customRole.SitePermissions), len(customRole.OrganizationPermissions), len(customRole.UserPermissions))
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Are you sure you wish to update the role? " + preview,
					Default:   "yes",
					IsConfirm: true,
				})
				if err != nil {
					return xerrors.Errorf("abort: %w", err)
				}
			}

			var updated codersdk.Role
			if dryRun {
				// Do not actually post
				updated = customRole
			} else {
				updated, err = client.UpdateOrganizationRole(ctx, customRole)
				if err != nil {
					return xerrors.Errorf("patch role: %w", err)
				}
			}

			output, err := formatter.Format(ctx, updated)
			if err != nil {
				return xerrors.Errorf("formatting: %w", err)
			}

			_, err = fmt.Fprintln(inv.Stdout, output)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func interactiveOrgRoleEdit(inv *serpent.Invocation, orgID uuid.UUID, updateRole *codersdk.Role) (*codersdk.Role, error) {
	var originalRole codersdk.Role
	if updateRole == nil {
		originalRole = codersdk.Role{
			Name:           inv.Args[0],
			OrganizationID: orgID.String(),
		}
	} else {
		originalRole = *updateRole
	}

	// Some checks since interactive mode is limited in what it currently sees
	if len(originalRole.SitePermissions) > 0 {
		return nil, xerrors.Errorf("unable to edit role in interactive mode, it contains site wide permissions")
	}

	if len(originalRole.UserPermissions) > 0 {
		return nil, xerrors.Errorf("unable to edit role in interactive mode, it contains user permissions")
	}

	role := &originalRole
	allowedResources := []codersdk.RBACResource{
		codersdk.ResourceTemplate,
		codersdk.ResourceWorkspace,
		codersdk.ResourceUser,
		codersdk.ResourceGroup,
	}

	const done = "Finish and submit changes"
	const abort = "Cancel changes"

	// Now starts the role editing "game".
customRoleLoop:
	for {
		selected, err := cliui.Select(inv, cliui.SelectOptions{
			Message: "Select which resources to edit permissions",
			Options: append(permissionPreviews(role, allowedResources), done, abort),
		})
		if err != nil {
			return role, xerrors.Errorf("selecting resource: %w", err)
		}
		switch selected {
		case done:
			break customRoleLoop
		case abort:
			return role, xerrors.Errorf("edit role %q aborted", role.Name)
		default:
			strs := strings.Split(selected, "::")
			resource := strings.TrimSpace(strs[0])

			actions, err := cliui.MultiSelect(inv, cliui.MultiSelectOptions{
				Message:  fmt.Sprintf("Select actions to allow across the whole deployment for resources=%q", resource),
				Options:  slice.ToStrings(codersdk.RBACResourceActions[codersdk.RBACResource(resource)]),
				Defaults: defaultActions(role, resource),
			})
			if err != nil {
				return role, xerrors.Errorf("selecting actions for resource %q: %w", resource, err)
			}
			applyOrgResourceActions(role, resource, actions)
			// back to resources!
		}
	}
	// This println is required because the prompt ends us on the same line as some text.
	_, _ = fmt.Println()

	return role, nil
}

func applyOrgResourceActions(role *codersdk.Role, resource string, actions []string) {
	if role.OrganizationPermissions == nil {
		role.OrganizationPermissions = make([]codersdk.Permission, 0)
	}

	// Construct new site perms with only new perms for the resource
	keep := make([]codersdk.Permission, 0)
	for _, perm := range role.OrganizationPermissions {
		perm := perm
		if string(perm.ResourceType) != resource {
			keep = append(keep, perm)
		}
	}

	// Add new perms
	for _, action := range actions {
		keep = append(keep, codersdk.Permission{
			Negate:       false,
			ResourceType: codersdk.RBACResource(resource),
			Action:       codersdk.RBACAction(action),
		})
	}

	role.OrganizationPermissions = keep
}

func defaultActions(role *codersdk.Role, resource string) []string {
	if role.OrganizationPermissions == nil {
		role.OrganizationPermissions = []codersdk.Permission{}
	}

	defaults := make([]string, 0)
	for _, perm := range role.OrganizationPermissions {
		if string(perm.ResourceType) == resource {
			defaults = append(defaults, string(perm.Action))
		}
	}
	return defaults
}

func permissionPreviews(role *codersdk.Role, resources []codersdk.RBACResource) []string {
	previews := make([]string, 0, len(resources))
	for _, resource := range resources {
		previews = append(previews, permissionPreview(role, resource))
	}
	return previews
}

func permissionPreview(role *codersdk.Role, resource codersdk.RBACResource) string {
	if role.OrganizationPermissions == nil {
		role.OrganizationPermissions = []codersdk.Permission{}
	}

	count := 0
	for _, perm := range role.OrganizationPermissions {
		if perm.ResourceType == resource {
			count++
		}
	}
	return fmt.Sprintf("%s :: %d permissions", resource, count)
}

func roleToTableView(role codersdk.Role) roleTableRow {
	return roleTableRow{
		Name:                    role.Name,
		DisplayName:             role.DisplayName,
		OrganizationID:          role.OrganizationID,
		SitePermissions:         fmt.Sprintf("%d permissions", len(role.SitePermissions)),
		OrganizationPermissions: fmt.Sprintf("%d permissions", len(role.OrganizationPermissions)),
		UserPermissions:         fmt.Sprintf("%d permissions", len(role.UserPermissions)),
	}
}

func existingRole(newRoleName string, existingRoles []codersdk.AssignableRoles) *codersdk.AssignableRoles {
	for _, existingRole := range existingRoles {
		if strings.EqualFold(newRoleName, existingRole.Name) {
			return &existingRole
		}
	}

	return nil
}

type roleTableRow struct {
	Name            string `table:"name,default_sort"`
	DisplayName     string `table:"display name"`
	OrganizationID  string `table:"organization id"`
	SitePermissions string ` table:"site permissions"`
	// map[<org_id>] -> Permissions
	OrganizationPermissions string `table:"organization permissions"`
	UserPermissions         string `table:"user permissions"`
}
