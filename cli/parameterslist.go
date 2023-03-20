package cli

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) parameterList() *clibase.Cmd {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.Parameter{}, []string{"name", "scope", "destination scheme"}),
		cliui.JSONFormat(),
	)

	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use:     "list",
		Aliases: []string{"ls"},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			scope, name := inv.Args[0], inv.Args[1]

			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			var scopeID uuid.UUID
			switch codersdk.ParameterScope(scope) {
			case codersdk.ParameterWorkspace:
				workspace, err := namedWorkspace(inv.Context(), client, name)
				if err != nil {
					return err
				}
				scopeID = workspace.ID
			case codersdk.ParameterTemplate:
				template, err := client.TemplateByName(inv.Context(), organization.ID, name)
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
				tv, err := client.TemplateVersion(inv.Context(), scopeID)
				if err == nil {
					scopeID = tv.Job.ID
				}

			default:
				return xerrors.Errorf("%q is an unsupported scope, use %v", scope, []codersdk.ParameterScope{
					codersdk.ParameterWorkspace, codersdk.ParameterTemplate, codersdk.ParameterImportJob,
				})
			}

			params, err := client.Parameters(inv.Context(), codersdk.ParameterScope(scope), scopeID)
			if err != nil {
				return xerrors.Errorf("fetch params: %w", err)
			}

			out, err := formatter.Format(inv.Context(), params)
			if err != nil {
				return xerrors.Errorf("render output: %w", err)
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
