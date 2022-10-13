package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/testutil"
)

func TestReplicas(t *testing.T) {
	t.Parallel()
	t.Run("WarningsWithoutLicense", func(t *testing.T) {
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
		secondClient := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
		})
		secondClient.SessionToken = firstClient.SessionToken
		ents, err := secondClient.Entitlements(context.Background())
		require.NoError(t, err)
		require.Len(t, ents.Warnings, 1)
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
			HighAvailability: true,
		})

		secondClient := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
		})
		secondClient.SessionToken = firstClient.SessionToken

		agentID := setupWorkspaceAgent(t, firstClient, firstUser)
		conn, err := secondClient.DialWorkspaceAgentTailnet(context.Background(), slogtest.Make(t, nil).Leveled(slog.LevelDebug), agentID)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			_, err = conn.Ping()
			return err == nil
		}, testutil.WaitShort, testutil.IntervalFast)
		_ = conn.Close()
	})
}
