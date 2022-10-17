package cli

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/url"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/types/key"

	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/enterprise/coderd"
	"github.com/coder/coder/tailnet"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	dflags := deployment.Flags()
	cmd := agpl.Server(dflags, func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		if dflags.DerpServerRelayAddress.Value != "" {
			_, err := url.Parse(dflags.DerpServerRelayAddress.Value)
			if err != nil {
				return nil, nil, xerrors.Errorf("derp-server-relay-address must be a valid HTTP URL: %w", err)
			}
		}

		options.DERPServer = derp.NewServer(key.NewNode(), tailnet.Logger(options.Logger.Named("derp")))
		meshKey, err := options.Database.GetDERPMeshKey(ctx)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return nil, nil, xerrors.Errorf("get mesh key: %w", err)
			}
			meshKey, err = cryptorand.String(32)
			if err != nil {
				return nil, nil, xerrors.Errorf("generate mesh key: %w", err)
			}
			err = options.Database.InsertDERPMeshKey(ctx, meshKey)
			if err != nil {
				return nil, nil, xerrors.Errorf("insert mesh key: %w", err)
			}
		}
		options.DERPServer.SetMeshKey(meshKey)

		o := &coderd.Options{
			AuditLogging:           dflags.AuditLogging.Value,
			BrowserOnly:            dflags.BrowserOnly.Value,
			SCIMAPIKey:             []byte(dflags.SCIMAuthHeader.Value),
			UserWorkspaceQuota:     dflags.UserWorkspaceQuota.Value,
			RBAC:                   true,
			DERPServerRelayAddress: dflags.DerpServerRelayAddress.Value,
			DERPServerRegionID:     dflags.DerpServerRegionID.Value,

			Options: options,
		}
		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, nil, err
		}
		return api.AGPL, api, nil
	})

	deployment.AttachFlags(cmd.Flags(), dflags, true)
	return cmd
}
