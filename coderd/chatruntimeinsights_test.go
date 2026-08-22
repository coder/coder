package coderd_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAgentRuntimeInsights(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
		Logger:   &logger,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    owner.OrganizationID,
		OwnerID:           owner.UserID,
		LastModelConfigID: dbgen.ChatModelConfig(t, db, database.ChatModelConfig{}).ID,
	})
	_ = dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:    chat.ID,
		CreatedBy: uuid.NullUUID{UUID: owner.UserID, Valid: true},
		RuntimeMs: sql.NullInt64{Int64: (2 * time.Hour).Milliseconds(), Valid: true},
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	now := time.Now()
	// start_time must be midnight-aligned. end_time must be hour-aligned,
	// and the endpoint's own "not in the future" rounds up to the next
	// hour boundary, so use that as end_time to safely include a message
	// inserted at now().
	startTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(-24 * time.Hour)
	endTime := now.Truncate(time.Hour).Add(time.Hour)

	t.Run("OwnerCanRead", func(t *testing.T) {
		t.Parallel()

		summary, err := client.AgentRuntimeInsights(ctx, codersdk.AgentRuntimeInsightsRequest{
			StartTime: startTime,
			EndTime:   endTime,
		})
		require.NoError(t, err)
		require.Equal(t, (2 * time.Hour).Milliseconds(), summary.TotalMs)
		require.NotEmpty(t, summary.ByDay)

		byUser, err := client.AgentRuntimeInsightsByUser(ctx, codersdk.AgentRuntimeInsightsByUserRequest{
			StartTime: startTime,
			EndTime:   endTime,
		})
		require.NoError(t, err)
		require.Len(t, byUser.Users, 1)
		require.Equal(t, owner.UserID, byUser.Users[0].UserID)
		require.Equal(t, (2 * time.Hour).Milliseconds(), byUser.Users[0].TotalMs)
	})

	t.Run("MemberDenied", func(t *testing.T) {
		t.Parallel()

		_, err := memberClient.AgentRuntimeInsights(ctx, codersdk.AgentRuntimeInsightsRequest{
			StartTime: startTime,
			EndTime:   endTime,
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 403, sdkErr.StatusCode())

		_, err = memberClient.AgentRuntimeInsightsByUser(ctx, codersdk.AgentRuntimeInsightsByUserRequest{
			StartTime: startTime,
			EndTime:   endTime,
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 403, sdkErr.StatusCode())
	})
}
