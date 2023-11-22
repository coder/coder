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

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templateCreate() *clibase.Cmd {
	var (
		provisioner          string
		provisionerTags      []string
		variablesFile        string
		variables            []string
		disableEveryone      bool
		requireActiveVersion bool

		defaultTTL           time.Duration
		failureTTL           time.Duration
		dormancyThreshold    time.Duration
		dormancyAutoDeletion time.Duration
		maxTTL               time.Duration

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
			isTemplateSchedulingOptionsSet := failureTTL != 0 || dormancyThreshold != 0 || dormancyAutoDeletion != 0 || maxTTL != 0

			if isTemplateSchedulingOptionsSet || requireActiveVersion {
				if failureTTL != 0 || dormancyThreshold != 0 || dormancyAutoDeletion != 0 {
					// This call can be removed when workspace_actions is no longer experimental
					experiments, exErr := client.Experiments(inv.Context())
					if exErr != nil {
						return xerrors.Errorf("get experiments: %w", exErr)
					}

					if !experiments.Enabled(codersdk.ExperimentWorkspaceActions) {
						return xerrors.Errorf("--failure-ttl, --dormancy-threshold, and --dormancy-auto-deletion are experimental features. Use the workspace_actions CODER_EXPERIMENTS flag to set these configuration values.")
					}
				}

				entitlements, err := client.Entitlements(inv.Context())
				if cerr, ok := codersdk.AsError(err); ok && cerr.StatusCode() == http.StatusNotFound {
					return xerrors.Errorf("your deployment appears to be an AGPL deployment, so you cannot set enterprise-only flags")
				} else if err != nil {
					return xerrors.Errorf("get entitlements: %w", err)
				}

				if isTemplateSchedulingOptionsSet {
					if !entitlements.Features[codersdk.FeatureAdvancedTemplateScheduling].Enabled {
						return xerrors.Errorf("your license is not entitled to use advanced template scheduling, so you cannot set --failure-ttl, --inactivity-ttl, or --max-ttl")
					}
				}

				if requireActiveVersion {
					if !entitlements.Features[codersdk.FeatureAccessControl].Enabled {
						return xerrors.Errorf("your license is not entitled to use enterprise access control, so you cannot set --require-active-version")
					}

					experiments, exErr := client.Experiments(inv.Context())
					if exErr != nil {
						return xerrors.Errorf("get experiments: %w", exErr)
					}

					if !experiments.Enabled(codersdk.ExperimentTemplateUpdatePolicies) {
						return xerrors.Errorf("--require-active-version is an experimental feature, contact an administrator to enable the 'template_update_policies' experiment on your Coder server")
					}
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
				Provisioner:     codersdk.ProvisionerType(provisioner),
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
				Name:                           templateName,
				VersionID:                      job.ID,
				DefaultTTLMillis:               ptr.Ref(defaultTTL.Milliseconds()),
				FailureTTLMillis:               ptr.Ref(failureTTL.Milliseconds()),
				MaxTTLMillis:                   ptr.Ref(maxTTL.Milliseconds()),
				TimeTilDormantMillis:           ptr.Ref(dormancyThreshold.Milliseconds()),
				TimeTilDormantAutoDeleteMillis: ptr.Ref(dormancyAutoDeletion.Milliseconds()),
				DisableEveryoneGroupAccess:     disableEveryone,
				RequireActiveVersion:           requireActiveVersion,
			}

			_, err = client.CreateTemplate(inv.Context(), organization.ID, createReq)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(inv.Stdout, "\n"+pretty.Sprint(cliui.DefaultStyles.Wrap,
				"The "+pretty.Sprint(
					cliui.DefaultStyles.Keyword, templateName)+" template has been created at "+
					pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, time.Now().Format(time.Stamp))+"! "+
					"Developers can provision a workspace with this template using:")+"\n")

			_, _ = fmt.Fprintln(inv.Stdout, "  "+pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("coder create --template=%q [workspace name]", templateName)))
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
			Description: "Specify a default TTL for workspaces created from this template. It is the default time before shutdown - workspaces created from this template default to this value. Maps to \"Default autostop\" in the UI.",
			Default:     "24h",
			Value:       clibase.DurationOf(&defaultTTL),
		},
		{
			Flag:        "failure-ttl",
			Description: "Specify a failure TTL for workspaces created from this template. It is the amount of time after a failed \"start\" build before coder automatically schedules a \"stop\" build to cleanup.This licensed feature's default is 0h (off). Maps to \"Failure cleanup\"in the UI.",
			Default:     "0h",
			Value:       clibase.DurationOf(&failureTTL),
		},
		{
			Flag:        "dormancy-threshold",
			Description: "Specify a duration workspaces may be inactive prior to being moved to the dormant state. This licensed feature's default is 0h (off). Maps to \"Dormancy threshold\" in the UI.",
			Default:     "0h",
			Value:       clibase.DurationOf(&dormancyThreshold),
		},
		{
			Flag:        "dormancy-auto-deletion",
			Description: "Specify a duration workspaces may be in the dormant state prior to being deleted. This licensed feature's default is 0h (off). Maps to \"Dormancy Auto-Deletion\" in the UI.",
			Default:     "0h",
			Value:       clibase.DurationOf(&dormancyAutoDeletion),
		},

		{
			Flag:        "max-ttl",
			Description: "Edit the template maximum time before shutdown - workspaces created from this template must shutdown within the given duration after starting. This is an enterprise-only feature.",
			Value:       clibase.DurationOf(&maxTTL),
		},
		{
			Flag:        "test.provisioner",
			Description: "Customize the provisioner backend.",
			Default:     "terraform",
			Value:       clibase.StringOf(&provisioner),
			Hidden:      true,
		},
		{
			Flag:        "require-active-version",
			Description: "Requires workspace builds to use the active template version. This setting does not apply to template admins. This is an enterprise-only feature.",
			Value:       clibase.BoolOf(&requireActiveVersion),
			Default:     "false",
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
	Provisioner  codersdk.ProvisionerType
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
		Provisioner:        args.Provisioner,
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
		if errors.As(err, &jobErr) && !codersdk.JobIsMissingParameterErrorCode(jobErr.Code) {
			return nil, err
		}
		if err != nil {
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
	prettyDir := dir
	if strings.HasPrefix(prettyDir, homeDir) {
		prettyDir = strings.TrimPrefix(prettyDir, homeDir)
		prettyDir = "~" + prettyDir
	}
	return prettyDir
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
