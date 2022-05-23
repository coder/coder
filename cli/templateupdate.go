package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
)

func templateUpdate() *cobra.Command {
	var (
		directory   string
		provisioner string
	)

	cmd := &cobra.Command{
		Use:   "update <template>",
		Args:  cobra.ExactArgs(1),
		Short: "Update the source-code of a template from the current directory or as specified by flag",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			template, err := client.TemplateByName(cmd.Context(), organization.ID, args[0])
			if err != nil {
				return err
			}

			// Confirm upload of the directory.
			prettyDir := prettyDirectoryPath(directory)
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      fmt.Sprintf("Upload %q?", prettyDir),
				IsConfirm: true,
				Default:   "yes",
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

			before := time.Now()
			templateVersion, err := client.CreateTemplateVersion(cmd.Context(), organization.ID, codersdk.CreateTemplateVersionRequest{
				TemplateID:    template.ID,
				StorageMethod: codersdk.ProvisionerStorageMethodFile,
				StorageSource: resp.Hash,
				Provisioner:   codersdk.ProvisionerType(provisioner),
			})
			if err != nil {
				return err
			}
			logs, err := client.TemplateVersionLogsAfter(cmd.Context(), templateVersion.ID, before)
			if err != nil {
				return err
			}
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				_, _ = fmt.Printf("%s (%s): %s\n", provisioner, log.Level, log.Output)
			}
			templateVersion, err = client.TemplateVersion(cmd.Context(), templateVersion.ID)
			if err != nil {
				return err
			}

			if templateVersion.Job.Status != codersdk.ProvisionerJobSucceeded {
				return xerrors.New("job failed")
			}

			err = client.UpdateActiveTemplateVersion(cmd.Context(), template.ID, codersdk.UpdateActiveTemplateVersion{
				ID: templateVersion.ID,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Printf("Updated version!\n")
			return nil
		},
	}

	currentDirectory, _ := os.Getwd()
	cmd.Flags().StringVarP(&directory, "directory", "d", currentDirectory, "Specify the directory to create from")
	cmd.Flags().StringVarP(&provisioner, "test.provisioner", "", "terraform", "Customize the provisioner backend")
	cliui.AllowSkipPrompt(cmd)
	// This is for testing!
	err := cmd.Flags().MarkHidden("test.provisioner")
	if err != nil {
		panic(err)
	}

	return cmd
}
