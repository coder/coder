package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
)

func projectCreate() *cobra.Command {
	var (
		yes         bool
		directory   string
		provisioner string
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
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

			var projectName string
			if len(args) == 0 {
				projectName = filepath.Base(directory)
			} else {
				projectName = args[0]
			}
			_, err = client.ProjectByName(cmd.Context(), organization.ID, projectName)
			if err == nil {
				return xerrors.Errorf("A project already exists named %q!", projectName)
			}

			start := time.Now()
			archive, err := provisionersdk.Tar(directory)
			if err != nil {
				return err
			}
			spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
			spin.Writer = cmd.OutOrStdout()
			spin.Suffix = " Uploading current directory..."
			err = spin.Color("fgHiGreen")
			if err != nil {
				return err
			}
			spin.Start()
			defer spin.Stop()
			resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, archive)
			if err != nil {
				return err
			}
			spin.FinalMSG = fmt.Sprintf("Uploaded %d byte archive! [%dms]", len(archive), time.Now().Sub(start).Milliseconds()) + "\n"
			spin.Stop()

			job, parameters, err := createValidProjectVersion(cmd, client, organization, database.ProvisionerType(provisioner), resp.Hash)
			if err != nil {
				return err
			}

			if !yes {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      "Create project?",
					IsConfirm: true,
					Default:   "yes",
				})
				if err != nil {
					if errors.Is(err, promptui.ErrAbort) {
						return nil
					}
					return err
				}
			}

			_, err = client.CreateProject(cmd.Context(), organization.ID, codersdk.CreateProjectRequest{
				Name:            projectName,
				VersionID:       job.ID,
				ParameterValues: parameters,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf("The %s project has been created!", projectName))
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
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Bypass prompts")
	return cmd
}

func createValidProjectVersion(cmd *cobra.Command, client *codersdk.Client, organization codersdk.Organization, provisioner database.ProvisionerType, hash string, parameters ...codersdk.CreateParameterRequest) (*codersdk.ProjectVersion, []codersdk.CreateParameterRequest, error) {
	before := time.Now()
	version, err := client.CreateProjectVersion(cmd.Context(), organization.ID, codersdk.CreateProjectVersionRequest{
		StorageMethod:   database.ProvisionerStorageMethodFile,
		StorageSource:   hash,
		Provisioner:     provisioner,
		ParameterValues: parameters,
	})
	if err != nil {
		return nil, nil, err
	}
	spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	spin.Writer = cmd.OutOrStdout()
	spin.Suffix = " Waiting for the import to complete..."
	spin.FinalMSG = "Parsed project..."
	spin.Start()

	doneChan := make(chan struct{})
	go func() {
		defer spin.Stop()
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-doneChan:
				return
			case <-ticker.C:
			}
			version, err := client.ProjectVersion(cmd.Context(), version.ID)
			if err != nil {
				continue
			}
			switch version.Job.Status {
			case codersdk.ProvisionerJobPending:
				spin.Suffix = "Pending..."
			case codersdk.ProvisionerJobCanceled:
				spin.Suffix = "Canceled..."
			case codersdk.ProvisionerJobCanceling:
				spin.Suffix = "Canceling..."
			case codersdk.ProvisionerJobFailed:
				spin.FinalMSG = version.Job.Error
			case codersdk.ProvisionerJobRunning:
				spin.Suffix = "Running..."
			}
		}
	}()

	logs, err := client.ProjectVersionLogsAfter(cmd.Context(), version.ID, before)
	if err != nil {
		return nil, nil, err
	}
	logBuffer := make([]codersdk.ProvisionerJobLog, 0, 64)
	for {
		log, ok := <-logs
		if !ok {
			break
		}
		logBuffer = append(logBuffer, log)
	}

	version, err = client.ProjectVersion(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterSchemas, err := client.ProjectVersionSchema(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterValues, err := client.ProjectVersionParameters(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	spin.Stop()

	if provisionerd.IsMissingParameterError(version.Job.Error) {
		valuesBySchemaID := map[string]codersdk.ProjectVersionParameter{}
		for _, parameterValue := range parameterValues {
			valuesBySchemaID[parameterValue.SchemaID.String()] = parameterValue
		}
		sort.Slice(parameterSchemas, func(i, j int) bool {
			return parameterSchemas[i].Name < parameterSchemas[j].Name
		})
		for _, parameterSchema := range parameterSchemas {
			_, ok := valuesBySchemaID[parameterSchema.ID.String()]
			if ok {
				continue
			}
			value, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text: fmt.Sprintf("Enter value for %s:", color.HiCyanString(parameterSchema.Name)),
			})
			if err != nil {
				return nil, nil, err
			}
			parameters = append(parameters, codersdk.CreateParameterRequest{
				Name:              parameterSchema.Name,
				SourceValue:       value,
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
		}
		return createValidProjectVersion(cmd, client, organization, provisioner, hash, parameters...)
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		for _, log := range logBuffer {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[tf]"), log.Output)
		}
		return nil, nil, xerrors.New(version.Job.Error)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Successfully imported project source!\n", color.HiGreenString("✓"))

	resources, err := client.ProjectVersionResources(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	return &version, parameters, displayProjectVersionInfo(cmd, parameterSchemas, parameterValues, resources)
}
