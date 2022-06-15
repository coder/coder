package cli

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/spf13/cobra"

	"github.com/coder/coder/codersdk"
)

func parameterList() *cobra.Command {
	var (
		columns []string
	)
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, name := args[0], args[1]

			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			var scopeID uuid.UUID
			switch codersdk.ParameterScope(scope) {
			case codersdk.ParameterWorkspace:
				workspace, err := namedWorkspace(cmd, client, name)
				if err != nil {
					return err
				}
				scopeID = workspace.ID
			case codersdk.ParameterTemplate:
				template, err := client.TemplateByName(cmd.Context(), organization.ID, name)
				if err != nil {
					return xerrors.Errorf("get workspace template: %w", err)
				}
				scopeID = template.ID

			case codersdk.ParameterScopeImportJob, "template_version":
				scope = string(codersdk.ParameterScopeImportJob)
				scopeID, err = uuid.Parse(name)
				if err != nil {
					return xerrors.Errorf("%q must be a uuid for this scope type", name)
				}
			default:
				return xerrors.Errorf("%q is an unsupported scope, use %v", scope, []codersdk.ParameterScope{
					codersdk.ParameterWorkspace, codersdk.ParameterTemplate, codersdk.ParameterScopeImportJob,
				})
			}

			params, err := client.Parameters(cmd.Context(), codersdk.ParameterScope(scope), scopeID)
			if err != nil {
				return xerrors.Errorf("fetch params: %w", err)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), displayParameters(columns, params...))
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&columns, "column", "c", []string{"name", "source_scheme", "destination_scheme"},
		"Specify a column to filter in the table.")
	return cmd
}
