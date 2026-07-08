//go:build !slim

package cli

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"net/url"
	"time"

	"golang.org/x/xerrors"
	"tailscale.com/derp"
	"tailscale.com/types/key"

	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/enterprise/audit"
	"github.com/coder/coder/v2/enterprise/audit/backends"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/coder/v2/enterprise/coderd/dormancy"
	"github.com/coder/coder/v2/enterprise/coderd/usage"
	"github.com/coder/coder/v2/enterprise/dbcrypt"
	"github.com/coder/coder/v2/enterprise/trialer"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func (r *RootCmd) Server(_ func()) *serpent.Command {
	cmd := r.RootCmd.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		var (
			derpURL *url.URL
			err     error
		)
		if options.DeploymentValues.DERP.Server.RelayURL.String() != "" {
			derpURL, err = url.Parse(options.DeploymentValues.DERP.Server.RelayURL.String())
			if err != nil {
				return nil, nil, xerrors.Errorf("derp-server-relay-address must be a valid HTTP URL: %w", err)
			}
		}
		clusterHost := options.DeploymentValues.Cluster.Host.String()
		if clusterHost == "" && derpURL != nil {
			// Use the DERP host if the operator didn't specify an explicit cluster host, since this is an older setting
			// and more likely to be configured by longtime HA customers.
			clusterHost = derpURL.Hostname()
		}

		// Always generate a mesh key, even if the built-in DERP server is
		// disabled. This mesh key is still used by workspace proxies running
		// HA.
		var meshKey string
		err = options.Database.InTx(func(tx database.Store) error {
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

		if options.DeploymentValues.DERP.Server.Enable {
			options.DERPServer = derp.NewServer(key.NewNode(), tailnet.Logger(options.Logger.Named("derp")))
			options.DERPServer.SetMeshKey(meshKey)
		}

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
			ConnectionLogging:         true,
			BrowserOnly:               options.DeploymentValues.BrowserOnly.Value(),
			SCIMAPIKey:                []byte(options.DeploymentValues.SCIMAPIKey.Value()),
			UseLegacySCIM:             options.DeploymentValues.UseLegacySCIM.Value(),
			RBAC:                      true,
			ClusterHost:               clusterHost,
			DERPServerRelayAddress:    options.DeploymentValues.DERP.Server.RelayURL.String(),
			DERPServerRegionID:        int(options.DeploymentValues.DERP.Server.RegionID.Value()),
			ProxyHealthInterval:       options.DeploymentValues.ProxyHealthStatusInterval.Value(),
			DefaultQuietHoursSchedule: options.DeploymentValues.UserQuietHoursSchedule.DefaultSchedule.Value(),
			ProvisionerDaemonPSK:      options.DeploymentValues.Provisioner.DaemonPSK.Value(),

			CheckInactiveUsersCancelFunc: dormancy.CheckInactiveUsers(ctx, options.Logger, quartz.NewReal(), options.Database, options.Auditor),
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

		if o.LicenseKeys == nil {
			o.LicenseKeys = coderd.Keys
		}

		closers := &multiCloser{}

		// Create the enterprise API.
		api, err := coderd.New(ctx, o)
		if err != nil {
			return nil, nil, err
		}
		closers.Add(api)

		// Start the enterprise usage publisher routine. This won't do anything
		// unless the deployment is licensed and one of the licenses has usage
		// publishing enabled.
		publisher := usage.NewTallymanPublisher(ctx, options.Logger, options.Database, o.LicenseKeys,
			usage.PublisherWithHTTPClient(api.HTTPClient),
		)
		err = publisher.Start()
		if err != nil {
			_ = closers.Close()
			return nil, nil, xerrors.Errorf("start usage publisher: %w", err)
		}
		closers.Add(publisher)

		// usageCron are heartbeat events to the usage table. These events are eventually sent
		// to Tallyman.
		usageCron := usage.NewCron(quartz.NewReal(), options.Logger.Named("usage-cron"), options.Database, *options.UsageInserter.Load())
		// ai-seats heartbeats track the number of users that have used an AI feature.
		// These users consume a seat for the AI addon to our License.
		_ = usageCron.Register(usage.CronJob{
			Name:     "ai-seats",
			Interval: usage.AISeatsInterval,
			Jitter:   10 * time.Minute,
			Fn:       usage.AISeatsHeartbeat(options.Database),
		})
		usageCron.Start(ctx)
		closers.Add(usageCron)

		// In-memory AI Bridge Proxy daemon. The bridge daemon itself is
		// started unconditionally by AGPL cli/server.go (chatd uses its
		// in-memory roundtripper regardless of license); only the proxy
		// daemon remains enterprise-gated by config.
		if options.DeploymentValues.AI.BridgeProxyConfig.Enabled.Value() {
			// Seed env-derived providers before the proxy daemon's reloader
			// reads them back so the proxy observes them on first startup.
			// options.Database is dbcrypt-wrapped at this point (set by
			// coderd.New above), so env-seeded keys are also written
			// encrypted. Detached ctx for the same reason as in agplcli
			// below: an early return would orphan newAPI's goroutines.
			// Seeding is idempotent; the agplcli path seeds again
			// post-newAPI.
			//nolint:gocritic // Production timeout, not a test wait.
			aibridgeInitCtx, aibridgeInitCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer aibridgeInitCancel()
			if err := agplcoderd.SeedAIProvidersFromEnv(
				aibridgeInitCtx,
				options.Database,
				options.DeploymentValues.AI.BridgeConfig,
				options.Logger.Named("aibridge.envseed"),
			); err != nil {
				return nil, nil, xerrors.Errorf("seed ai providers from env: %w", err)
			}
			aiBridgeProxyCloser, err := newAIBridgeProxyDaemon(api)
			if err != nil {
				_ = closers.Close()
				return nil, nil, xerrors.Errorf("create aibridgeproxyd: %w", err)
			}
			closers.Add(aiBridgeProxyCloser)
		}

		return api.AGPL, closers, nil
	})

	cmd.AddSubcommands(
		r.dbcryptCmd(),
	)
	return cmd
}

type multiCloser struct {
	closers []io.Closer
}

var _ io.Closer = &multiCloser{}

func (m *multiCloser) Add(closer io.Closer) {
	m.closers = append(m.closers, closer)
}

func (m *multiCloser) Close() error {
	var errs []error
	for _, closer := range m.closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, xerrors.Errorf("close %T: %w", closer, err))
		}
	}
	return errors.Join(errs...)
}
