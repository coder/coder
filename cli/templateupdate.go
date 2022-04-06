package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
)

func templateUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update <template> [directory]",
		Args:  cobra.MinimumNArgs(1),
		Short: "Update the source-code of a template from a directory.",
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

			directory, err := os.Getwd()
			if err != nil {
				return err
			}
			if len(args) >= 2 {
				directory, err = filepath.Abs(args[1])
				if err != nil {
					return err
				}
			}
			content, err := provisionersdk.Tar(directory)
			if err != nil {
				return err
			}
			resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, content)
			if err != nil {
				return err
			}

			before := time.Now()
			templateVersion, err := client.CreateTemplateVersion(cmd.Context(), organization.ID, codersdk.CreateTemplateVersionRequest{
				TemplateID:    template.ID,
				StorageMethod: database.ProvisionerStorageMethodFile,
				StorageSource: resp.Hash,
				Provisioner:   database.ProvisionerTypeTerraform,
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
				_, _ = fmt.Printf("terraform (%s): %s\n", log.Level, log.Output)
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
}
