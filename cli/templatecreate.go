package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd"
)

func templateCreate() *cobra.Command {
	var (
		provisioner     string
		provisionerTags []string
		parameterFile   string
		defaultTTL      time.Duration

		uploadFlags templateUploadFlags
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a template from the current directory or as specified by flag",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			organization, err := CurrentOrganization(cmd, client)
			if err != nil {
				return err
			}

			templateName, err := uploadFlags.templateName(args)
			if err != nil {
				return err
			}

			if utf8.RuneCountInString(templateName) > 31 {
				return xerrors.Errorf("Template name must be less than 32 characters")
			}

			_, err = client.TemplateByName(cmd.Context(), organization.ID, templateName)
			if err == nil {
				return xerrors.Errorf("A template already exists named %q!", templateName)
			}

			// Confirm upload of the directory.
			resp, err := uploadFlags.upload(cmd, client)
			if err != nil {
				return err
			}

			tags, err := ParseProvisionerTags(provisionerTags)
			if err != nil {
				return err
			}

			job, _, err := createValidTemplateVersion(cmd, createValidTemplateVersionArgs{
				Client:          client,
				Organization:    organization,
				Provisioner:     database.ProvisionerType(provisioner),
				FileID:          resp.ID,
				ParameterFile:   parameterFile,
				ProvisionerTags: tags,
			})
			if err != nil {
				return err
			}

			if !uploadFlags.stdin() {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      "Confirm create?",
					IsConfirm: true,
				})
				if err != nil {
					return err
				}
			}

			createReq := codersdk.CreateTemplateRequest{
				Name:             templateName,
				VersionID:        job.ID,
				DefaultTTLMillis: ptr.Ref(defaultTTL.Milliseconds()),
			}

			_, err = client.CreateTemplate(cmd.Context(), organization.ID, createReq)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\n"+cliui.Styles.Wrap.Render(
				"The "+cliui.Styles.Keyword.Render(templateName)+" template has been created at "+cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp))+"! "+
					"Developers can provision a workspace with this template using:")+"\n")

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Code.Render(fmt.Sprintf("coder create --template=%q [workspace name]", templateName)))
			_, _ = fmt.Fprintln(cmd.OutOrStdout())

			return nil
		},
	}
	cmd.Flags().StringVarP(&parameterFile, "parameter-file", "", "", "Specify a file path with parameter values.")
	cmd.Flags().StringArrayVarP(&provisionerTags, "provisioner-tag", "", []string{}, "Specify a set of tags to target provisioner daemons.")
	cmd.Flags().DurationVarP(&defaultTTL, "default-ttl", "", 24*time.Hour, "Specify a default TTL for workspaces created from this template.")
	uploadFlags.register(cmd.Flags())
	cmd.Flags().StringVarP(&provisioner, "test.provisioner", "", "terraform", "Customize the provisioner backend")
	// This is for testing!
	err := cmd.Flags().MarkHidden("test.provisioner")
	if err != nil {
		panic(err)
	}
	cliui.AllowSkipPrompt(cmd)
	return cmd
}

type createValidTemplateVersionArgs struct {
	Name          string
	Client        *codersdk.Client
	Organization  codersdk.Organization
	Provisioner   database.ProvisionerType
	FileID        uuid.UUID
	ParameterFile string
	ValuesFile    string
	// Template is only required if updating a template's active version.
	Template *codersdk.Template
	// ReuseParameters will attempt to reuse params from the Template field
	// before prompting the user. Set to false to always prompt for param
	// values.
	ReuseParameters bool
	ProvisionerTags map[string]string
}

func createValidTemplateVersion(cmd *cobra.Command, args createValidTemplateVersionArgs, parameters ...codersdk.CreateParameterRequest) (*codersdk.TemplateVersion, []codersdk.CreateParameterRequest, error) {
	client := args.Client

	// FIXME(mtojek): I will iterate on CLI experience in the follow-up.
	// see: https://github.com/coder/coder/issues/5980
	variableValues, err := loadVariableValues(args.ValuesFile)
	if err != nil {
		return nil, nil, err
	}

	req := codersdk.CreateTemplateVersionRequest{
		Name:               args.Name,
		StorageMethod:      codersdk.ProvisionerStorageMethodFile,
		FileID:             args.FileID,
		Provisioner:        codersdk.ProvisionerType(args.Provisioner),
		ParameterValues:    parameters,
		ProvisionerTags:    args.ProvisionerTags,
		UserVariableValues: variableValues,
	}
	if args.Template != nil {
		req.TemplateID = args.Template.ID
	}
	version, err := client.CreateTemplateVersion(cmd.Context(), args.Organization.ID, req)
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
		Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
			return client.TemplateVersionLogsAfter(cmd.Context(), version.ID, 0)
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

	// lastParameterValues are pulled from the current active template version if
	// templateID is provided. This allows pulling params from the last
	// version instead of prompting if we are updating template versions.
	lastParameterValues := make(map[string]codersdk.Parameter)
	if args.ReuseParameters && args.Template != nil {
		activeVersion, err := client.TemplateVersion(cmd.Context(), args.Template.ActiveVersionID)
		if err != nil {
			return nil, nil, xerrors.Errorf("Fetch current active template version: %w", err)
		}

		// We don't want to compute the params, we only want to copy from this scope
		values, err := client.Parameters(cmd.Context(), codersdk.ParameterImportJob, activeVersion.Job.ID)
		if err != nil {
			return nil, nil, xerrors.Errorf("Fetch previous version parameters: %w", err)
		}
		for _, value := range values {
			lastParameterValues[value.Name] = value
		}
	}

	if provisionerd.IsMissingParameterError(version.Job.Error) {
		valuesBySchemaID := map[string]codersdk.ComputedParameter{}
		for _, parameterValue := range parameterValues {
			valuesBySchemaID[parameterValue.SchemaID.String()] = parameterValue
		}

		// parameterMapFromFile can be nil if parameter file is not specified
		var parameterMapFromFile map[string]string
		if args.ParameterFile != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("Attempting to read the variables from the parameter file.")+"\r\n")
			parameterMapFromFile, err = createParameterMapFromFile(args.ParameterFile)
			if err != nil {
				return nil, nil, err
			}
		}

		// pulled params come from the last template version
		pulled := make([]string, 0)
		missingSchemas := make([]codersdk.ParameterSchema, 0)
		for _, parameterSchema := range parameterSchemas {
			_, ok := valuesBySchemaID[parameterSchema.ID.String()]
			if ok {
				continue
			}

			// The file values are handled below. So don't handle them here,
			// just check if a value is present in the file.
			_, fileOk := parameterMapFromFile[parameterSchema.Name]
			if inherit, ok := lastParameterValues[parameterSchema.Name]; ok && !fileOk {
				// If the value is not in the param file, and can be pulled from the last template version,
				// then don't mark it as missing.
				parameters = append(parameters, codersdk.CreateParameterRequest{
					CloneID: inherit.ID,
				})
				pulled = append(pulled, fmt.Sprintf("%q", parameterSchema.Name))
				continue
			}

			missingSchemas = append(missingSchemas, parameterSchema)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has required variables! They are scoped to the template, and not viewable after being set."))
		if len(pulled) > 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render(fmt.Sprintf("The following parameter values are being pulled from the latest template version: %s.", strings.Join(pulled, ", "))))
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("Use \"--always-prompt\" flag to change the values."))
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "\r\n")

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

	// Only display the resources on the start transition, to avoid listing them more than once.
	var startResources []codersdk.WorkspaceResource
	for _, r := range resources {
		if r.Transition == codersdk.WorkspaceTransitionStart {
			startResources = append(startResources, r)
		}
	}
	err = cliui.WorkspaceResources(cmd.OutOrStdout(), startResources, cliui.WorkspaceResourcesOptions{
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

func ParseProvisionerTags(rawTags []string) (map[string]string, error) {
	tags := map[string]string{}
	for _, rawTag := range rawTags {
		parts := strings.SplitN(rawTag, "=", 2)
		if len(parts) < 2 {
			return nil, xerrors.Errorf("invalid tag format for %q. must be key=value", rawTag)
		}
		tags[parts[0]] = parts[1]
	}
	return tags, nil
}
