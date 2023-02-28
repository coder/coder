package coderd_test

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/testutil"
)

func TestReplicas(t *testing.T) {
	t.Parallel()
	t.Run("ErrorWithoutLicense", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		firstClient := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   pubsub,
			},
		})
		_ = coderdtest.CreateFirstUser(t, firstClient)
		secondClient, _, secondAPI := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
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
		firstClient := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   pubsub,
			},
		})
		firstUser := coderdtest.CreateFirstUser(t, firstClient)
		coderdenttest.AddLicense(t, firstClient, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureHighAvailability: 1,
			},
		})

		secondClient := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
		})
		secondClient.SetSessionToken(firstClient.SessionToken())
		replicas, err := secondClient.Replicas(context.Background())
		require.NoError(t, err)
		require.Len(t, replicas, 2)

		_, agent := setupWorkspaceAgent(t, firstClient, firstUser, 0)
		conn, err := secondClient.DialWorkspaceAgent(context.Background(), agent.ID, &codersdk.DialWorkspaceAgentOptions{
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
		firstClient := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
				Database:                 db,
				Pubsub:                   pubsub,
				TLSCertificates:          certificates,
			},
		})
		firstUser := coderdtest.CreateFirstUser(t, firstClient)
		coderdenttest.AddLicense(t, firstClient, coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureHighAvailability: 1,
			},
		})

		secondClient := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:        db,
				Pubsub:          pubsub,
				TLSCertificates: certificates,
			},
		})
		secondClient.SetSessionToken(firstClient.SessionToken())
		replicas, err := secondClient.Replicas(context.Background())
		require.NoError(t, err)
		require.Len(t, replicas, 2)

		_, agent := setupWorkspaceAgent(t, firstClient, firstUser, 0)
		conn, err := secondClient.DialWorkspaceAgent(context.Background(), agent.ID, &codersdk.DialWorkspaceAgentOptions{
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
