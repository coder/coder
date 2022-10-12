package cli

import (
	"context"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"cdr.dev/slog"

	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/enterprise/coderd"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	dflags := deployment.Flags()
	cmd := agpl.Server(dflags, func(ctx context.Context, cfg config.Root, options *agplcoderd.Options) (*agplcoderd.API, error) {
		replicaIDRaw, err := cfg.ReplicaID().Read()
		if err != nil {
			replicaIDRaw = uuid.NewString()
		}
		replicaID, err := uuid.Parse(replicaIDRaw)
		if err != nil {
			options.Logger.Warn(ctx, "failed to parse replica id", slog.Error(err), slog.F("replica_id", replicaIDRaw))
			replicaID = uuid.New()
		}
		o := &coderd.Options{
			AuditLogging:       dflags.AuditLogging.Value,
			BrowserOnly:        dflags.BrowserOnly.Value,
			SCIMAPIKey:         []byte(dflags.SCIMAuthHeader.Value),
			UserWorkspaceQuota: dflags.UserWorkspaceQuota.Value,
			RBAC:               true,
			HighAvailability:   dflags.HighAvailability.Value,

			ReplicaID:              replicaID,
			DERPServerRelayAddress: dflags.DerpServerRelayAddress.Value,
			DERPServerRegionID:     dflags.DerpServerRegionID.Value,

			Options: options,
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
