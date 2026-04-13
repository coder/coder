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
	groupA := dbgen.Group(t, db, database.Group{
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
		GroupID: groupB.ID,
	})

	// Set group spend limits: orgA=$5, orgB=$50.
	_, err := db.UpsertChatUsageLimitGroupOverride(ctx, database.UpsertChatUsageLimitGroupOverrideParams{
		GroupID:          groupA.ID,
		SpendLimitMicros: 5_000_000,
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

	t.Run("OrgA_gets_orgA_limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, orgA.ID, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(5_000_000), *status.SpendLimitMicros,
			"orgA should resolve to groupA's $5 limit, not global MIN")
	})

	t.Run("OrgB_gets_orgB_limit", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, orgB.ID, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(50_000_000), *status.SpendLimitMicros,
			"orgB should resolve to groupB's $50 limit, not global MIN")
	})

	t.Run("UnknownOrg_falls_through_to_global_default", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// When the org ID doesn't match any group the user belongs to,
		// the MIN() over an empty set returns NULL, COALESCE yields -1
		// ("no group limit"), and the CASE falls through to the global
		// default. This subtest guards that contract — if someone changes
		// the COALESCE or NULL-handling in ResolveUserChatSpendLimit, this
		// will catch it.
		randomOrg := uuid.New()
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
		// Nil UUID = legacy global behavior: MIN across all groups.
		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, uuid.Nil, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(5_000_000), *status.SpendLimitMicros,
			"nil org should fall back to global MIN($5, $50) = $5")
	})

	t.Run("Spend_scoped_to_org", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// Insert a chat in orgA with some spend.
		chatA, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    orgA.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelConfig.ID,
			Title:             "orgA chat",
			Status:            database.ChatStatusWaiting,
			MCPServerIDs:      []uuid.UUID{},
		})
		require.NoError(t, err)

		// Insert message with $3 cost in orgA's chat.
		_, err = db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
			ChatID:              chatA.ID,
			CreatedBy:           []uuid.UUID{uuid.Nil},
			ModelConfigID:       []uuid.UUID{modelConfig.ID},
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
			TotalCostMicros:     []int64{3_000_000},
			RuntimeMs:           []int64{500},
			ProviderResponseID:  []string{"resp-1"},
		})
		require.NoError(t, err)

		// Resolve for orgB — should see zero spend (orgA's $3 not counted).
		statusB, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, orgB.ID, now)
		require.NoError(t, err)
		require.NotNil(t, statusB)
		require.Equal(t, int64(0), statusB.CurrentSpend,
			"orgB should not include orgA's spend")

		// Resolve for orgA — should see $3 spend.
		statusA, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, orgA.ID, now)
		require.NoError(t, err)
		require.NotNil(t, statusA)
		require.Equal(t, int64(3_000_000), statusA.CurrentSpend,
			"orgA should include its own spend")

		// Nil org — should see $3 (global).
		statusNil, err := chatd.ResolveUsageLimitStatus(ctx, db, user.ID, uuid.Nil, now)
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

		status, err := chatd.ResolveUsageLimitStatus(ctx, db, user2.ID, orgA.ID, now)
		require.NoError(t, err)
		require.NotNil(t, status)
		require.NotNil(t, status.SpendLimitMicros)
		require.Equal(t, int64(10_000_000), *status.SpendLimitMicros,
			"user override should take priority over group limit")
	})
}
