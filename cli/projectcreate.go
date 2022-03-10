package cli

import (
	"errors"
	"fmt"
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
	"github.com/coder/coder/provisionersdk"
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
					project, _ := client.ProjectByName(cmd.Context(), organization.ID, s)
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

			project, err := client.CreateProject(cmd.Context(), organization.ID, coderd.CreateProjectRequest{
				Name:      name,
				VersionID: job.ID,
			})
			if err != nil {
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

func validateProjectVersionSource(cmd *cobra.Command, client *codersdk.Client, organization coderd.Organization, provisioner database.ProvisionerType, directory string, parameters ...coderd.CreateParameterRequest) (*coderd.ProjectVersion, error) {
	spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	spin.Writer = cmd.OutOrStdout()
	spin.Suffix = " Uploading current directory..."
	err := spin.Color("fgHiGreen")
	if err != nil {
		return nil, err
	}
	spin.Start()
	defer spin.Stop()

	tarData, err := provisionersdk.Tar(directory)
	if err != nil {
		return nil, err
	}
	resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, tarData)
	if err != nil {
		return nil, err
	}

	before := time.Now()
	version, err := client.CreateProjectVersion(cmd.Context(), organization.ID, coderd.CreateProjectVersionRequest{
		StorageMethod:   database.ProvisionerStorageMethodFile,
		StorageSource:   resp.Hash,
		Provisioner:     provisioner,
		ParameterValues: parameters,
	})
	if err != nil {
		return nil, err
	}
	spin.Suffix = " Waiting for the import to complete..."
	logs, err := client.ProjectVersionLogsAfter(cmd.Context(), version.ID, before)
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

	version, err = client.ProjectVersion(cmd.Context(), version.ID)
	if err != nil {
		return nil, err
	}
	parameterSchemas, err := client.ProjectVersionSchema(cmd.Context(), version.ID)
	if err != nil {
		return nil, err
	}
	parameterValues, err := client.ProjectVersionParameters(cmd.Context(), version.ID)
	if err != nil {
		return nil, err
	}
	spin.Stop()

	if provisionerd.IsMissingParameterError(version.Job.Error) {
		valuesBySchemaID := map[string]coderd.ProjectVersionParameter{}
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
			parameters = append(parameters, coderd.CreateParameterRequest{
				Name:              parameterSchema.Name,
				SourceValue:       value,
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
		}
		return validateProjectVersionSource(cmd, client, organization, provisioner, directory, parameters...)
	}

	if version.Job.Status != coderd.ProvisionerJobSucceeded {
		for _, log := range logBuffer {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[tf]"), log.Output)
		}
		return nil, xerrors.New(version.Job.Error)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Successfully imported project source!\n", color.HiGreenString("✓"))

	resources, err := client.ProjectVersionResources(cmd.Context(), version.ID)
	if err != nil {
		return nil, err
	}
	return &version, displayProjectImportInfo(cmd, parameterSchemas, parameterValues, resources)
}
