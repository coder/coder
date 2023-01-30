//go:build !slim

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
	"github.com/coder/coder/enterprise/audit"
	"github.com/coder/coder/enterprise/audit/backends"
	"github.com/coder/coder/enterprise/coderd"
	"github.com/coder/coder/enterprise/trialer"
	"github.com/coder/coder/tailnet"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	vip := deployment.NewViper()
	cmd := agpl.Server(vip, func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		if options.DeploymentConfig.DERP.Server.RelayURL.Value != "" {
			_, err := url.Parse(options.DeploymentConfig.DERP.Server.RelayURL.Value)
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

		if options.DeploymentConfig.AuditLogging.Value {
			options.Auditor = audit.NewAuditor(audit.DefaultFilter,
				backends.NewPostgres(options.Database, true),
				backends.NewSlog(options.Logger),
			)
		}

		options.TrialGenerator = trialer.New(options.Database, "https://v2-licensor.coder.com/trial", coderd.Keys)

		o := &coderd.Options{
			AuditLogging:           options.DeploymentConfig.AuditLogging.Value,
			BrowserOnly:            options.DeploymentConfig.BrowserOnly.Value,
			SCIMAPIKey:             []byte(options.DeploymentConfig.SCIMAPIKey.Value),
			RBAC:                   true,
			DERPServerRelayAddress: options.DeploymentConfig.DERP.Server.RelayURL.Value,
			DERPServerRegionID:     options.DeploymentConfig.DERP.Server.RegionID.Value,

			Options: options,
		}

		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, nil, err
		}
		return api.AGPL, api, nil
	})

	deployment.AttachFlags(cmd.Flags(), vip, true)

	return cmd
}
