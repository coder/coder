package coderd_test

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestReplicas(t *testing.T) {
	t.Parallel()
	if !dbtestutil.WillUsePostgres() {
		t.Skip("only test with real postgresF")
	}
	t.Run("ErrorWithoutLicense", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		firstClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   pubsub,
			},
			DontAddLicense: true,
		})
		secondClient, _, secondAPI, _ := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
			DontAddFirstUser: true,
			DontAddLicense:   true,
		})
		secondClient.SetSessionToken(firstClient.SessionToken())
		ents, err := secondClient.Entitlements(context.Background())
		require.NoError(t, err)
		require.Len(t, ents.Errors, 1)
		_ = secondAPI.Close()

		ents, err = firstClient.Entitlements(context.Background())
		require.NoError(t, err)
		require.Len(t, ents.Warnings, 0)
	})
	t.Run("ConnectAcrossMultiple", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		firstClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   pubsub,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureHighAvailability: 1,
				},
			},
		})

		secondClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
			DontAddLicense:   true,
			DontAddFirstUser: true,
		})
		secondClient.SetSessionToken(firstClient.SessionToken())
		replicas, err := secondClient.Replicas(context.Background())
		require.NoError(t, err)
		require.Len(t, replicas, 2)

		r := setupWorkspaceAgent(t, firstClient, firstUser, 0)
		conn, err := secondClient.DialWorkspaceAgent(context.Background(), r.sdkAgent.ID, &codersdk.DialWorkspaceAgentOptions{
			BlockEndpoints: true,
			Logger:         slogtest.Make(t, nil).Leveled(slog.LevelDebug),
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitShort)
			defer cancelFunc()
			_, _, _, err = conn.Ping(ctx)
			return err == nil
		}, testutil.WaitLong, testutil.IntervalFast)
		_ = conn.Close()
	})
	t.Run("ConnectAcrossMultipleTLS", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		certificates := []tls.Certificate{testutil.GenerateTLSCertificate(t, "localhost")}
		firstClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   pubsub,
				TLSCertificates:          certificates,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureHighAvailability: 1,
				},
			},
		})

		secondClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:        db,
				Pubsub:          pubsub,
				TLSCertificates: certificates,
			},
			DontAddFirstUser: true,
			DontAddLicense:   true,
		})
		secondClient.SetSessionToken(firstClient.SessionToken())
		replicas, err := secondClient.Replicas(context.Background())
		require.NoError(t, err)
		require.Len(t, replicas, 2)

		r := setupWorkspaceAgent(t, firstClient, firstUser, 0)
		conn, err := secondClient.DialWorkspaceAgent(context.Background(), r.sdkAgent.ID, &codersdk.DialWorkspaceAgentOptions{
			BlockEndpoints: true,
			Logger:         slogtest.Make(t, nil).Named("client").Leveled(slog.LevelDebug),
		})
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.IntervalSlow)
			defer cancelFunc()
			_, _, _, err = conn.Ping(ctx)
			return err == nil
		}, testutil.WaitLong, testutil.IntervalFast)
		_ = conn.Close()
		replicas, err = secondClient.Replicas(context.Background())
		require.NoError(t, err)
		require.Len(t, replicas, 2)
		for _, replica := range replicas {
			require.Empty(t, replica.Error)
		}
	})
}
