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
		auditLogging bool
		browserOnly  bool
	)
	cmd := agpl.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
		api, err := coderd.New(ctx, &coderd.Options{
			AuditLogging: auditLogging,
			BrowserOnly:  browserOnly,
			Options:      options,
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

	return cmd
}
