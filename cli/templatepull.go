package cli

import (
	"fmt"
	"io/fs"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templatePull() *cobra.Command {
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
			organization, err := currentOrganization(cmd, client)
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

			// If the destination is empty then we write to stdout
			// and bail early.
			if dest == "" {
				_, err = cmd.OutOrStdout().Write(raw)
				if err != nil {
					return xerrors.Errorf("write stdout: %w", err)
				}
				return nil
			}

			// Stat the destination to ensure nothing exists already.
			fi, err := os.Stat(dest)
			if err != nil && !xerrors.Is(err, fs.ErrNotExist) {
				return xerrors.Errorf("stat destination: %w", err)
			}

			if fi != nil && fi.IsDir() {
				// If the destination is a directory we just bail.
				return xerrors.Errorf("%q already exists.", dest)
			}

			// If a file exists at the destination prompt the user
			// to ensure we don't overwrite something valuable.
			if fi != nil {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      fmt.Sprintf("%q already exists, do you want to overwrite it?", dest),
					IsConfirm: true,
				})
				if err != nil {
					return xerrors.Errorf("parse prompt: %w", err)
				}
			}

			err = os.WriteFile(dest, raw, 0600)
			if err != nil {
				return xerrors.Errorf("write to path: %w", err)
			}

			return nil
		},
	}

	cliui.AllowSkipPrompt(cmd)

	return cmd
}
