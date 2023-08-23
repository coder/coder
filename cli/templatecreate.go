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

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd"
)

func (r *RootCmd) templateCreate() *clibase.Cmd {
	var (
		provisioner     string
		provisionerTags []string
		variablesFile   string
		variables       []string
		disableEveryone bool
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

			err = uploadFlags.checkForLockfile(inv)
			if err != nil {
				return xerrors.Errorf("check for lockfile: %w", err)
			}

			message := uploadFlags.templateMessage(inv)

			// Confirm upload of the directory.
			resp, err := uploadFlags.upload(inv, client)
			if err != nil {
				return err
			}

			tags, err := ParseProvisionerTags(provisionerTags)
			if err != nil {
				return err
			}

			job, err := createValidTemplateVersion(inv, createValidTemplateVersionArgs{
				Message:         message,
				Client:          client,
				Organization:    organization,
				Provisioner:     database.ProvisionerType(provisioner),
				FileID:          resp.ID,
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
				Name:                       templateName,
				VersionID:                  job.ID,
				DefaultTTLMillis:           ptr.Ref(defaultTTL.Milliseconds()),
				FailureTTLMillis:           ptr.Ref(failureTTL.Milliseconds()),
				InactivityTTLMillis:        ptr.Ref(inactivityTTL.Milliseconds()),
				DisableEveryoneGroupAccess: disableEveryone,
			}

			_, err = client.CreateTemplate(inv.Context(), organization.ID, createReq)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(inv.Stdout, "\n"+cliui.DefaultStyles.Wrap.Render(
				"The "+cliui.DefaultStyles.Keyword.Render(templateName)+" template has been created at "+cliui.DefaultStyles.DateTimeStamp.Render(time.Now().Format(time.Stamp))+"! "+
					"Developers can provision a workspace with this template using:")+"\n")

			_, _ = fmt.Fprintln(inv.Stdout, "  "+cliui.DefaultStyles.Code.Render(fmt.Sprintf("coder create --template=%q [workspace name]", templateName)))
			_, _ = fmt.Fprintln(inv.Stdout)

			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag: "private",
			Description: "Disable the default behavior of granting template access to the 'everyone' group. " +
				"The template permissions must be updated to allow non-admin users to use this template.",
			Value: clibase.BoolOf(&disableEveryone),
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
			Flag:        "var",
			Description: "Alias of --variable.",
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
		{
			Flag:        "test.provisioner",
			Description: "Customize the provisioner backend.",
			Default:     "terraform",
			Value:       clibase.StringOf(&provisioner),
			Hidden:      true,
		},
		cliui.SkipPromptOption(),
	}
	cmd.Options = append(cmd.Options, uploadFlags.options()...)
	return cmd
}

type createValidTemplateVersionArgs struct {
	Name         string
	Message      string
	Client       *codersdk.Client
	Organization codersdk.Organization
	Provisioner  database.ProvisionerType
	FileID       uuid.UUID

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

func createValidTemplateVersion(inv *clibase.Invocation, args createValidTemplateVersionArgs) (*codersdk.TemplateVersion, error) {
	client := args.Client

	variableValues, err := loadVariableValuesFromFile(args.VariablesFile)
	if err != nil {
		return nil, err
	}

	variableValuesFromKeyValues, err := loadVariableValuesFromOptions(args.Variables)
	if err != nil {
		return nil, err
	}
	variableValues = append(variableValues, variableValuesFromKeyValues...)

	req := codersdk.CreateTemplateVersionRequest{
		Name:               args.Name,
		Message:            args.Message,
		StorageMethod:      codersdk.ProvisionerStorageMethodFile,
		FileID:             args.FileID,
		Provisioner:        codersdk.ProvisionerType(args.Provisioner),
		ProvisionerTags:    args.ProvisionerTags,
		UserVariableValues: variableValues,
	}
	if args.Template != nil {
		req.TemplateID = args.Template.ID
	}
	version, err := client.CreateTemplateVersion(inv.Context(), args.Organization.ID, req)
	if err != nil {
		return nil, err
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
			return nil, err
		}
	}
	version, err = client.TemplateVersion(inv.Context(), version.ID)
	if err != nil {
		return nil, err
	}

	if version.Job.Status != codersdk.ProvisionerJobSucceeded {
		return nil, xerrors.New(version.Job.Error)
	}

	resources, err := client.TemplateVersionResources(inv.Context(), version.ID)
	if err != nil {
		return nil, err
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
		return nil, xerrors.Errorf("preview template resources: %w", err)
	}

	return &version, nil
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
