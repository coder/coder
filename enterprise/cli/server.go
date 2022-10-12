package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/enterprise/coderd"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	dflags := deployment.Flags()
	cmd := agpl.Server(dflags, func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
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

	deployment.AttachFlags(cmd.Flags(), dflags, true)

	return cmd
}
