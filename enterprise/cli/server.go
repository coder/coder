package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/enterprise/coderd"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	dflags := deployment.Flags()
	cmd := agpl.Server(dflags, func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
		options.DeploymentFlags = &dflags
		o := &coderd.Options{
			AuditLogging:       dflags.AuditLogging.Value,
			BrowserOnly:        dflags.BrowserOnly.Value,
			SCIMAPIKey:         []byte(dflags.SCIMAuthHeader.Value),
			UserWorkspaceQuota: dflags.UserWorkspaceQuota.Value,
			RBACEnabled:        true,
			Options:            options,
		}
		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, err
		}
		return api.AGPL, nil
	})

	// append enterprise description to flags
	enterpriseOnly := cliui.Styles.Keyword.Render(" This is an Enterprise feature. Contact sales@coder.com for licensing")
	dflags.AuditLogging.Description += enterpriseOnly
	dflags.BrowserOnly.Description += enterpriseOnly
	dflags.SCIMAuthHeader.Description += enterpriseOnly
	dflags.UserWorkspaceQuota.Description += enterpriseOnly

	deployment.BoolFlag(cmd.Flags(), &dflags.AuditLogging)
	deployment.BoolFlag(cmd.Flags(), &dflags.BrowserOnly)
	deployment.StringFlag(cmd.Flags(), &dflags.SCIMAuthHeader)
	deployment.IntFlag(cmd.Flags(), &dflags.UserWorkspaceQuota)

	return cmd
}
