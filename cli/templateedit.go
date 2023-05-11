package cli

import (
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) templateEdit() *clibase.Cmd {
	var (
		name                         string
		displayName                  string
		description                  string
		icon                         string
		defaultTTL                   time.Duration
		maxTTL                       time.Duration
		failureTTL                   time.Duration
		inactivityTTL                time.Duration
		allowUserCancelWorkspaceJobs bool
		allowUserAutostart           bool
		allowUserAutostop            bool
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
			if failureTTL != 0 || inactivityTTL != 0 {
				experiments, exErr := client.Experiments(inv.Context())
				if exErr != nil {
					return xerrors.Errorf("get experiments: %w", exErr)
				}

				if !experiments.Enabled(codersdk.ExperimentWorkspaceActions) {
					return xerrors.Errorf("--failure-ttl and --inactivityTTL are experimental features. Use the workspace_actions CODER_EXPERIMENTS flag to set these configuration values.")
				}
			}

			if maxTTL != 0 || !allowUserAutostart || !allowUserAutostop || failureTTL != 0 || inactivityTTL != 0 {
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

			// NOTE: coderd will ignore empty fields.
			req := codersdk.UpdateTemplateMeta{
				Name:                         name,
				DisplayName:                  displayName,
				Description:                  description,
				Icon:                         icon,
				DefaultTTLMillis:             defaultTTL.Milliseconds(),
				MaxTTLMillis:                 maxTTL.Milliseconds(),
				FailureTTLMillis:             failureTTL.Milliseconds(),
				InactivityTTLMillis:          inactivityTTL.Milliseconds(),
				AllowUserCancelWorkspaceJobs: allowUserCancelWorkspaceJobs,
				AllowUserAutostart:           allowUserAutostart,
				AllowUserAutostop:            allowUserAutostop,
			}

			_, err = client.UpdateTemplateMeta(inv.Context(), template.ID, req)
			if err != nil {
				return xerrors.Errorf("update template metadata: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Updated template metadata at %s!\n", cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
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
			Description: "Edit the template default time before shutdown - workspaces created from this template default to this value.",
			Value:       clibase.DurationOf(&defaultTTL),
		},
		{
			Flag:        "max-ttl",
			Description: "Edit the template maximum time before shutdown - workspaces created from this template must shutdown within the given duration after starting. This is an enterprise-only feature.",
			Value:       clibase.DurationOf(&maxTTL),
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
