package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/provisionersdk"
)

func templateCreate() *cobra.Command {
	var (
		directory            string
		provisioner          string
		parameterFile        string
		maxTTL               time.Duration
		minAutostartInterval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a template from the current directory or as specified by flag",
		Args:  cobra.MaximumNArgs(1),
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

			// Confirm upload of the directory.
			prettyDir := prettyDirectoryPath(directory)
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      fmt.Sprintf("Create and upload %q?", prettyDir),
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
			archive, err := provisionersdk.Tar(directory, provisionersdk.TemplateArchiveLimit)
			if err != nil {
				return err
			}

			resp, err := client.Upload(cmd.Context(), codersdk.ContentTypeTar, archive)
			if err != nil {
				return err
			}
			spin.Stop()

			job, parameters, err := createValidTemplateVersion(cmd, createValidTemplateVersionArgs{
				Client:        client,
				Organization:  organization,
				Provisioner:   database.ProvisionerType(provisioner),
				FileHash:      resp.Hash,
				ParameterFile: parameterFile,
			})
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Confirm create?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			createReq := codersdk.CreateTemplateRequest{
				Name:                       templateName,
				VersionID:                  job.ID,
				ParameterValues:            parameters,
				MaxTTLMillis:               ptr.Ref(maxTTL.Milliseconds()),
				MinAutostartIntervalMillis: ptr.Ref(minAutostartInterval.Milliseconds()),
			}

			_, err = client.CreateTemplate(cmd.Context(), organization.ID, createReq)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\n"+cliui.Styles.Wrap.Render(
				"The "+cliui.Styles.Keyword.Render(templateName)+" template has been created! "+
					"Developers can provision a workspace with this template using:")+"\n")

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Code.Render(fmt.Sprintf("coder create --template=%q [workspace name]", templateName)))
			_, _ = fmt.Fprintln(cmd.OutOrStdout())

			return nil
		},
	}
	currentDirectory, _ := os.Getwd()
	cmd.Flags().StringVarP(&directory, "directory", "d", currentDirectory, "Specify the directory to create from")
	cmd.Flags().StringVarP(&provisioner, "test.provisioner", "", "terraform", "Customize the provisioner backend")
	cmd.Flags().StringVarP(&parameterFile, "parameter-file", "", "", "Specify a file path with parameter values.")
	cmd.Flags().DurationVarP(&maxTTL, "max-ttl", "", 168*time.Hour, "Specify a maximum TTL for worksapces created from this template.")
	cmd.Flags().DurationVarP(&minAutostartInterval, "min-autostart-interval", "", time.Hour, "Specify a minimum autostart interval for workspaces created from this template.")
	// This is for testing!
	err := cmd.Flags().MarkHidden("test.provisioner")
	if err != nil {
		panic(err)
	}
	cliui.AllowSkipPrompt(cmd)
	return cmd
}

type createValidTemplateVersionArgs struct {
	Client        *codersdk.Client
	Organization  codersdk.Organization
	Provisioner   database.ProvisionerType
	FileHash      string
	ParameterFile string
	// TemplateID is only required if updating a template's active version.
	TemplateID uuid.UUID
}

func createValidTemplateVersion(cmd *cobra.Command, args createValidTemplateVersionArgs, parameters ...codersdk.CreateParameterRequest) (*codersdk.TemplateVersion, []codersdk.CreateParameterRequest, error) {
	before := time.Now()
	client := args.Client

	version, err := client.CreateTemplateVersion(cmd.Context(), args.Organization.ID, codersdk.CreateTemplateVersionRequest{
		StorageMethod:   codersdk.ProvisionerStorageMethodFile,
		StorageSource:   args.FileHash,
		Provisioner:     codersdk.ProvisionerType(args.Provisioner),
		ParameterValues: parameters,
		TemplateID:      args.TemplateID,
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
		missingSchemas := make([]codersdk.ParameterSchema, 0)
		for _, parameterSchema := range parameterSchemas {
			_, ok := valuesBySchemaID[parameterSchema.ID.String()]
			if ok {
				continue
			}
			missingSchemas = append(missingSchemas, parameterSchema)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has required variables! They are scoped to the template, and not viewable after being set.")+"\r\n")

		// parameterMapFromFile can be nil if parameter file is not specified
		var parameterMapFromFile map[string]string
		if args.ParameterFile != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("Attempting to read the variables from the parameter file.")+"\r\n")
			parameterMapFromFile, err = createParameterMapFromFile(args.ParameterFile)
			if err != nil {
				return nil, nil, err
			}
		}
		for _, parameterSchema := range missingSchemas {
			parameterValue, err := getParameterValueFromMapOrInput(cmd, parameterMapFromFile, parameterSchema)
			if err != nil {
				return nil, nil, err
			}
			parameters = append(parameters, codersdk.CreateParameterRequest{
				Name:              parameterSchema.Name,
				SourceValue:       parameterValue,
				SourceScheme:      codersdk.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
		}

		// This recursion is only 1 level deep in practice.
		// The first pass populates the missing parameters, so it does not enter this `if` block again.
		return createValidTemplateVersion(cmd, args, parameters...)
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		return nil, nil, xerrors.New(version.Job.Error)
	}

	resources, err := client.TemplateVersionResources(cmd.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}

	err = cliui.WorkspaceResources(cmd.OutOrStdout(), resources, cliui.WorkspaceResourcesOptions{
		HideAgentState: true,
		HideAccess:     true,
		Title:          "Template Preview",
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("preview template resources: %w", err)
	}

	return &version, parameters, nil
}

// prettyDirectoryPath returns a prettified path when inside the users
// home directory. Falls back to dir if the users home directory cannot
// discerned. This function calls filepath.Clean on the result.
func prettyDirectoryPath(dir string) string {
	dir = filepath.Clean(dir)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return dir
	}
	pretty := dir
	if strings.HasPrefix(pretty, homeDir) {
		pretty = strings.TrimPrefix(pretty, homeDir)
		pretty = "~" + pretty
	}
	return pretty
}
