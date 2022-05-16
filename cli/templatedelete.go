package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templateDelete() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [name...]",
		Short: "Delete templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx           = cmd.Context()
				templateNames = []string{}
				templates     = []codersdk.Template{}
			)

			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			if len(args) > 0 {
				templateNames = args
			} else {
				allTemplates, err := client.TemplatesByOrganization(ctx, organization.ID)
				if err != nil {
					return xerrors.Errorf("get templates by organization: %w", err)
				}

				if len(allTemplates) == 0 {
					return xerrors.Errorf("no templates exist in the current organization %q", organization.Name)
				}

				opts := make([]string, 0, len(allTemplates))
				for _, template := range allTemplates {
					opts = append(opts, template.Name)
				}

				selection, err := cliui.Select(cmd, cliui.SelectOptions{
					Options: opts,
				})
				if err != nil {
					return xerrors.Errorf("select template: %w", err)
				}

				for _, template := range allTemplates {
					if template.Name == selection {
						templates = append(templates, template)
					}
				}
			}

			for _, templateName := range templateNames {
				template, err := client.TemplateByName(ctx, organization.ID, templateName)
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
				}

				templates = append(templates, template)
			}

			for _, template := range templates {
				err := client.DeleteTemplate(ctx, template.ID)
				if err != nil {
					return xerrors.Errorf("delete template %q: %w", template.Name, err)
				}

				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Deleted template "+cliui.Styles.Code.Render(template.Name)+"!")
			}

			return nil
		},
	}
}
