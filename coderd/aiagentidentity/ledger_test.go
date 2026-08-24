package aiagentidentity_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/testutil"
)

// TestTheLedgerIsTheIdentity asserts that every question ai_agents answers, the
// ledger answers for the same agent under the same identifier. That identity of
// identifiers is what lets the columns referring to an AI agent carry a foreign
// key to the ledger.
func TestTheLedgerIsTheIdentity(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: organization.ID,
		UserID:         owner.ID,
	})

	site := uuid.New()
	agentUser, agent, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     database.AIAgentOriginWorkspace,
		OriginID:       site,
	})
	require.NoError(t, err)

	row, err := db.GetAIAgentLedgerRowByID(ctx, agent.UserID)
	require.NoError(t, err, "the ledger holds the agent the identity code created")

	require.Equal(t, agent.UserID, row.ID, "one identifier, not two")
	require.Equal(t, agentUser.ID, row.ID)
	require.Equal(t, string(entity.TypeUser), row.OwnerType)
	require.Equal(t, owner.ID, row.OwnerID)
	require.Equal(t, string(entity.CreationSiteTypeWorkspace), row.CreationSiteType)
	require.Equal(t, site, row.CreationSiteID)
	require.Equal(t, entity.AIAgentStateActive, row.State)
	require.False(t, row.CreationTime.IsZero(), "the ledger knows when the agent came into being")

	// The creation time is the effective date of the entry it was folded from,
	// not a second reading of the clock.
	entries, err := db.GetAIAgentLifecycleEntriesBySubject(ctx, database.GetAIAgentLifecycleEntriesBySubjectParams{
		Subject: agent.UserID,
		Limit:   2,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, string(entity.EventAIAgentCreate), entries[0].Event)
	require.WithinDuration(t, entries[0].EffectiveDate, row.CreationTime, 0)
}

// TestChatTreeAdmitsOneLiveAgent asserts the capacity of a chat tree, which is
// a fact about the container rather than a uniqueness rule over agents. The
// refusal is a posting that affects no rows, not an error from the storage
// engine.
func TestChatTreeAdmitsOneLiveAgent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	owner := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})
	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})

	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    organization.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelConfig.ID,
	})
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    organization.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelConfig.ID,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	admitted, err := db.OccupyChatTree(ctx, root.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, admitted, "an empty tree admits an agent")

	refused, err := db.OccupyChatTree(ctx, root.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, refused, "an occupied tree refuses a second")

	// The container is the tree, so occupying through a sub-chat is the same
	// posting and meets the same refusal.
	refusedThroughChild, err := db.OccupyChatTree(ctx, child.ID)
	require.NoError(t, err)
	require.EqualValues(t, 0, refusedThroughChild, "a sub-chat resolves to its root")

	vacated, err := db.VacateChatTree(ctx, child.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, vacated, "vacating through a sub-chat empties the tree")

	readmitted, err := db.OccupyChatTree(ctx, root.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, readmitted, "a vacated tree admits another")

	// The count is meaningless on a non-root chat, and the constraint is what
	// keeps it that way. Nothing in the query layer can reach this, a sub-chat
	// always resolving to its root, so it is asserted directly.
	_, err = sqlDB.ExecContext(ctx,
		"UPDATE chats SET occupancy_count = 1 WHERE id = $1", child.ID)
	require.Error(t, err, "a non-root chat cannot carry an occupancy count")
	require.Contains(t, err.Error(), "chats_occupancy_only_on_root_chats")
}
