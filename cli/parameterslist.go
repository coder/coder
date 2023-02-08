package cli

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func parameterList() *cobra.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.Parameter{}, []string{"name", "scope", "destination scheme"}),
		cliui.JSONFormat(),
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, name := args[0], args[1]

			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			organization, err := CurrentOrganization(cmd, client)
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
			case codersdk.ParameterImportJob, "template_version":
				scope = string(codersdk.ParameterImportJob)
				scopeID, err = uuid.Parse(name)
				if err != nil {
					return xerrors.Errorf("%q must be a uuid for this scope type", name)
				}

				// Could be a template_version id or a job id. Check for the
				// version id.
				tv, err := client.TemplateVersion(cmd.Context(), scopeID)
				if err == nil {
					scopeID = tv.Job.ID
				}

			default:
				return xerrors.Errorf("%q is an unsupported scope, use %v", scope, []codersdk.ParameterScope{
					codersdk.ParameterWorkspace, codersdk.ParameterTemplate, codersdk.ParameterImportJob,
				})
			}

			params, err := client.Parameters(cmd.Context(), codersdk.ParameterScope(scope), scopeID)
			if err != nil {
				return xerrors.Errorf("fetch params: %w", err)
			}

			out, err := formatter.Format(cmd.Context(), params)
			if err != nil {
				return xerrors.Errorf("render output: %w", err)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	formatter.AttachFlags(cmd)
	return cmd
}
