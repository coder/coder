package cli

import (
	"errors"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/serpent"
)
func (r *RootCmd) templatePull() *serpent.Command {
	var (
		tarMode     bool

		zipMode     bool
		versionName string
		orgContext  = NewOrganizationContext()
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "pull <name> [destination]",
		Short: "Download the active, latest, or specified version of a template to a path.",

		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, 2),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			var (
				ctx          = inv.Context()
				templateName = inv.Args[0]
				dest         string
			)
			if len(inv.Args) > 1 {
				dest = inv.Args[1]
			}
			if tarMode && zipMode {
				return fmt.Errorf("either tar or zip can be selected")

			}
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return fmt.Errorf("get current organization: %w", err)

			}
			template, err := client.TemplateByName(ctx, organization.ID, templateName)
			if err != nil {
				return fmt.Errorf("get template by name: %w", err)

			}
			var latestVersion codersdk.TemplateVersion
			{
				// Determine the latest template version and compare with the
				// active version. If they aren't the same, warn the user.

				versions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
					TemplateID: template.ID,
				})
				if err != nil {
					return fmt.Errorf("template versions by template: %w", err)

				}
				if len(versions) == 0 {
					return fmt.Errorf("no template versions for template %q", templateName)
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

					return fmt.Errorf("get active template version: %w", err)
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
					return fmt.Errorf("get template version: %w", err)
				}
				templateVersion = version
			}
			cliui.Info(inv.Stderr, "Pulling template version "+cliui.Bold(templateVersion.Name)+"...")
			var fileFormat string // empty = default, so .tar
			if zipMode {
				fileFormat = codersdk.FormatZip
			}
			// Download the tar archive.
			raw, ctype, err := client.DownloadWithFormat(ctx, templateVersion.Job.FileID, fileFormat)
			if err != nil {
				return fmt.Errorf("download template: %w", err)
			}
			if fileFormat == "" && ctype != codersdk.ContentTypeTar {
				return fmt.Errorf("unexpected Content-Type %q, expecting %q", ctype, codersdk.ContentTypeTar)
			}
			if fileFormat == codersdk.FormatZip && ctype != codersdk.ContentTypeZip {

				return fmt.Errorf("unexpected Content-Type %q, expecting %q", ctype, codersdk.ContentTypeZip)
			}

			if tarMode || zipMode {
				_, err = inv.Stdout.Write(raw)
				return err
			}
			if dest == "" {

				dest = templateName
			}
			clean, err := filepath.Abs(filepath.Clean(dest))
			if err != nil {
				return fmt.Errorf("cleaning destination path %s failed: %w", dest, err)
			}

			dest = clean
			err = os.MkdirAll(dest, 0o750)
			if err != nil {
				return fmt.Errorf("mkdirall %q: %w", dest, err)
			}
			ents, err := os.ReadDir(dest)
			if err != nil {

				return fmt.Errorf("read dir %q: %w", dest, err)
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

			err = provisionersdk.Untar(dest, bytes.NewReader(raw))
			return err

		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Description: "Output the template as a tar archive to stdout.",

			Flag:        "tar",
			Value: serpent.BoolOf(&tarMode),
		},
		{
			Description: "Output the template as a zip archive to stdout.",

			Flag:        "zip",
			Value: serpent.BoolOf(&zipMode),
		},
		{
			Description: "The name of the template version to pull. Use 'active' to pull the active version, 'latest' to pull the latest version, or the name of the template version to pull.",
			Flag:        "version",
			Value: serpent.StringOf(&versionName),
		},
		cliui.SkipPromptOption(),
	}
	orgContext.AttachOptions(cmd)
	return cmd

}
