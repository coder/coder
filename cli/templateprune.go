package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templatePrune() *clibase.Cmd {
	var (
		all clibase.Bool
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "prune [name...]",
		Short: "Prune unused failed template versions from a given template(s)",
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Options: clibase.OptionSet{
			cliui.SkipPromptOption(),
			clibase.Option{
				Name:        "all",
				Description: "Include all unused template versions. By default, only failed template versions are pruned.",
				Flag:        "all",
				Value:       &all,
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			var (
				ctx           = inv.Context()
				templateNames = []string{}
				templates     = []codersdk.Template{}
			)

			organization, err := CurrentOrganization(inv, client)
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

			// Confirm prune of the template.
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Prune template versions of these templates: %s?", pretty.Sprint(cliui.DefaultStyles.Code, strings.Join(templateNames, ", "))),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			for _, template := range templates {
				resp, err := client.PruneTemplateVersions(ctx, template.ID, all.Value())
				if err != nil {
					return xerrors.Errorf("delete template %q: %w", template.Name, err)
				}

				_, _ = fmt.Fprintln(
					inv.Stdout, fmt.Sprintf("Deleted %s versions from "+pretty.Sprint(cliui.DefaultStyles.Keyword, template.Name)+" at "+cliui.Timestamp(time.Now()), len(resp.DeletedIDs)),
				)

				if ok, _ := inv.ParsedFlags().GetBool("verbose"); err == nil && ok {
					data, err := json.Marshal(resp)
					if err != nil {
						return xerrors.Errorf("marshal verbose response: %w", err)
					}
					_, _ = fmt.Fprintln(
						inv.Stdout, string(data),
					)
				}
			}

			return nil
		},
	}

	return cmd
}
