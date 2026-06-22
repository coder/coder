package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

func (r *RootCmd) templateFavorite() *serpent.Command {
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use:   "favorite <template>",
		Short: "Add a template to your favorites",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			template, err := client.TemplateByName(inv.Context(), organization.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get template: %w", err)
			}

			if err := client.FavoriteTemplate(inv.Context(), template.ID); err != nil {
				return xerrors.Errorf("favorite template: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Template %q added to favorites.\n", template.Name)
			return nil
		},
	}

	orgContext.AttachOptions(cmd)
	return cmd
}

func (r *RootCmd) templateUnfavorite() *serpent.Command {
	orgContext := NewOrganizationContext()

	cmd := &serpent.Command{
		Use:   "unfavorite <template>",
		Short: "Remove a template from your favorites",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			template, err := client.TemplateByName(inv.Context(), organization.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get template: %w", err)
			}

			if err := client.UnfavoriteTemplate(inv.Context(), template.ID); err != nil {
				return xerrors.Errorf("unfavorite template: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Template %q removed from favorites.\n", template.Name)
			return nil
		},
	}

	orgContext.AttachOptions(cmd)
	return cmd
}
