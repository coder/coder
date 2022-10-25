package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
)

func templatePush() *cobra.Command {
	var (
		directory     string
		versionName   string
		provisioner   string
		parameterFile string
		alwaysPrompt  bool
	)

	cmd := &cobra.Command{
		Use:   "push [template]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Push a new template version from the current directory or as specified by flag",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			name := filepath.Base(directory)
			if len(args) > 0 {
				name = args[0]
			}

			template, err := client.TemplateByName(cmd.Context(), organization.ID, name)
			if err != nil {
				return err
			}

			// Confirm upload of the directory.
			prettyDir := prettyDirectoryPath(directory)
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      fmt.Sprintf("Upload %q?", prettyDir),
				IsConfirm: true,
				Default:   cliui.ConfirmYes,
			})
			if err != nil {
				return err
			}

			spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
			spin.Writer = cmd.OutOrStdout()
			spin.Suffix = cliui.Styles.Keyword.Render(" Uploading directory...")
			spin.Start()
			defer spin.Stop()
			content, err := provisionersdk.Tar(directory, provisionersdk.TemplateArchiveLimit)
			if err != nil {
				return err
			}
			resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, content)
			if err != nil {
				return err
			}
			spin.Stop()

			job, _, err := createValidTemplateVersion(cmd, createValidTemplateVersionArgs{
				Name:            versionName,
				Client:          client,
				Organization:    organization,
				Provisioner:     database.ProvisionerType(provisioner),
				FileID:          resp.ID,
				ParameterFile:   parameterFile,
				Template:        &template,
				ReuseParameters: !alwaysPrompt,
			})
			if err != nil {
				return err
			}

			if job.Job.Status != codersdk.ProvisionerJobSucceeded {
				return xerrors.Errorf("job failed: %s", job.Job.Status)
			}

			err = client.UpdateActiveTemplateVersion(cmd.Context(), template.ID, codersdk.UpdateActiveTemplateVersion{
				ID: job.ID,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Updated version at %s!\n", cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}

	currentDirectory, _ := os.Getwd()
	cmd.Flags().StringVarP(&directory, "directory", "d", currentDirectory, "Specify the directory to create from")
	cmd.Flags().StringVarP(&provisioner, "test.provisioner", "", "terraform", "Customize the provisioner backend")
	cmd.Flags().StringVarP(&parameterFile, "parameter-file", "", "", "Specify a file path with parameter values.")
	cmd.Flags().StringVarP(&versionName, "name", "", "", "Specify a name for the new template version. It will be automatically generated if not provided.")
	cmd.Flags().BoolVar(&alwaysPrompt, "always-prompt", false, "Always prompt all parameters. Does not pull parameter values from active template version")
	cliui.AllowSkipPrompt(cmd)
	// This is for testing!
	err := cmd.Flags().MarkHidden("test.provisioner")
	if err != nil {
		panic(err)
	}

	return cmd
}
