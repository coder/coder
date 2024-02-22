package cli

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templateDelete() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "delete [name...]",
		Short: "Delete templates",
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Options: clibase.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *clibase.Invocation) error {
			var (
				ctx           = inv.Context()
				templateNames = []string{}
				templates     = []codersdk.Template{}
			)

			organization, err := CurrentOrganization(r, inv, client)
			if err != nil {
				return err
			}

			if len(inv.Args) > 0 {
				templateNames = inv.Args

				for _, templateName := range templateNames {
					template, err := client.TemplateByName(ctx, organization.ID, templateName)
					if err != nil {
						return xerrors.Errorf("get template by name: %w", err)
					}
					templates = append(templates, template)
				}
			} else {
				template, err := selectTemplate(inv, client, organization)
				if err != nil {
					return err
				}

				templates = append(templates, template)
				templateNames = append(templateNames, template.Name)
			}

			// Confirm deletion of the template.
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Delete these templates: %s?", pretty.Sprint(cliui.DefaultStyles.Code, strings.Join(templateNames, ", "))),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			for _, template := range templates {
				err := client.DeleteTemplate(ctx, template.ID)
				if err != nil {
					return xerrors.Errorf("delete template %q: %w", template.Name, err)
				}

				_, _ = fmt.Fprintln(
					inv.Stdout, "Deleted template "+pretty.Sprint(cliui.DefaultStyles.Keyword, template.Name)+" at "+cliui.Timestamp(time.Now()),
				)
			}

			return nil
		},
	}

	return cmd
}
