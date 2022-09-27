package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/enterprise/coderd"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	var (
		auditLogging       bool
		browserOnly        bool
		scimAuthHeader     string
		userWorkspaceQuota int
	)
	cmd := agpl.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
		api, err := coderd.New(ctx, &coderd.Options{
			AuditLogging:       auditLogging,
			BrowserOnly:        browserOnly,
			SCIMAPIKey:         []byte(scimAuthHeader),
			UserWorkspaceQuota: userWorkspaceQuota,
			Options:            options,
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

	return cmd
}
