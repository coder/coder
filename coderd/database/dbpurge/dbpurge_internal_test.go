package dbpurge

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	agplusage "github.com/coder/coder/v2/coderd/usage"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/serpent"
)

func TestDBPurgeAuthorization(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	rawDB, _ := dbtestutil.NewDB(t)

	require.NoError(t, rawDB.UpsertChatRetentionDays(ctx, 30))
	require.NoError(t, rawDB.EnsureAgentRuntimeBackfillCheckpoint(ctx))

	authz := rbac.NewAuthorizer(prometheus.NewRegistry())
	db := dbauthz.New(rawDB, authz, testutil.Logger(t), coderdtest.AccessControlStorePointer())

	ctx = dbauthz.AsDBPurge(ctx)

	clk := quartz.NewMock(t)
	now := time.Date(2025, 1, 15, 7, 30, 0, 0, time.UTC)
	clk.Set(now)

	vals := &codersdk.DeploymentValues{ /* same vals as before */ }

	inst := &instance{
		logger: testutil.Logger(t),
		vals:   vals,
		clk:    clk,
		// metrics can be nil in this test
	}

	err := inst.purgeTick(ctx, db, now)
	require.NoError(t, err)
}

func TestAgentRuntimeBackfillDefersOnlyChatPurge(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, rawDB := dbtestutil.NewDBWithSQLDB(t)
	now := time.Date(2025, 3, 10, 14, 0, 0, 0, time.UTC)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Model: "purge-catchup", ContextLimit: 8192})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: modelConfig.ID,
	})
	_, err := db.ArchiveChatByID(ctx, chat.ID)
	require.NoError(t, err)
	_, err = rawDB.ExecContext(ctx, "UPDATE chats SET updated_at = $1 WHERE id = $2", now.Add(-31*24*time.Hour), chat.ID)
	require.NoError(t, err)

	expiredKey, _ := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		ExpiresAt: now.Add(-8 * 24 * time.Hour),
		TokenName: "catchup-purge-test",
	})
	require.NoError(t, db.UpsertChatRetentionDays(ctx, 30))
	require.NoError(t, db.EnsureAgentRuntimeBackfillCheckpoint(ctx))

	inst := &instance{
		logger: testutil.Logger(t),
		vals: &codersdk.DeploymentValues{Retention: codersdk.RetentionConfig{
			APIKeys: serpent.Duration(7 * 24 * time.Hour),
		}},
		clk: quartz.NewMock(t),
	}
	require.NoError(t, inst.purgeTick(ctx, db, now))

	_, err = db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err, "pending catch-up must preserve the archived chat")
	_, err = db.GetAPIKeyByID(ctx, expiredKey.ID)
	require.Error(t, err, "unrelated API key purge must still run")

	completedAt := now
	endExclusive := now.Add(-7 * 24 * time.Hour).Truncate(time.Hour)
	state, err := agplusage.MarshalAgentRuntimeBackfillState(agplusage.AgentRuntimeBackfillState{
		Version:      agplusage.AgentRuntimeBackfillVersion,
		Status:       agplusage.AgentRuntimeBackfillStatusComplete,
		NextBucket:   &endExclusive,
		EndExclusive: &endExclusive,
		CompletedAt:  &completedAt,
	})
	require.NoError(t, err)
	updated, err := db.UpdateAgentRuntimeBackfillCheckpoint(ctx, state)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	require.NoError(t, inst.purgeTick(ctx, db, now))

	_, err = db.GetChatByID(ctx, chat.ID)
	require.Error(t, err, "completed catch-up must allow archived chat deletion")
}
