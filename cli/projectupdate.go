package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisionersdk"
)

func projectUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update <project> [directory]",
		Args:  cobra.MinimumNArgs(1),
		Short: "Update the source-code of a project from a directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			project, err := client.ProjectByName(cmd.Context(), organization.ID, args[0])
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
			projectVersion, err := client.CreateProjectVersion(cmd.Context(), organization.ID, codersdk.CreateProjectVersionRequest{
				ProjectID:     project.ID,
				StorageMethod: database.ProvisionerStorageMethodFile,
				StorageSource: resp.Hash,
				Provisioner:   database.ProvisionerTypeTerraform,
			})
			if err != nil {
				return err
			}
			logs, err := client.ProjectVersionLogsAfter(cmd.Context(), projectVersion.ID, before)
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
			projectVersion, err = client.ProjectVersion(cmd.Context(), projectVersion.ID)
			if err != nil {
				return err
			}

			if projectVersion.Job.Status != codersdk.ProvisionerJobSucceeded {
				return xerrors.New("job failed")
			}

			err = client.UpdateActiveProjectVersion(cmd.Context(), project.ID, codersdk.UpdateActiveProjectVersion{
				ID: projectVersion.ID,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Printf("Updated version!\n")
			return nil
		},
	}
}
