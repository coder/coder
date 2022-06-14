package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

func templatePull() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull <name> [destination]",
		Short: "Download the latest version of a template to a path.",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx          = cmd.Context()
				templateName = args[0]
				dest         string
			)

			if len(args) > 1 {
				dest = args[1]
			}

			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			// TODO(JonA): Do we need to add a flag for organization?
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			template, err := client.TemplateByName(ctx, organization.ID, templateName)
			if err != nil {
				return xerrors.Errorf("template by name: %w", err)
			}

			versions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			if err != nil {
				return xerrors.Errorf("template versions by template: %w", err)
			}

			if len(versions) == 0 {
				return xerrors.Errorf("no template versions for template %q", templateName)
			}

			// TemplateVersionsByTemplate returns the versions in order from newest
			// to oldest.
			latest := versions[0]

			raw, ctype, err := client.Download(ctx, latest.Job.SourceHash)
			if err != nil {
				return xerrors.Errorf("download template: %w", err)
			}

			if ctype != codersdk.ContentTypeTar {
				return xerrors.Errorf("unexpected Content-Type %q, expecting %q", ctype, codersdk.ContentTypeTar)
			}

			if dest == "" {
				_, err = cmd.OutOrStdout().Write(raw)
				if err != nil {
					return xerrors.Errorf("write stdout: %w", err)
				}
				return nil
			}

			name := fmt.Sprintf("%s.tar", templateName)
			err = os.WriteFile(filepath.Join(dest, name), raw, 0600)
			if err != nil {
				return xerrors.Errorf("write to path: %w", err)
			}

			// TODO(Handle '~')

			return nil
		},
	}

	return cmd
}
