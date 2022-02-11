package cli

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/xlab/treeprint"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/parameter"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisionerd"
)

func projectCreate() *cobra.Command {
	var (
		directory string
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
			_, err = runPrompt(cmd, &promptui.Prompt{
				Default:   "y",
				IsConfirm: true,
				Label:     fmt.Sprintf("Set up %s in your organization?", color.New(color.FgHiCyan).Sprintf("%q", directory)),
			})
			if err != nil {
				return err
			}

			name, err := runPrompt(cmd, &promptui.Prompt{
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

			job, err := doProjectLoop(cmd, client, organization, directory, []coderd.CreateParameterValueRequest{})
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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s The %s project has been created!\n", color.HiBlackString(">"), color.HiCyanString(project.Name))
			_, err = runPrompt(cmd, &promptui.Prompt{
				Label:     "Create a new workspace?",
				IsConfirm: true,
				Default:   "y",
			})
			if err != nil {
				return err
			}

			fmt.Printf("Create a new workspace now!\n")
			return nil
		},
	}
	currentDirectory, _ := os.Getwd()
	cmd.Flags().StringVarP(&directory, "directory", "d", currentDirectory, "Specify the directory to create from")

	return cmd
}

func doProjectLoop(cmd *cobra.Command, client *codersdk.Client, organization coderd.Organization, directory string, params []coderd.CreateParameterValueRequest) (*coderd.ProvisionerJob, error) {
	spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	spin.Writer = cmd.OutOrStdout()
	spin.Suffix = " Uploading current directory..."
	spin.Color("fgHiGreen")
	spin.Start()
	defer spin.Stop()

	bytes, err := tarDirectory(directory)
	if err != nil {
		return nil, err
	}

	resp, err := client.UploadFile(cmd.Context(), codersdk.ContentTypeTar, bytes)
	if err != nil {
		return nil, err
	}

	job, err := client.CreateProjectVersionImportProvisionerJob(cmd.Context(), organization.Name, coderd.CreateProjectImportJobRequest{
		StorageMethod:   database.ProvisionerStorageMethodFile,
		StorageSource:   resp.Hash,
		Provisioner:     database.ProvisionerTypeTerraform,
		ParameterValues: params,
	})
	if err != nil {
		return nil, err
	}

	spin.Suffix = " Waiting for the import to complete..."

	logs, err := client.FollowProvisionerJobLogsAfter(cmd.Context(), organization.Name, job.ID, time.Time{})
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

	job, err = client.ProvisionerJob(cmd.Context(), organization.Name, job.ID)
	if err != nil {
		return nil, err
	}

	parameterSchemas, err := client.ProvisionerJobParameterSchemas(cmd.Context(), organization.Name, job.ID)
	if err != nil {
		return nil, err
	}
	parameterValues, err := client.ProvisionerJobParameterValues(cmd.Context(), organization.Name, job.ID)
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
			if parameterSchema.Name == parameter.CoderWorkspaceTransition {
				continue
			}
			value, err := runPrompt(cmd, &promptui.Prompt{
				Label: fmt.Sprintf("Enter value for %s:", color.HiCyanString(parameterSchema.Name)),
			})
			if err != nil {
				return nil, err
			}
			params = append(params, coderd.CreateParameterValueRequest{
				Name:              parameterSchema.Name,
				SourceValue:       value,
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
		}
		return doProjectLoop(cmd, client, organization, directory, params)
	}

	if job.Status != coderd.ProvisionerJobStatusSucceeded {
		for _, log := range logBuffer {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[tf]"), log.Output)
		}

		return nil, xerrors.New(job.Error)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Successfully imported project source!\n", color.HiGreenString("✓"))

	resources, err := client.ProvisionerJobResources(cmd.Context(), organization.Name, job.ID)
	if err != nil {
		return nil, err
	}
	return &job, outputProjectInformation(cmd, parameterSchemas, parameterValues, resources)
}

func outputProjectInformation(cmd *cobra.Command, parameterSchemas []coderd.ParameterSchema, parameterValues []coderd.ComputedParameterValue, resources []coderd.ProjectImportJobResource) error {
	schemaByID := map[string]coderd.ParameterSchema{}
	for _, schema := range parameterSchemas {
		schemaByID[schema.ID.String()] = schema
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n  %s\n\n", color.HiBlackString("Parameters"))
	for _, value := range parameterValues {
		schema, ok := schemaByID[value.SchemaID.String()]
		if !ok {
			return xerrors.Errorf("schema not found: %s", value.Name)
		}
		displayValue := value.SourceValue
		if !schema.RedisplayValue {
			displayValue = "<redacted>"
		}
		output := fmt.Sprintf("%s %s %s", color.HiCyanString(value.Name), color.HiBlackString("="), displayValue)
		if value.DefaultSourceValue {
			output += " (default value)"
		} else if value.Scope != database.ParameterScopeImportJob {
			output += fmt.Sprintf(" (inherited from %s)", value.Scope)
		}

		root := treeprint.NewWithRoot(output)
		if schema.Description != "" {
			root.AddBranch(fmt.Sprintf("%s\n%s\n", color.HiBlackString("Description"), schema.Description))
		}
		if schema.AllowOverrideSource {
			root.AddBranch(fmt.Sprintf("%s Users can customize this value!", color.HiYellowString("+")))
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    "+strings.Join(strings.Split(root.String(), "\n"), "\n    "))
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n\n", color.HiBlackString("Resources"))
	for _, resource := range resources {
		transition := color.HiGreenString("start")
		if resource.Transition == database.WorkspaceTransitionStop {
			transition = color.HiRedString("stop")
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    %s %s on %s\n\n", color.HiCyanString(resource.Type), color.HiCyanString(resource.Name), transition)
	}
	return nil
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
