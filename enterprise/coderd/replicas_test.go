package coderd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/dbtestutil"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
)

func TestReplicas(t *testing.T) {
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
	}, 10*time.Second, 250*time.Millisecond)

	_ = conn.Close()
}
