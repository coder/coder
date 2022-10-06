package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/enterprise/coderd"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	var (
		auditLogging                    bool
		browserOnly                     bool
		scimAuthHeader                  string
		userWorkspaceQuota              int
		allowedApplicationSharingLevels []string
	)
	cmd := agpl.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
		appSharingLevels := make([]database.AppSharingLevel, len(allowedApplicationSharingLevels))
		for i, val := range allowedApplicationSharingLevels {
			appSharingLevels[i] = database.AppSharingLevel(val)
		}

		api, err := coderd.New(ctx, &coderd.Options{
			AuditLogging:                    auditLogging,
			BrowserOnly:                     browserOnly,
			SCIMAPIKey:                      []byte(scimAuthHeader),
			UserWorkspaceQuota:              userWorkspaceQuota,
			AllowedApplicationSharingLevels: appSharingLevels,
			Options:                         options,
		})
		if err != nil {
			return nil, err
		}
		return api.AGPL, nil
	})

	enterpriseOnly := cliui.Styles.Keyword.Render("This is an Enterprise feature. Contact sales@coder.com for licensing")
	cliflag.BoolVarP(cmd.Flags(), &auditLogging, "audit-logging", "", "CODER_AUDIT_LOGGING", true,
		"Specifies whether audit logging is enabled. "+enterpriseOnly)
	cliflag.BoolVarP(cmd.Flags(), &browserOnly, "browser-only", "", "CODER_BROWSER_ONLY", false,
		"Whether Coder only allows connections to workspaces via the browser. "+enterpriseOnly)
	cliflag.StringVarP(cmd.Flags(), &scimAuthHeader, "scim-auth-header", "", "CODER_SCIM_API_KEY", "",
		"Enables SCIM and sets the authentication header for the built-in SCIM server. New users are automatically created with OIDC authentication. "+enterpriseOnly)
	cliflag.IntVarP(cmd.Flags(), &userWorkspaceQuota, "user-workspace-quota", "", "CODER_USER_WORKSPACE_QUOTA", 0,
		"A positive number applies a limit on how many workspaces each user can create. "+enterpriseOnly)
	cliflag.StringArrayVarP(cmd.Flags(), &allowedApplicationSharingLevels, "permitted-app-sharing-levels", "", "CODER_PERMITTED_APP_SHARING_LEVELS", []string{"owner"},
		`Specifies the application sharing levels that are available site-wide. Available values are "owner", "template", "authenticated", "public". Multiple values can be specified, comma separated. `+enterpriseOnly)

	return cmd
}
