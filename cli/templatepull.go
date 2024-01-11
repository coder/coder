package cli

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/codeclysm/extract/v3"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templatePull() *clibase.Cmd {
	var (
		tarMode     bool
		versionName string
	)

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "pull <name> [destination]",
		Short: "Download the active, latest, or specified version of a template to a path.",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(1, 2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			var (
				ctx          = inv.Context()
				templateName = inv.Args[0]
				dest         string
			)

			if len(inv.Args) > 1 {
				dest = inv.Args[1]
			}

			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			template, err := client.TemplateByName(ctx, organization.ID, templateName)
			if err != nil {
				return xerrors.Errorf("get template by name: %w", err)
			}

			var latestVersion codersdk.TemplateVersion
			{
				// Determine the latest template version and compare with the
				// active version. If they aren't the same, warn the user.
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

				latestVersion = versions[0]
			}

			var templateVersion codersdk.TemplateVersion
			switch versionName {
			case "", "active":
				activeVersion, err := client.TemplateVersion(ctx, template.ActiveVersionID)
				if err != nil {
					return xerrors.Errorf("get active template version: %w", err)
				}
				if versionName == "" && activeVersion.ID != latestVersion.ID {
					cliui.Warn(inv.Stderr,
						"A newer template version than the active version exists. Pulling the active version instead.",
						"Use "+cliui.Code("--version latest")+" to pull the latest version.",
					)
				}
				templateVersion = activeVersion
			case "latest":
				templateVersion = latestVersion
			default:
				version, err := client.TemplateVersionByName(ctx, template.ID, versionName)
				if err != nil {
					return xerrors.Errorf("get template version: %w", err)
				}
				templateVersion = version
			}

			cliui.Info(inv.Stderr, "Pulling template version "+cliui.Bold(templateVersion.Name)+"...")

			// Download the tar archive.
			raw, ctype, err := client.Download(ctx, templateVersion.Job.FileID)
			if err != nil {
				return xerrors.Errorf("download template: %w", err)
			}

			if ctype != codersdk.ContentTypeTar {
				return xerrors.Errorf("unexpected Content-Type %q, expecting %q", ctype, codersdk.ContentTypeTar)
			}

			if tarMode {
				_, err = inv.Stdout.Write(raw)
				return err
			}

			if dest == "" {
				dest = templateName
			}

			err = os.MkdirAll(dest, 0o750)
			if err != nil {
				return xerrors.Errorf("mkdirall %q: %w", dest, err)
			}

			ents, err := os.ReadDir(dest)
			if err != nil {
				return xerrors.Errorf("read dir %q: %w", dest, err)
			}

			if len(ents) > 0 {
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:      fmt.Sprintf("Directory %q is not empty, existing files may be overwritten.\nContinue extracting?", dest),
					Default:   "No",
					Secret:    false,
					IsConfirm: true,
				})
				if err != nil {
					return err
				}
			}

			_, _ = fmt.Fprintf(inv.Stderr, "Extracting template to %q\n", dest)
			err = extract.Tar(ctx, bytes.NewReader(raw), dest, nil)
			return err
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Description: "Output the template as a tar archive to stdout.",
			Flag:        "tar",

			Value: clibase.BoolOf(&tarMode),
		},
		{
			Description: "The name of the template version to pull. Use 'active' to pull the active version, 'latest' to pull the latest version, or the name of the template version to pull.",
			Flag:        "version",

			Value: clibase.StringOf(&versionName),
		},
		cliui.SkipPromptOption(),
	}

	return cmd
}
