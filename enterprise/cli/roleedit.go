package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) editRole() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]codersdk.Role{}, []string{"name", "display_name", "site_permissions", "org_permissions", "user_permissions"}),
			func(data any) (any, error) {
				return []codersdk.Role{data.(codersdk.Role)}, nil
			},
		),
		cliui.JSONFormat(),
	)

	var (
		dryRun bool
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "edit <role_name>",
		Short: "Edit a custom role",
		Long: cli.FormatExamples(
			cli.Example{
				Description: "Run with an input.json file",
				Command:     "coder roles edit custom_name < role.json",
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
		},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			roles, err := client.ListSiteRoles(ctx)
			if err != nil {
				return xerrors.Errorf("listing roles: %w", err)
			}

			// Make sure the role actually exists first
			var originalRole codersdk.AssignableRoles
			for _, r := range roles {
				if strings.EqualFold(inv.Args[0], r.Name) {
					originalRole = r
					break
				}
			}

			if originalRole.Name == "" {
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "No role exists with that name, do you want to create one?",
					Default:   "yes",
					IsConfirm: true,
				})
				if err != nil {
					return xerrors.Errorf("abort: %w", err)
				}

				originalRole.Role = codersdk.Role{
					Name: inv.Args[0],
				}
			}

			var customRole *codersdk.Role
			// Either interactive, or take input mode.
			fi, _ := os.Stdin.Stat()
			if (fi.Mode() & os.ModeCharDevice) == 0 {
				bytes, err := io.ReadAll(os.Stdin)
				if err != nil {
					return xerrors.Errorf("reading stdin: %w", err)
				}

				err = json.Unmarshal(bytes, customRole)
				if err != nil {
					return xerrors.Errorf("parsing stdin json: %w", err)
				}
			} else {
				// Interactive mode
				if len(originalRole.OrganizationPermissions) > 0 {
					return xerrors.Errorf("unable to edit role in interactive mode, it contains organization permissions")
				}

				if len(originalRole.UserPermissions) > 0 {
					return xerrors.Errorf("unable to edit role in interactive mode, it contains user permissions")
				}

				customRole, err = interactiveEdit(inv, &originalRole.Role)
				if err != nil {
					return xerrors.Errorf("editing role: %w", err)
				}
			}

			totalOrg := 0
			for _, o := range customRole.OrganizationPermissions {
				totalOrg += len(o)
			}
			preview := fmt.Sprintf("perms: %d site, %d over %d orgs, %d user",
				len(customRole.SitePermissions), totalOrg, len(customRole.OrganizationPermissions), len(customRole.UserPermissions))
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Are you sure you wish to update the role? " + preview,
				Default:   "yes",
				IsConfirm: true,
			})
			if err != nil {
				return xerrors.Errorf("abort: %w", err)
			}

			var updated codersdk.Role
			if dryRun {
				// Do not actually post
				updated = *customRole
			} else {
				updated, err = client.PatchRole(ctx, *customRole)
				if err != nil {
					return fmt.Errorf("patch role: %w", err)
				}
			}

			_, err = formatter.Format(ctx, updated)
			if err != nil {
				return xerrors.Errorf("formatting: %w", err)
			}
			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func interactiveEdit(inv *serpent.Invocation, role *codersdk.Role) (*codersdk.Role, error) {
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
			applyResourceActions(role, resource, actions)
			// back to resources!
		}
	}
	// This println is required because the prompt ends us on the same line as some text.
	_, _ = fmt.Println()

	return role, nil
}

func applyResourceActions(role *codersdk.Role, resource string, actions []string) {
	// Construct new site perms with only new perms for the resource
	keep := make([]codersdk.Permission, 0)
	for _, perm := range role.SitePermissions {
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

	role.SitePermissions = keep
}

func defaultActions(role *codersdk.Role, resource string) []string {
	defaults := make([]string, 0)
	for _, perm := range role.SitePermissions {
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
	count := 0
	for _, perm := range role.SitePermissions {
		if perm.ResourceType == resource {
			count++
		}
	}
	return fmt.Sprintf("%s :: %d permissions", resource, count)
}
