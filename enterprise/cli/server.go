//go:build !slim

package cli

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"net/url"

	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/dormancy"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/enterprise/trialer"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/serpent"

	agplcoderd "github.com/coder/coder/v2/coderd"
)

func (r *RootCmd) Server(_ func()) *serpent.Cmd {
	cmd := r.RootCmd.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		if options.DeploymentValues.DERP.Server.RelayURL.String() != "" {
			_, err := url.Parse(options.DeploymentValues.DERP.Server.RelayURL.String())
			if err != nil {
				return nil, nil, xerrors.Errorf("derp-server-relay-address must be a valid HTTP URL: %w", err)
			}
		}

		options.DERPServer = derp.NewServer(key.NewNode(), tailnet.Logger(options.Logger.Named("derp")))

		var meshKey string
		err := options.Database.InTx(func(tx database.Store) error {
			// This will block until the lock is acquired, and will be
			// automatically released when the transaction ends.
			err := tx.AcquireLock(ctx, database.LockIDEnterpriseDeploymentSetup)
			if err != nil {
				return xerrors.Errorf("acquire lock: %w", err)
			}

			meshKey, err = tx.GetDERPMeshKey(ctx)
			if err == nil {
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return xerrors.Errorf("get DERP mesh key: %w", err)
			}
			meshKey, err = cryptorand.String(32)
			if err != nil {
				return xerrors.Errorf("generate DERP mesh key: %w", err)
			}
			err = tx.InsertDERPMeshKey(ctx, meshKey)
			if err != nil {
				return xerrors.Errorf("insert DERP mesh key: %w", err)
			}
			return nil
		}, nil)
		if err != nil {
			return nil, nil, err
		}
		if meshKey == "" {
			return nil, nil, xerrors.New("mesh key is empty")
		}
		options.DERPServer.SetMeshKey(meshKey)

		options.Auditor = audit.NewAuditor(
			options.Database,
			audit.DefaultFilter,
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

			CheckInactiveUsersCancelFunc: dormancy.CheckInactiveUsers(ctx, options.Logger, options.Database),
		}

		if encKeys := options.DeploymentValues.ExternalTokenEncryptionKeys.Value(); len(encKeys) != 0 {
			keys := make([][]byte, 0, len(encKeys))
			for idx, ek := range encKeys {
				dk, err := base64.StdEncoding.DecodeString(ek)
				if err != nil {
					return nil, nil, xerrors.Errorf("decode external-token-encryption-key %d: %w", idx, err)
				}
				keys = append(keys, dk)
			}
			cs, err := dbcrypt.NewCiphers(keys...)
			if err != nil {
				return nil, nil, xerrors.Errorf("initialize encryption: %w", err)
			}
			o.ExternalTokenEncryption = cs
		}

		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, nil, err
		}
		return api.AGPL, api, nil
	})

	cmd.AddSubcommands(
		r.dbcryptCmd(),
	)
	return cmd
}
