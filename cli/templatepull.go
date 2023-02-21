package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"sort"

	"github.com/codeclysm/extract"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templatePull() *cobra.Command {
	var tarMode bool
	cmd := &cobra.Command{
		Use:   "pull <name> [destination]",
		Short: "Download the latest version of a template to a path.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx          = cmd.Context()
				templateName = args[0]
				dest         string
			)

			if len(args) > 1 {
				dest = args[1]
			}

			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			// TODO(JonA): Do we need to add a flag for organization?
			organization, err := CurrentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			template, err := client.TemplateByName(ctx, organization.ID, templateName)
			if err != nil {
				return xerrors.Errorf("template by name: %w", err)
			}

			// Pull the versions for the template. We'll find the latest
			// one and download the source.
			versions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			if err != nil {
				return xerrors.Errorf("template versions by template: %w", err)
			}

			if len(versions) == 0 {
				return xerrors.Errorf("no template versions for template %q", templateName)
			}

			// Sort the slice from newest to oldest template.
			sort.SliceStable(versions, func(i, j int) bool {
				return versions[i].CreatedAt.After(versions[j].CreatedAt)
			})

			latest := versions[0]

			// Download the tar archive.
			raw, ctype, err := client.Download(ctx, latest.Job.FileID)
			if err != nil {
				return xerrors.Errorf("download template: %w", err)
			}

			if ctype != codersdk.ContentTypeTar {
				return xerrors.Errorf("unexpected Content-Type %q, expecting %q", ctype, codersdk.ContentTypeTar)
			}

			if tarMode {
				_, err = cmd.OutOrStdout().Write(raw)
				return err
			}

			if dest == "" {
				dest = templateName + "/"
			}

			_, _ = fmt.Fprintf(cmd.OutOrStderr(), "Extracting template to %q\n", dest)

			// Stat the destination to ensure nothing exists already.
			_, err = os.Stat(dest)
			if err != nil && !xerrors.Is(err, fs.ErrNotExist) {
				return xerrors.Errorf("stat destination: %w", err)
			}

			err = extract.Tar(ctx, bytes.NewReader(raw), dest, nil)
			return err
		},
	}

	cmd.Flags().BoolVar(&tarMode, "tar", false, "output the template as a tar archive to stdout")
	cliui.AllowSkipPrompt(cmd)

	return cmd
}
