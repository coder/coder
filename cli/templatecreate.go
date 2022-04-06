package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/briandowns/spinner"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
)

func templateCreate() *cobra.Command {
	var (
		yes         bool
		directory   string
		provisioner string
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a template from the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			var templateName string
			if len(args) == 0 {
				templateName = filepath.Base(directory)
			} else {
				templateName = args[0]
			}
			_, err = client.TemplateByName(cmd.Context(), organization.ID, templateName)
			if err == nil {
				return xerrors.Errorf("A template already exists named %q!", templateName)
			}

			spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
			spin.Writer = cmd.OutOrStdout()
			spin.Suffix = cliui.Styles.Keyword.Render(" Uploading current directory...")
			spin.Start()
			defer spin.Stop()
			archive, err := provisionersdk.Tar(directory)
			if err != nil {
				return err
			}

			resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, archive)
			if err != nil {
				return err
			}
			spin.Stop()

			spin = spinner.New(spinner.CharSets[5], 100*time.Millisecond)
			spin.Writer = cmd.OutOrStdout()
			spin.Suffix = cliui.Styles.Keyword.Render("Something")
			job, parameters, err := createValidTemplateVersion(cmd, client, organization, database.ProvisionerType(provisioner), resp.Hash)
			if err != nil {
				return err
			}

			if !yes {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      "Create template?",
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

			_, err = client.CreateTemplate(cmd.Context(), organization.ID, codersdk.CreateTemplateRequest{
				Name:            templateName,
				VersionID:       job.ID,
				ParameterValues: parameters,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "The %s template has been created!\n", templateName)
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

func createValidTemplateVersion(cmd *cobra.Command, client *codersdk.Client, organization codersdk.Organization, provisioner database.ProvisionerType, hash string, parameters ...codersdk.CreateParameterRequest) (*codersdk.TemplateVersion, []codersdk.CreateParameterRequest, error) {
	before := time.Now()
	version, err := client.CreateTemplateVersion(cmd.Context(), organization.ID, codersdk.CreateTemplateVersionRequest{
		StorageMethod:   database.ProvisionerStorageMethodFile,
		StorageSource:   hash,
		Provisioner:     provisioner,
		ParameterValues: parameters,
	})
	if err != nil {
		return nil, nil, err
	}

	err = cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
		Fetch: func() (codersdk.ProvisionerJob, error) {
			version, err := client.TemplateVersion(cmd.Context(), version.ID)
			return version.Job, err
		},
		Cancel: func() error {
			return client.CancelTemplateVersion(cmd.Context(), version.ID)
		},
		Logs: func() (<-chan codersdk.ProvisionerJobLog, error) {
			return client.TemplateVersionLogsAfter(cmd.Context(), version.ID, before)
		},
	})
	if err != nil {
		if !provisionerd.IsMissingParameterError(err.Error()) {
			return nil, nil, err
		}
	}
	version, err = client.TemplateVersion(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterSchemas, err := client.TemplateVersionSchema(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterValues, err := client.TemplateVersionParameters(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}

	if provisionerd.IsMissingParameterError(version.Job.Error) {
		valuesBySchemaID := map[string]codersdk.TemplateVersionParameter{}
		for _, parameterValue := range parameterValues {
			valuesBySchemaID[parameterValue.SchemaID.String()] = parameterValue
		}
		sort.Slice(parameterSchemas, func(i, j int) bool {
			return parameterSchemas[i].Name < parameterSchemas[j].Name
		})
		missingSchemas := make([]codersdk.TemplateVersionParameterSchema, 0)
		for _, parameterSchema := range parameterSchemas {
			_, ok := valuesBySchemaID[parameterSchema.ID.String()]
			if ok {
				continue
			}
			missingSchemas = append(missingSchemas, parameterSchema)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has required variables! They are scoped to the template, and not viewable after being set.")+"\r\n")
		for _, parameterSchema := range missingSchemas {
			value, err := cliui.ParameterSchema(cmd, parameterSchema)
			if err != nil {
				return nil, nil, err
			}
			parameters = append(parameters, codersdk.CreateParameterRequest{
				Name:              parameterSchema.Name,
				SourceValue:       value,
				SourceScheme:      database.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
		}
		return createValidTemplateVersion(cmd, client, organization, provisioner, hash, parameters...)
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		return nil, nil, xerrors.New(version.Job.Error)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), cliui.Styles.Checkmark.String()+" Successfully imported template source!\n")

	resources, err := client.TemplateVersionResources(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	return &version, parameters, displayTemplateVersionInfo(cmd, resources)
}
