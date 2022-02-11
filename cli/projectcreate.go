package cli

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func projectCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a project from the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			workingDir, err := os.Getwd()
			if err != nil {
				return err
			}

			_, err = runPrompt(cmd, &promptui.Prompt{
				Default:   "y",
				IsConfirm: true,
				Label:     fmt.Sprintf("Set up %s in your organization?", color.New(color.FgHiCyan).Sprintf("%q", workingDir)),
			})
			if err != nil {
				return err
			}

			name, err := runPrompt(cmd, &promptui.Prompt{
				Default: filepath.Base(workingDir),
				Label:   "What's your project's name?",
				Validate: func(s string) error {
					_, err = client.Project(cmd.Context(), organization.Name, s)
					if err == nil {
						return xerrors.New("A project already exists with that name!")
					}
					return nil
				},
			})
			if err != nil {
				return err
			}

			spin := spinner.New(spinner.CharSets[0], 50*time.Millisecond)
			spin.Suffix = " Uploading current directory..."
			spin.Start()
			defer spin.Stop()

			bytes, err := tarDirectory(workingDir)
			if err != nil {
				return err
			}

			resp, err := client.UploadFile(cmd.Context(), codersdk.ContentTypeTar, bytes)
			if err != nil {
				return err
			}

			job, err := client.CreateProjectVersionImportProvisionerJob(cmd.Context(), organization.Name, coderd.CreateProjectImportJobRequest{
				StorageMethod: database.ProvisionerStorageMethodFile,
				StorageSource: resp.Hash,
				Provisioner:   database.ProvisionerTypeTerraform,
				// SkipResources on first import to detect variables defined by the project.
				SkipResources: true,
			})
			if err != nil {
				return err
			}
			spin.Stop()

			logs, err := client.FollowProvisionerJobLogsAfter(context.Background(), organization.Name, job.ID, time.Time{})
			if err != nil {
				return err
			}
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[parse]"), log.Output)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Create project %q!\n", name)
			return nil
		},
	}
}

func tarDirectory(directory string) ([]byte, error) {
	var buffer bytes.Buffer
	tarWriter := tar.NewWriter(&buffer)
	err := filepath.Walk(directory, func(file string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(fileInfo, file)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(directory, file)
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if fileInfo.IsDir() {
			return nil
		}
		data, err := os.Open(file)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tarWriter, data); err != nil {
			return err
		}
		return data.Close()
	})
	if err != nil {
		return nil, err
	}
	err = tarWriter.Flush()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
