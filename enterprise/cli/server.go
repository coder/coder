//go:build !slim

package cli

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/url"

	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/types/key"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cryptorand"
	"github.com/coder/coder/enterprise/audit"
	"github.com/coder/coder/enterprise/audit/backends"
	"github.com/coder/coder/enterprise/coderd"
	"github.com/coder/coder/enterprise/trialer"
	"github.com/coder/coder/tailnet"

	agplcoderd "github.com/coder/coder/coderd"
)

func (r *RootCmd) server() *clibase.Cmd {
	cmd := r.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		if options.DeploymentValues.DERP.Server.RelayURL.String() != "" {
			_, err := url.Parse(options.DeploymentValues.DERP.Server.RelayURL.String())
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
		options.Auditor = audit.NewAuditor(audit.DefaultFilter,
			backends.NewPostgres(options.Database, true),
			backends.NewSlog(options.Logger),
		)

		options.TrialGenerator = trialer.New(options.Database, "https://v2-licensor.coder.com/trial", coderd.Keys)

		o := &coderd.Options{
			Options:                   options,
			AuditLogging:              true,
			BrowserOnly:               options.DeploymentValues.BrowserOnly.Value(),
			SCIMAPIKey:                []byte(options.DeploymentValues.SCIMAPIKey.Value()),
			RBAC:                      true,
			DERPServerRelayAddress:    options.DeploymentValues.DERP.Server.RelayURL.String(),
			DERPServerRegionID:        int(options.DeploymentValues.DERP.Server.RegionID.Value()),
			ProxyHealthInterval:       options.DeploymentValues.ProxyHealthStatusInterval.Value(),
			DefaultQuietHoursSchedule: options.DeploymentValues.UserQuietHoursSchedule.DefaultSchedule.Value(),
			ProvisionerDaemonPSK:      options.DeploymentValues.Provisioner.DaemonPSK.Value(),
		}

		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, nil, err
		}
		return api.AGPL, api, nil
	})
	return cmd
}
