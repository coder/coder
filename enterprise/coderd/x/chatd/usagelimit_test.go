package chatd_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveUsageLimitStatus_OrgScoped(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	// Create two orgs and a user in both.
	orgA := dbgen.Organization(t, db, database.Organization{})
	orgB := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: orgA.ID,
	})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: orgB.ID,
	})

	// Create groups with different spend limits.
	// groupA ($5) and groupA2 ($20) are both in orgA to exercise
	// MIN aggregation within a single org.
	groupA := dbgen.Group(t, db, database.Group{
		OrganizationID: orgA.ID,
	})
	groupA2 := dbgen.Group(t, db, database.Group{
		OrganizationID: orgA.ID,
	})
	groupB := dbgen.Group(t, db, database.Group{
		OrganizationID: orgB.ID,
	})
	dbgen.GroupMember(t, db, database.GroupMemberTable{
		UserID:  user.ID,
		GroupID: groupA.ID,
	})
	dbgen.GroupMember(t, db, database.GroupMemberTable{
		UserID:  user.ID,
		GroupID: groupA2.ID,
	})
	dbgen.GroupMember(t, db, database.GroupMemberTable{
		UserID:  user.ID,
		GroupID: groupB.ID,
	})

	// Set group spend limits: groupA=$5, groupA2=$20, groupB=$50.
	_, err := db.UpsertChatUsageLimitGroupOverride(ctx, database.UpsertChatUsageLimitGroupOverrideParams{
		GroupID:          groupA.ID,
		SpendLimitMicros: 5_000_000,
	})
	require.NoError(t, err)
	_, err = db.UpsertChatUsageLimitGroupOverride(ctx, database.UpsertChatUsageLimitGroupOverrideParams{
		GroupID:          groupA2.ID,
		SpendLimitMicros: 20_000_000,
	})
	require.NoError(t, err)
	_, err = db.UpsertChatUsageLimitGroupOverride(ctx, database.UpsertChatUsageLimitGroupOverrideParams{
		GroupID:          groupB.ID,
		SpendLimitMicros: 50_000_000,
	})
	require.NoError(t, err)

	// Enable usage limits with a high default so group limits win.
	_, err = db.UpsertChatUsageLimitConfig(ctx, database.UpsertChatUsageLimitConfigParams{
		Enabled:            true,
		DefaultLimitMicros: 100_000_000,
		Period:             string(codersdk.ChatUsageLimitPeriodMonth),
	})
	require.NoError(t, err)

	// We need a chat provider + model config for inserting chats.
	_, err = db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             "openai",
		DisplayName:          "openai",
		APIKey:               "test-key",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)
	modelConfig, err := db.InsertChatModelConfig(ctx, database.InsertChatModelConfigParams{
		Provider:             "openai",
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		CreatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedBy:            uuid.NullUUID{UUID: user.ID, Valid: true},
		Enabled:              true,
		IsDefault:            true,
		ContextLimit:         128000,
		CompressionThreshold: 70,
		Options:              json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	now := time.Now().UTC()

	// insertChatWithSpend is a test helper that creates a chat in the
	// given org and inserts a single message with the specified cost.
	insertChatWithSpend := func(t *testing.T, ownerID, orgID, modelCfgID uuid.UUID, costMicros int64) {
		t.Helper()
		tctx := testutil.Context(t, testutil.WaitLong)
		c, err := db.InsertChat(tctx, database.InsertChatParams{
			OrganizationID:    orgID,
			OwnerID:           ownerID,
			LastModelConfigID: modelCfgID,
			Title:             "test chat",
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeUi,
			MCPServerIDs:      []uuid.UUID{},
		})
		require.NoError(t, err)
		_, err = db.InsertChatMessages(tctx, database.InsertChatMessagesParams{
			ChatID:              c.ID,
			CreatedBy:           []uuid.UUID{uuid.Nil},
			ModelConfigID:       []uuid.UUID{modelCfgID},
			Role:                []database.ChatMessageRole{database.ChatMessageRoleAssistant},
			Content:             []string{`[{"type":"text","text":"hello"}]`},
			ContentVersion:      []int16{1},
			Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
			InputTokens:         []int64{100},
			OutputTokens:        []int64{50},
			TotalTokens:         []int64{150},
			ReasoningTokens:     []int64{0},
			CacheCreationTokens: []int64{0},
			CacheReadTokens:     []int64{0},
			ContextLimit:        []int64{128000},
			Compressed:          []bool{false},
			TotalCostMicros:     []int64{costMicros},
			RuntimeMs:           []int64{500},
			ProviderResponseID:  []string{uuid.NewString()},
		})
		require.NoError(t, err)
	}

	t.Run("OrgA_gets_orgA_limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// orgA has groupA ($5) and groupA2 ($20). MIN($5, $20) = $5.
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, uuid.NullUUID{UUID: orgA.ID, Valid: true}, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(5_000_000), *status.SpendLimitMicros,
			"orgA should resolve to MIN of both groups ($5, $20) = $5")
	})

	t.Run("OrgB_gets_orgB_limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, uuid.NullUUID{UUID: orgB.ID, Valid: true}, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(50_000_000), *status.SpendLimitMicros,
			"orgB should resolve to groupB's $50 limit, not global MIN")
	})

	t.Run("UnknownOrg_gets_global_default", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// When the org ID does not match any group the user belongs
		// to, MIN() over an empty set returns NULL, the CASE sees
		// gl.limit_micros IS NOT NULL as false, and falls through
		// to the global default. This subtest guards that contract:
		// if someone changes the NULL-handling in
		// ResolveUserChatSpendLimit, this will catch it.
		randomOrg := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, randomOrg, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(100_000_000), *status.SpendLimitMicros,
			"org with no matching groups should fall through to global default ($100)")
	})

	t.Run("NilOrg_gets_global_min", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// NULL org = global behavior: MIN across all groups.
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, uuid.NullUUID{}, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(5_000_000), *status.SpendLimitMicros,
			"nil org should fall back to global MIN($5, $20, $50) = $5")
	})

	t.Run("Spend_scoped_to_org", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Dedicated user so spend insertion doesn't affect sibling subtests.
		spendUser := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         spendUser.ID,
			OrganizationID: orgA.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         spendUser.ID,
			OrganizationID: orgB.ID,
		})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  spendUser.ID,
			GroupID: groupA.ID,
		})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  spendUser.ID,
			GroupID: groupB.ID,
		})

		insertChatWithSpend(t, spendUser.ID, orgA.ID, modelConfig.ID, 3_000_000)

		// Resolve for orgB: should see zero spend (orgA's $3 not counted).
		statusB, err := chatd.ResolveUsageLimitStatus(ctx, db, spendUser.ID, uuid.NullUUID{UUID: orgB.ID, Valid: true}, now)
		require.NoError(t, err)
		require.NotNil(t, statusB)
		require.Equal(t, int64(0), statusB.CurrentSpend,
			"orgB should not include orgA's spend")

		// Resolve for orgA: should see $3 spend.
		statusA, err := chatd.ResolveUsageLimitStatus(ctx, db, spendUser.ID, uuid.NullUUID{UUID: orgA.ID, Valid: true}, now)
		require.NoError(t, err)
		require.NotNil(t, statusA)
		require.Equal(t, int64(3_000_000), statusA.CurrentSpend,
			"orgA should include its own spend")

		// Nil org: should see $3 (global).
		statusNil, err := chatd.ResolveUsageLimitStatus(ctx, db, spendUser.ID, uuid.NullUUID{}, now)
		require.NoError(t, err)
		require.NotNil(t, statusNil)
		require.Equal(t, int64(3_000_000), statusNil.CurrentSpend,
			"nil org should include all spend globally")
	})

	t.Run("User_override_beats_group", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// Create a separate user with a personal override.
		user2 := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user2.ID,
			OrganizationID: orgA.ID,
		})
		dbgen.GroupMember(t, db, database.GroupMemberTable{
			UserID:  user2.ID,
			GroupID: groupA.ID,
		})

		// Set $10 user override (beats groupA's $5 limit).
		_, err := db.UpsertChatUsageLimitUserOverride(ctx, database.UpsertChatUsageLimitUserOverrideParams{
			UserID:           user2.ID,
			SpendLimitMicros: 10_000_000,
		})
		require.NoError(t, err)

		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user2.ID, uuid.NullUUID{UUID: orgA.ID, Valid: true}, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(10_000_000), *status.SpendLimitMicros,
			"user override should take priority over group limit")
	})

	t.Run("UserOverride_spend_is_global", func(t *testing.T) {
		t.Parallel()
		// When user override wins, spend should be checked globally,
		// not per-org. Otherwise a user in N orgs can spend limit*N.
		user3 := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user3.ID,
			OrganizationID: orgA.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user3.ID,
			OrganizationID: orgB.ID,
		})

		// Set $10 user override.
		_, err := db.UpsertChatUsageLimitUserOverride(testutil.Context(t, testutil.WaitLong), database.UpsertChatUsageLimitUserOverrideParams{
			UserID:           user3.ID,
			SpendLimitMicros: 10_000_000,
		})
		require.NoError(t, err)

		// $6 in orgA + $6 in orgB = $12 total.
		insertChatWithSpend(t, user3.ID, orgA.ID, modelConfig.ID, 6_000_000)
		insertChatWithSpend(t, user3.ID, orgB.ID, modelConfig.ID, 6_000_000)

		ctx := testutil.Context(t, testutil.WaitLong)
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user3.ID, uuid.NullUUID{UUID: orgA.ID, Valid: true}, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(10_000_000), *status.SpendLimitMicros)
		// Spend should be global ($12), not org-scoped ($6).
		require.Equal(t, int64(12_000_000), status.CurrentSpend,
			"user override should check global spend to prevent cross-org evasion")
	})

	t.Run("GlobalDefault_spend_is_global", func(t *testing.T) {
		t.Parallel()
		// When global default wins (no groups in the target org,
		// no user override), spend should also be checked globally.
		user4 := dbgen.User(t, db, database.User{})
		orgC := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user4.ID,
			OrganizationID: orgA.ID,
		})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user4.ID,
			OrganizationID: orgC.ID,
		})

		// $30 in orgA + $40 in orgC = $70 total.
		insertChatWithSpend(t, user4.ID, orgA.ID, modelConfig.ID, 30_000_000)
		insertChatWithSpend(t, user4.ID, orgC.ID, modelConfig.ID, 40_000_000)

		ctx := testutil.Context(t, testutil.WaitLong)
		// user4 has no groups in orgC, no override: falls through
		// to global default ($100).
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user4.ID, uuid.NullUUID{UUID: orgC.ID, Valid: true}, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(100_000_000), *status.SpendLimitMicros,
			"should fall through to global default ($100)")
		// Spend should be global ($70), not org-scoped ($40).
		require.Equal(t, int64(70_000_000), status.CurrentSpend,
			"global default should check global spend")
	})
}
