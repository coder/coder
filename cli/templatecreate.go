package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionerd"
)

func (r *RootCmd) templateCreate() *clibase.Cmd {
	var (
		provisioner     string
		provisionerTags []string
		parameterFile   string
		variablesFile   string
		variables       []string
		defaultTTL      time.Duration
		failureTTL      time.Duration
		inactivityTTL   time.Duration

		uploadFlags templateUploadFlags
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "create [name]",
		Short: "Create a template from the current directory or as specified by flag",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			if failureTTL != 0 || inactivityTTL != 0 {
				// This call can be removed when workspace_actions is no longer experimental
				experiments, exErr := client.Experiments(inv.Context())
				if exErr != nil {
					return xerrors.Errorf("get experiments: %w", exErr)
				}

				if !experiments.Enabled(codersdk.ExperimentWorkspaceActions) {
					return xerrors.Errorf("--failure-ttl and --inactivityTTL are experimental features. Use the workspace_actions CODER_EXPERIMENTS flag to set these configuration values.")
				}

				entitlements, err := client.Entitlements(inv.Context())
				var sdkErr *codersdk.Error
				if xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
					return xerrors.Errorf("your deployment appears to be an AGPL deployment, so you cannot set --failure-ttl or --inactivityTTL")
				} else if err != nil {
					return xerrors.Errorf("get entitlements: %w", err)
				}

				if !entitlements.Features[codersdk.FeatureAdvancedTemplateScheduling].Enabled {
					return xerrors.Errorf("your license is not entitled to use advanced template scheduling, so you cannot set --failure-ttl or --inactivityTTL")
				}
			}

			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return err
			}

			templateName, err := uploadFlags.templateName(inv.Args)
			if err != nil {
				return err
			}

			if utf8.RuneCountInString(templateName) > 31 {
				return xerrors.Errorf("Template name must be less than 32 characters")
			}

			_, err = client.TemplateByName(inv.Context(), organization.ID, templateName)
			if err == nil {
				return xerrors.Errorf("A template already exists named %q!", templateName)
			}

			// Confirm upload of the directory.
			resp, err := uploadFlags.upload(inv, client)
			if err != nil {
				return err
			}

			tags, err := ParseProvisionerTags(provisionerTags)
			if err != nil {
				return err
			}

			job, _, err := createValidTemplateVersion(inv, createValidTemplateVersionArgs{
				Client:          client,
				Organization:    organization,
				Provisioner:     database.ProvisionerType(provisioner),
				FileID:          resp.ID,
				ParameterFile:   parameterFile,
				ProvisionerTags: tags,
				VariablesFile:   variablesFile,
				Variables:       variables,
			})
			if err != nil {
				return err
			}

			if !uploadFlags.stdin() {
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:      "Confirm create?",
					IsConfirm: true,
				})
				if err != nil {
					return err
				}
			}

			createReq := codersdk.CreateTemplateRequest{
				Name:                templateName,
				VersionID:           job.ID,
				DefaultTTLMillis:    ptr.Ref(defaultTTL.Milliseconds()),
				FailureTTLMillis:    ptr.Ref(failureTTL.Milliseconds()),
				InactivityTTLMillis: ptr.Ref(inactivityTTL.Milliseconds()),
			}

			_, err = client.CreateTemplate(inv.Context(), organization.ID, createReq)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(inv.Stdout, "\n"+cliui.Styles.Wrap.Render(
				"The "+cliui.Styles.Keyword.Render(templateName)+" template has been created at "+cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp))+"! "+
					"Developers can provision a workspace with this template using:")+"\n")

			_, _ = fmt.Fprintln(inv.Stdout, "  "+cliui.Styles.Code.Render(fmt.Sprintf("coder create --template=%q [workspace name]", templateName)))
			_, _ = fmt.Fprintln(inv.Stdout)

			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        "parameter-file",
			Description: "Specify a file path with parameter values.",
			Value:       clibase.StringOf(&parameterFile),
		},
		{
			Flag:        "variables-file",
			Description: "Specify a file path with values for Terraform-managed variables.",
			Value:       clibase.StringOf(&variablesFile),
		},
		{
			Flag:        "variable",
			Description: "Specify a set of values for Terraform-managed variables.",
			Value:       clibase.StringArrayOf(&variables),
		},
		{
			Flag:        "provisioner-tag",
			Description: "Specify a set of tags to target provisioner daemons.",
			Value:       clibase.StringArrayOf(&provisionerTags),
		},
		{
			Flag:        "default-ttl",
			Description: "Specify a default TTL for workspaces created from this template.",
			Default:     "24h",
			Value:       clibase.DurationOf(&defaultTTL),
		},
		{
			Flag:        "failure-ttl",
			Description: "Specify a failure TTL for workspaces created from this template. This licensed feature's default is 0h (off).",
			Default:     "0h",
			Value:       clibase.DurationOf(&failureTTL),
		},
		{
			Flag:        "inactivity-ttl",
			Description: "Specify an inactivity TTL for workspaces created from this template. This licensed feature's default is 0h (off).",
			Default:     "0h",
			Value:       clibase.DurationOf(&inactivityTTL),
		},
		uploadFlags.option(),
		{
			Flag:        "test.provisioner",
			Description: "Customize the provisioner backend.",
			Default:     "terraform",
			Value:       clibase.StringOf(&provisioner),
			Hidden:      true,
		},
		cliui.SkipPromptOption(),
	}
	return cmd
}

type createValidTemplateVersionArgs struct {
	Name          string
	Client        *codersdk.Client
	Organization  codersdk.Organization
	Provisioner   database.ProvisionerType
	FileID        uuid.UUID
	ParameterFile string

	VariablesFile string
	Variables     []string

	// Template is only required if updating a template's active version.
	Template *codersdk.Template
	// ReuseParameters will attempt to reuse params from the Template field
	// before prompting the user. Set to false to always prompt for param
	// values.
	ReuseParameters bool
	ProvisionerTags map[string]string
}

func createValidTemplateVersion(inv *clibase.Invocation, args createValidTemplateVersionArgs, parameters ...codersdk.CreateParameterRequest) (*codersdk.TemplateVersion, []codersdk.CreateParameterRequest, error) {
	client := args.Client

	variableValues, err := loadVariableValuesFromFile(args.VariablesFile)
	if err != nil {
		return nil, nil, err
	}

	variableValuesFromKeyValues, err := loadVariableValuesFromOptions(args.Variables)
	if err != nil {
		return nil, nil, err
	}
	variableValues = append(variableValues, variableValuesFromKeyValues...)

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
	version, err := client.CreateTemplateVersion(inv.Context(), args.Organization.ID, req)
	if err != nil {
		return nil, nil, err
	}

	err = cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
		Fetch: func() (codersdk.ProvisionerJob, error) {
			version, err := client.TemplateVersion(inv.Context(), version.ID)
			return version.Job, err
		},
		Cancel: func() error {
			return client.CancelTemplateVersion(inv.Context(), version.ID)
		},
		Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
			return client.TemplateVersionLogsAfter(inv.Context(), version.ID, 0)
		},
	})
	if err != nil {
		var jobErr *cliui.ProvisionerJobError
		if errors.As(err, &jobErr) && !provisionerd.IsMissingParameterErrorCode(string(jobErr.Code)) {
			return nil, nil, err
		}
	}
	version, err = client.TemplateVersion(inv.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterSchemas, err := client.TemplateVersionSchema(inv.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}
	parameterValues, err := client.TemplateVersionParameters(inv.Context(), version.ID)
	if err != nil {
		return nil, nil, err
	}

	// lastParameterValues are pulled from the current active template version if
	// templateID is provided. This allows pulling params from the last
	// version instead of prompting if we are updating template versions.
	lastParameterValues := make(map[string]codersdk.Parameter)
	if args.ReuseParameters && args.Template != nil {
		activeVersion, err := client.TemplateVersion(inv.Context(), args.Template.ActiveVersionID)
		if err != nil {
			return nil, nil, xerrors.Errorf("Fetch current active template version: %w", err)
		}

		// We don't want to compute the params, we only want to copy from this scope
		values, err := client.Parameters(inv.Context(), codersdk.ParameterImportJob, activeVersion.Job.ID)
		if err != nil {
			return nil, nil, xerrors.Errorf("Fetch previous version parameters: %w", err)
		}
		for _, value := range values {
			lastParameterValues[value.Name] = value
		}
	}

	if provisionerd.IsMissingParameterErrorCode(string(version.Job.ErrorCode)) {
		valuesBySchemaID := map[string]codersdk.ComputedParameter{}
		for _, parameterValue := range parameterValues {
			valuesBySchemaID[parameterValue.SchemaID.String()] = parameterValue
		}

		// parameterMapFromFile can be nil if parameter file is not specified
		var parameterMapFromFile map[string]string
		if args.ParameterFile != "" {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("Attempting to read the variables from the parameter file.")+"\r\n")
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
		_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("This template has required variables! They are scoped to the template, and not viewable after being set."))
		if len(pulled) > 0 {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render(fmt.Sprintf("The following parameter values are being pulled from the latest template version: %s.", strings.Join(pulled, ", "))))
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("Use \"--always-prompt\" flag to change the values."))
		}
		_, _ = fmt.Fprint(inv.Stdout, "\r\n")

		for _, parameterSchema := range missingSchemas {
			parameterValue, err := getParameterValueFromMapOrInput(inv, parameterMapFromFile, parameterSchema)
			if err != nil {
				return nil, nil, err
			}
			parameters = append(parameters, codersdk.CreateParameterRequest{
				Name:              parameterSchema.Name,
				SourceValue:       parameterValue,
				SourceScheme:      codersdk.ParameterSourceSchemeData,
				DestinationScheme: parameterSchema.DefaultDestinationScheme,
			})
			_, _ = fmt.Fprintln(inv.Stdout)
		}

		// This recursion is only 1 level deep in practice.
		// The first pass populates the missing parameters, so it does not enter this `if` block again.
		return createValidTemplateVersion(inv, args, parameters...)
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		return nil, nil, xerrors.New(version.Job.Error)
	}

	resources, err := client.TemplateVersionResources(inv.Context(), version.ID)
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
	err = cliui.WorkspaceResources(inv.Stdout, startResources, cliui.WorkspaceResourcesOptions{
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
