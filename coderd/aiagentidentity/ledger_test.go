package aiagentidentity_test

import (
	"sync"
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

// TestConcurrentWorkspaceResolutionCreatesOneAgent asserts that a workspace
// ends up with one AI agent when several resolutions race.
//
// Resolution is check-then-create, and a unique index over agents used to make
// the losing insert fail. Capacity is a fact about the container rather than
// about agents, so that index is gone and the workspace's row lock is what
// serializes the resolutions.
func TestConcurrentWorkspaceResolutionCreatesOneAgent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	owner := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: organization.ID,
		UserID:         owner.ID,
	})
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: organization.ID,
		CreatedBy:      owner.ID,
	})
	table := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: organization.ID,
		OwnerID:        owner.ID,
		TemplateID:     template.ID,
	})
	workspace, err := db.GetWorkspaceByID(ctx, table.ID)
	require.NoError(t, err)

	const racers = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		agentID = map[uuid.UUID]int{}
		errs    []error
	)
	for range racers {
		wg.Go(func() {
			agent, err := aiagentidentity.ResolveWorkspaceOrigin(ctx, db, workspace)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			agentID[agent.UserID]++
		})
	}
	wg.Wait()

	require.Empty(t, errs, "every resolution succeeds")
	require.Len(t, agentID, 1, "every resolution names the same agent")

	var live int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM ai_agents WHERE origin_type = 'workspace' AND origin_id = $1 AND NOT deleted",
		workspace.ID).Scan(&live))
	require.Equal(t, 1, live, "the workspace has one live agent")

	var ledgerRows int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM ai_agent_ledger WHERE creation_site_type = 'workspace' AND creation_site_id = $1",
		workspace.ID).Scan(&ledgerRows))
	require.Equal(t, 1, ledgerRows, "and one ledger row, no losing transaction having left one behind")
}
