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
	var (
		name                          string
		displayName                   string
		description                   string
		icon                          string
		defaultTTL                    time.Duration
		maxTTL                        time.Duration
		autostopRequirementDaysOfWeek []string
		autostopRequirementWeeks      int64
		failureTTL                    time.Duration
		inactivityTTL                 time.Duration
		allowUserCancelWorkspaceJobs  bool
		allowUserAutostart            bool
		allowUserAutostop             bool
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
			unsetAutostopRequirementDaysOfWeek := len(autostopRequirementDaysOfWeek) == 1 && autostopRequirementDaysOfWeek[0] == "none"
			requiresEntitlement := (len(autostopRequirementDaysOfWeek) > 0 && !unsetAutostopRequirementDaysOfWeek) ||
				autostopRequirementWeeks > 0 ||
				!allowUserAutostart ||
				!allowUserAutostop ||
				maxTTL != 0 ||
				failureTTL != 0 ||
				inactivityTTL != 0
			if requiresEntitlement {
				entitlements, err := client.Entitlements(inv.Context())
				var sdkErr *codersdk.Error
				if xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound {
					return xerrors.Errorf("your deployment appears to be an AGPL deployment, so you cannot set --max-ttl, --failure-ttl, --inactivityTTL, --allow-user-autostart=false or --allow-user-autostop=false")
				} else if err != nil {
					return xerrors.Errorf("get entitlements: %w", err)
				}

				if !entitlements.Features[codersdk.FeatureAdvancedTemplateScheduling].Enabled {
					return xerrors.Errorf("your license is not entitled to use advanced template scheduling, so you cannot set --max-ttl, --failure-ttl, --inactivityTTL, --allow-user-autostart=false or --allow-user-autostop=false")
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
			if unsetAutostopRequirementDaysOfWeek {
				autostopRequirementDaysOfWeek = []string{}
			}

			// NOTE: coderd will ignore empty fields.
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
				FailureTTLMillis:             failureTTL.Milliseconds(),
				TimeTilDormantMillis:         inactivityTTL.Milliseconds(),
				AllowUserCancelWorkspaceJobs: allowUserCancelWorkspaceJobs,
				AllowUserAutostart:           allowUserAutostart,
				AllowUserAutostop:            allowUserAutostop,
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
			Flag:        "inactivity-ttl",
			Description: "Specify an inactivity TTL for workspaces created from this template. It is the amount of time the workspace is not used before it is be stopped and auto-locked. This includes across multiple builds (e.g. auto-starts and stops). This licensed feature's default is 0h (off). Maps to \"Dormancy threshold\" in the UI.",
			Default:     "0h",
			Value:       clibase.DurationOf(&inactivityTTL),
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
		cliui.SkipPromptOption(),
	}

	return cmd
}
