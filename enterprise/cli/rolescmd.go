package cli

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// **NOTE** Only covers site wide roles at present. Org scoped roles maybe
// should be nested under some command that scopes to an org??

func (r *RootCmd) roles() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "roles",
		Short:   "Manage site-wide roles.",
		Aliases: []string{"role"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Hidden: true,
		Children: []*serpent.Command{
			r.showRole(),
		},
	}
	return cmd
}

func (r *RootCmd) showRole() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat([]assignableRolesTableRow{}, []string{"name", "display_name", "built_in", "site_permissions", "org_permissions", "user_permissions"}),
			func(data any) (any, error) {
				input, ok := data.([]codersdk.AssignableRoles)
				if !ok {
					return nil, xerrors.Errorf("expected []codersdk.AssignableRoles got %T", data)
				}
				rows := make([]assignableRolesTableRow, 0, len(input))
				for _, role := range input {
					rows = append(rows, assignableRolesTableRow{
						Name:                    role.Name,
						DisplayName:             role.DisplayName,
						SitePermissions:         fmt.Sprintf("%d permissions", len(role.SitePermissions)),
						OrganizationPermissions: fmt.Sprintf("%d organizations", len(role.OrganizationPermissions)),
						UserPermissions:         fmt.Sprintf("%d permissions", len(role.UserPermissions)),
						Assignable:              role.Assignable,
						BuiltIn:                 role.BuiltIn,
					})
				}
				return rows, nil
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
			roles, err := client.ListSiteRoles(ctx)
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

type assignableRolesTableRow struct {
	Name            string `table:"name,default_sort"`
	DisplayName     string `table:"display_name"`
	SitePermissions string ` table:"site_permissions"`
	// map[<org_id>] -> Permissions
	OrganizationPermissions string `table:"org_permissions"`
	UserPermissions         string `table:"user_permissions"`
	Assignable              bool   `table:"assignable"`
	BuiltIn                 bool   `table:"built_in"`
}
