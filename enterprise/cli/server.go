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
	vip := deployment.DefaultViper()
	cmd := agpl.Server(vip, func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		cfg, err := deployment.Config(vip)
		if err != nil {
			return nil, nil, xerrors.Errorf("failed to read config: %w", err)
		}

		if cfg.DERP.Server.RelayAddress != "" {
			_, err := url.Parse(cfg.DERP.Server.RelayAddress)
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
			AuditLogging:           options.DeploymentConfig.AuditLogging,
			BrowserOnly:            options.DeploymentConfig.BrowserOnly,
			SCIMAPIKey:             []byte(options.DeploymentConfig.SCIMAuthHeader),
			UserWorkspaceQuota:     options.DeploymentConfig.UserWorkspaceQuota,
			RBAC:                   true,
			DERPServerRelayAddress: options.DeploymentConfig.DERP.Server.RelayAddress,
			DERPServerRegionID:     options.DeploymentConfig.DERP.Server.RegionID,

			Options: options,
		}
		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, nil, err
		}
		return api.AGPL, api, nil
	})

	deployment.AttachEnterpriseFlags(cmd.Flags(), vip)

	return cmd
}
