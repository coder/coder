package cli

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templateEdit() *clibase.Cmd {
	const deprecatedFlagName = "deprecated"
	var (
		name                           string
		displayName                    string
		description                    string
		icon                           string
		defaultTTL                     time.Duration
		maxTTL                         time.Duration
		autostopRequirementDaysOfWeek  []string
		autostopRequirementWeeks       int64
		autostartRequirementDaysOfWeek []string
		failureTTL                     time.Duration
		dormancyThreshold              time.Duration
		dormancyAutoDeletion           time.Duration
		allowUserCancelWorkspaceJobs   bool
		allowUserAutostart             bool
		allowUserAutostop              bool
		requireActiveVersion           bool
		deprecationMessage             string
	)
	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use: "edit <template>",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Short: "Edit the metadata of a template by name.",
		Handler: func(inv *clibase.Invocation) error {
			// This clause can be removed when workspace_actions is no longer experimental
			if failureTTL != 0 || dormancyThreshold != 0 || dormancyAutoDeletion != 0 {
				experiments, exErr := client.Experiments(inv.Context())
				if exErr != nil {
					return xerrors.Errorf("get experiments: %w", exErr)
				}

				if !experiments.Enabled(codersdk.ExperimentWorkspaceActions) {
					return xerrors.Errorf("--failure-ttl, --dormancy-threshold, and --dormancy-auto-deletion are experimental features. Use the workspace_actions CODER_EXPERIMENTS flag to set these configuration values.")
				}
			}

			unsetAutostopRequirementDaysOfWeek := len(autostopRequirementDaysOfWeek) == 1 && autostopRequirementDaysOfWeek[0] == "none"
			requiresScheduling := (len(autostopRequirementDaysOfWeek) > 0 && !unsetAutostopRequirementDaysOfWeek) ||
				autostopRequirementWeeks > 0 ||
				!allowUserAutostart ||
				!allowUserAutostop ||
				maxTTL != 0 ||
				failureTTL != 0 ||
				dormancyThreshold != 0 ||
				dormancyAutoDeletion != 0 ||
				len(autostartRequirementDaysOfWeek) > 0

			requiresEntitlement := requiresScheduling || requireActiveVersion
			if requiresEntitlement {
				entitlements, err := client.Entitlements(inv.Context())
				if cerr, ok := codersdk.AsError(err); ok && cerr.StatusCode() == http.StatusNotFound {
					return xerrors.Errorf("your deployment appears to be an AGPL deployment, so you cannot set enterprise-only flags")
				} else if err != nil {
					return xerrors.Errorf("get entitlements: %w", err)
				}

				if requiresScheduling && !entitlements.Features[codersdk.FeatureAdvancedTemplateScheduling].Enabled {
					return xerrors.Errorf("your license is not entitled to use advanced template scheduling, so you cannot set --max-ttl, --failure-ttl, --inactivityTTL, --allow-user-autostart=false or --allow-user-autostop=false")
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
				return xerrors.Errorf("get current organization: %w", err)
			}
			template, err := client.TemplateByName(inv.Context(), organization.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace template: %w", err)
			}

			// Copy the default value if the list is empty, or if the user
			// specified the "none" value clear the list.
			if len(autostopRequirementDaysOfWeek) == 0 {
				autostopRequirementDaysOfWeek = template.AutostopRequirement.DaysOfWeek
			}
			if len(autostartRequirementDaysOfWeek) == 1 && autostartRequirementDaysOfWeek[0] == "all" {
				// Set it to every day of the week
				autostartRequirementDaysOfWeek = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
			} else if len(autostartRequirementDaysOfWeek) == 0 {
				autostartRequirementDaysOfWeek = template.AutostartRequirement.DaysOfWeek
			}
			if unsetAutostopRequirementDaysOfWeek {
				autostopRequirementDaysOfWeek = []string{}
			}
			if failureTTL == 0 {
				failureTTL = time.Duration(template.FailureTTLMillis) * time.Millisecond
			}
			if dormancyThreshold == 0 {
				dormancyThreshold = time.Duration(template.TimeTilDormantMillis) * time.Millisecond
			}
			if dormancyAutoDeletion == 0 {
				dormancyAutoDeletion = time.Duration(template.TimeTilDormantAutoDeleteMillis) * time.Millisecond
			}

			// Default values
			if !userSetOption(inv, "description") {
				description = template.Description
			}

			if !userSetOption(inv, "icon") {
				icon = template.Icon
			}

			if !userSetOption(inv, "display-name") {
				displayName = template.DisplayName
			}

			var deprecated *string
			if !userSetOption(inv, "deprecated") {
				deprecated = &deprecationMessage
			}

			req := codersdk.UpdateTemplateMeta{
				Name:             name,
				DisplayName:      displayName,
				Description:      description,
				Icon:             icon,
				DefaultTTLMillis: defaultTTL.Milliseconds(),
				MaxTTLMillis:     maxTTL.Milliseconds(),
				AutostopRequirement: &codersdk.TemplateAutostopRequirement{
					DaysOfWeek: autostopRequirementDaysOfWeek,
					Weeks:      autostopRequirementWeeks,
				},
				AutostartRequirement: &codersdk.TemplateAutostartRequirement{
					DaysOfWeek: autostartRequirementDaysOfWeek,
				},
				FailureTTLMillis:               failureTTL.Milliseconds(),
				TimeTilDormantMillis:           dormancyThreshold.Milliseconds(),
				TimeTilDormantAutoDeleteMillis: dormancyAutoDeletion.Milliseconds(),
				AllowUserCancelWorkspaceJobs:   allowUserCancelWorkspaceJobs,
				AllowUserAutostart:             allowUserAutostart,
				AllowUserAutostop:              allowUserAutostop,
				RequireActiveVersion:           requireActiveVersion,
				DeprecationMessage:             deprecated,
			}

			_, err = client.UpdateTemplateMeta(inv.Context(), template.ID, req)
			if err != nil {
				return xerrors.Errorf("update template metadata: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Updated template metadata at %s!\n", pretty.Sprint(cliui.DefaultStyles.DateTimeStamp, time.Now().Format(time.Stamp)))
			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "name",
			Description: "Edit the template name.",
			Value:       clibase.StringOf(&name),
		},
		{
			Flag:        "display-name",
			Description: "Edit the template display name.",
			Value:       clibase.StringOf(&displayName),
		},
		{
			Flag:        "description",
			Description: "Edit the template description.",
			Value:       clibase.StringOf(&description),
		},
		{
			Name:        deprecatedFlagName,
			Flag:        "deprecated",
			Description: "Sets the template as deprecated. Must be a message explaining why the template is deprecated.",
			Value:       clibase.StringOf(&deprecationMessage),
		},
		{
			Flag:        "icon",
			Description: "Edit the template icon path.",
			Value:       clibase.StringOf(&icon),
		},
		{
			Flag:        "default-ttl",
			Description: "Edit the template default time before shutdown - workspaces created from this template default to this value. Maps to \"Default autostop\" in the UI.",
			Value:       clibase.DurationOf(&defaultTTL),
		},
		{
			Flag:        "max-ttl",
			Description: "Edit the template maximum time before shutdown - workspaces created from this template must shutdown within the given duration after starting, regardless of user activity. This is an enterprise-only feature. Maps to \"Max lifetime\" in the UI.",
			Value:       clibase.DurationOf(&maxTTL),
		},
		{
			Flag: "autostart-requirement-weekdays",
			// workspaces created from this template must be restarted on the given weekdays. To unset this value for the template (and disable the autostop requirement for the template), pass 'none'.
			Description: "Edit the template autostart requirement weekdays - workspaces created from this template can only autostart on the given weekdays. To unset this value for the template (and allow autostart on all days), pass 'all'.",
			Value: clibase.Validate(clibase.StringArrayOf(&autostartRequirementDaysOfWeek), func(value *clibase.StringArray) error {
				v := value.GetSlice()
				if len(v) == 1 && v[0] == "all" {
					return nil
				}
				_, err := codersdk.WeekdaysToBitmap(v)
				if err != nil {
					return xerrors.Errorf("invalid autostart requirement days of week %q: %w", strings.Join(v, ","), err)
				}
				return nil
			}),
		},
		{
			Flag:        "autostop-requirement-weekdays",
			Description: "Edit the template autostop requirement weekdays - workspaces created from this template must be restarted on the given weekdays. To unset this value for the template (and disable the autostop requirement for the template), pass 'none'.",
			// TODO(@dean): unhide when we delete max_ttl
			Hidden: true,
			Value: clibase.Validate(clibase.StringArrayOf(&autostopRequirementDaysOfWeek), func(value *clibase.StringArray) error {
				v := value.GetSlice()
				if len(v) == 1 && v[0] == "none" {
					return nil
				}
				_, err := codersdk.WeekdaysToBitmap(v)
				if err != nil {
					return xerrors.Errorf("invalid autostop requirement days of week %q: %w", strings.Join(v, ","), err)
				}
				return nil
			}),
		},
		{
			Flag:        "autostop-requirement-weeks",
			Description: "Edit the template autostop requirement weeks - workspaces created from this template must be restarted on an n-weekly basis.",
			// TODO(@dean): unhide when we delete max_ttl
			Hidden: true,
			Value:  clibase.Int64Of(&autostopRequirementWeeks),
		},
		{
			Flag:        "failure-ttl",
			Description: "Specify a failure TTL for workspaces created from this template. It is the amount of time after a failed \"start\" build before coder automatically schedules a \"stop\" build to cleanup.This licensed feature's default is 0h (off). Maps to \"Failure cleanup\" in the UI.",
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
			Flag:        "allow-user-cancel-workspace-jobs",
			Description: "Allow users to cancel in-progress workspace jobs.",
			Default:     "true",
			Value:       clibase.BoolOf(&allowUserCancelWorkspaceJobs),
		},
		{
			Flag:        "allow-user-autostart",
			Description: "Allow users to configure autostart for workspaces on this template. This can only be disabled in enterprise.",
			Default:     "true",
			Value:       clibase.BoolOf(&allowUserAutostart),
		},
		{
			Flag:        "allow-user-autostop",
			Description: "Allow users to customize the autostop TTL for workspaces on this template. This can only be disabled in enterprise.",
			Default:     "true",
			Value:       clibase.BoolOf(&allowUserAutostop),
		},
		{
			Flag:        "require-active-version",
			Description: "Requires workspace builds to use the active template version. This setting does not apply to template admins. This is an enterprise-only feature.",
			Value:       clibase.BoolOf(&requireActiveVersion),
			Default:     "false",
		},
		cliui.SkipPromptOption(),
	}

	return cmd
}
