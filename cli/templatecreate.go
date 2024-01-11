package cli

import (
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

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
		commandLineVariables []string
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
		Short: "DEPRECATED: Create a template from the current directory or as specified by flag",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(0, 1),
			cliui.DeprecationWarning(
				"Use `coder templates push` command for creating and updating templates. \n"+
					"Use `coder templates edit` command for editing template settings. ",
			),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			isTemplateSchedulingOptionsSet := failureTTL != 0 || dormancyThreshold != 0 || dormancyAutoDeletion != 0 || maxTTL != 0

			if isTemplateSchedulingOptionsSet || requireActiveVersion {
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

			userVariableValues, err := ParseUserVariableValues(
				variablesFile,
				commandLineVariables)
			if err != nil {
				return err
			}

			job, err := createValidTemplateVersion(inv, createValidTemplateVersionArgs{
				Message:            message,
				Client:             client,
				Organization:       organization,
				Provisioner:        codersdk.ProvisionerType(provisioner),
				FileID:             resp.ID,
				ProvisionerTags:    tags,
				UserVariableValues: userVariableValues,
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
			Value:       clibase.StringArrayOf(&commandLineVariables),
		},
		{
			Flag:        "var",
			Description: "Alias of --variable.",
			Value:       clibase.StringArrayOf(&commandLineVariables),
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
