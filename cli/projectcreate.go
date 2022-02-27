package cli

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisionerd"
)

func projectCreate() *cobra.Command {
	var (
		directory   string
		provisioner string
	)
	cmd := &cobra.Command{
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
			_, err = prompt(cmd, &promptui.Prompt{
				Default:   "y",
				IsConfirm: true,
				Label:     fmt.Sprintf("Set up %s in your organization?", color.New(color.FgHiCyan).Sprintf("%q", directory)),
			})
			if err != nil {
				if errors.Is(err, promptui.ErrAbort) {
					return nil
				}
				return err
			}

			name, err := prompt(cmd, &promptui.Prompt{
				Default: filepath.Base(directory),
				Label:   "What's your project's name?",
				Validate: func(s string) error {
					project, _ := client.Project(cmd.Context(), organization.Name, s)
					if project.ID.String() != uuid.Nil.String() {
						return xerrors.New("A project already exists with that name!")
					}
					return nil
				},
			})
			if err != nil {
				return err
			}

			job, err := validateProjectVersionSource(cmd, client, organization, database.ProvisionerType(provisioner), directory)
			if err != nil {
				return err
			}
			project, err := client.CreateProject(cmd.Context(), organization.Name, coderd.CreateProjectRequest{
				Name:               name,
				VersionImportJobID: job.ID,
			})
			if err != nil {
				return err
			}

			_, err = prompt(cmd, &promptui.Prompt{
				Label:     "Create project?",
				IsConfirm: true,
				Default:   "y",
			})
			if err != nil {
				if errors.Is(err, promptui.ErrAbort) {
					return nil
				}
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s The %s project has been created!\n", caret, color.HiCyanString(project.Name))
			_, err = prompt(cmd, &promptui.Prompt{
				Label:     "Create a new workspace?",
				IsConfirm: true,
				Default:   "y",
			})
			if err != nil {
				if errors.Is(err, promptui.ErrAbort) {
					return nil
				}
				return err
			}

			return nil
		},
	}
	currentDirectory, _ := os.Getwd()
	cmd.Flags().StringVarP(&directory, "directory", "d", currentDirectory, "Specify the directory to create from")
	cmd.Flags().StringVarP(&provisioner, "provisioner", "p", "terraform", "Customize the provisioner backend")
	// This is for testing!
	err := cmd.Flags().MarkHidden("provisioner")
	if err != nil {
		panic(err)
	}
	return cmd
}

func validateProjectVersionSource(cmd *cobra.Command, client *codersdk.Client, organization coderd.Organization, provisioner database.ProvisionerType, directory string, parameters ...coderd.CreateParameterValueRequest) (*coderd.ProvisionerJob, error) {
	spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	spin.Writer = cmd.OutOrStdout()
	spin.Suffix = " Uploading current directory..."
	err := spin.Color("fgHiGreen")
	if err != nil {
		return nil, err
	}
	spin.Start()
	defer spin.Stop()

	tarData, err := tarDirectory(directory)
	if err != nil {
		return nil, err
	}
	resp, err := client.UploadFile(cmd.Context(), codersdk.ContentTypeTar, tarData)
	if err != nil {
		return nil, err
	}

	before := time.Now()
	job, err := client.CreateProjectImportJob(cmd.Context(), organization.Name, coderd.CreateProjectImportJobRequest{
		StorageMethod:   database.ProvisionerStorageMethodFile,
		StorageSource:   resp.Hash,
		Provisioner:     provisioner,
		ParameterValues: parameters,
	})
	if err != nil {
		return nil, err
	}
	spin.Suffix = " Waiting for the import to complete..."
	logs, err := client.ProjectImportJobLogsAfter(cmd.Context(), organization.Name, job.ID, before)
	if err != nil {
		return nil, err
	}
	logBuffer := make([]coderd.ProvisionerJobLog, 0, 64)
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		logBuffer = append(logBuffer, log)
	}

	job, err = client.ProjectImportJob(cmd.Context(), organization.Name, job.ID)
	if err != nil {
		return nil, err
	}
	parameterSchemas, err := client.ProjectImportJobSchemas(cmd.Context(), organization.Name, job.ID)
	if err != nil {
		return nil, err
	}
	parameterValues, err := client.ProjectImportJobParameters(cmd.Context(), organization.Name, job.ID)
	if err != nil {
		return nil, err
	}
	spin.Stop()

	if provisionerd.IsMissingParameterError(job.Error) {
		valuesBySchemaID := map[string]coderd.ComputedParameterValue{}
		for _, parameterValue := range parameterValues {
			valuesBySchemaID[parameterValue.SchemaID.String()] = parameterValue
		}
		for _, parameterSchema := range parameterSchemas {
			_, ok := valuesBySchemaID[parameterSchema.ID.String()]
			if ok {
				continue
			}
			value, err := prompt(cmd, &promptui.Prompt{
				Label: fmt.Sprintf("Enter value for %s:", color.HiCyanString(parameterSchema.Name)),
			})
			if err != nil {
				return nil, err
			}
			parameters = append(parameters, coderd.CreateParameterValueRequest{
				Name:              parameterSchema.Name,
				SourceValue:       value,
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
		}
		return validateProjectVersionSource(cmd, client, organization, provisioner, directory, parameters...)
	}

	if job.Status != coderd.ProvisionerJobStatusSucceeded {
		for _, log := range logBuffer {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[tf]"), log.Output)
		}

		return nil, xerrors.New(job.Error)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Successfully imported project source!\n", color.HiGreenString("✓"))

	resources, err := client.ProjectImportJobResources(cmd.Context(), organization.Name, job.ID)
	if err != nil {
		return nil, err
	}
	return &job, displayProjectImportInfo(cmd, parameterSchemas, parameterValues, resources)
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
