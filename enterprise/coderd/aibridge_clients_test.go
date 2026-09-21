package coderd_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
	"github.com/stretchr/testify/require"
)

func TestAIBridgeListClientsDeduplicatesUnknown(t *testing.T) {
	t.Parallel()

	client, db, firstUser := coderdenttest.NewWithDatabase(t, aibridgeOpts(t))
	ctx := testutil.Context(t, testutil.WaitLong)

	now := dbtime.Now()
	endedAt := now.Add(time.Minute)

	for _, clientName := range []sql.NullString{
		{Valid: false},
		{String: "Unknown", Valid: true},
	} {
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: firstUser.UserID,
			Provider:    "openai",
			Model:       "gpt-5",
			StartedAt:   now,
			Client:      clientName,
		}, &endedAt)
	}

	//nolint:gocritic // Owner role is irrelevant here.
	clients, err := client.AIBridgeListClients(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"Unknown"}, clients)
}
