package aiagentidentity_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/testutil"
)

// TestTheLedgerIsTheIdentity asserts that the ledger answers owner, creation
// site, creation time and state for the agent the identity code created, under
// the same identifier. That identity of identifiers is what lets the columns
// referring to an AI agent carry a foreign key to the ledger.
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
	agentUser, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     entity.CreationSiteTypeWorkspace,
		OriginID:       site,
	})
	require.NoError(t, err)

	row, err := db.GetAIAgentLedgerRowByID(ctx, agentUser.ID)
	require.NoError(t, err, "the ledger holds the agent the identity code created")

	require.Equal(t, agentUser.ID, row.ID, "one identifier, not two")
	require.Equal(t, string(entity.TypeUser), row.OwnerType)
	require.Equal(t, owner.ID, row.OwnerID)
	require.Equal(t, string(entity.CreationSiteTypeWorkspace), row.CreationSiteType)
	require.Equal(t, site, row.CreationSiteID)
	require.Equal(t, entity.AIAgentStateActive, row.State)
	require.False(t, row.CreationTime.IsZero(), "the ledger knows when the agent came into being")

	// The creation time is the effective date of the entry it was folded from,
	// not a second reading of the clock.
	entries, err := db.GetAIAgentLifecycleEntriesBySubject(ctx, database.GetAIAgentLifecycleEntriesBySubjectParams{
		Subject: agentUser.ID,
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
			agentID[agent.ID]++
		})
	}
	wg.Wait()

	require.Empty(t, errs, "every resolution succeeds")
	require.Len(t, agentID, 1, "every resolution names the same agent")

	var live int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM ai_agent_ledger WHERE creation_site_type = 'workspace' AND creation_site_id = $1 AND state = 'active'",
		workspace.ID).Scan(&live))
	require.Equal(t, 1, live, "the workspace has one live agent")

	var ledgerRows int
	require.NoError(t, sqlDB.QueryRowContext(ctx,
		"SELECT count(*) FROM ai_agent_ledger WHERE creation_site_type = 'workspace' AND creation_site_id = $1",
		workspace.ID).Scan(&ledgerRows))
	require.Equal(t, 1, ledgerRows, "and one ledger row, no losing transaction having left one behind")
}

// TestOwnershipChangeRetiresTheAgentInTheLedger asserts that revoking an agent
// reaches the ledger, which is what resolution reads. Before this the two
// revocation paths wrote only the mirror, so a revoked agent still resolved as
// live once resolution moved onto the ledger.
//
// The event is `kill` and the actor is the new owner, both of which are proof
// of concept cheats: nobody ordered this agent's death, and the ownership
// changed at some earlier moment that this resolution is merely noticing.
func TestOwnershipChangeRetiresTheAgentInTheLedger(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	organization := dbgen.Organization(t, db, database.Organization{})
	first := dbgen.User(t, db, database.User{})
	second := dbgen.User(t, db, database.User{})
	for _, u := range []database.User{first, second} {
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         u.ID,
		})
	}
	template := dbgen.Template(t, db, database.Template{
		OrganizationID: organization.ID,
		CreatedBy:      first.ID,
	})
	table := dbgen.Workspace(t, db, database.WorkspaceTable{
		OrganizationID: organization.ID,
		OwnerID:        first.ID,
		TemplateID:     template.ID,
	})
	workspace, err := db.GetWorkspaceByID(ctx, table.ID)
	require.NoError(t, err)

	original, err := aiagentidentity.ResolveWorkspaceOrigin(ctx, db, workspace)
	require.NoError(t, err)

	// No query changes a workspace's owner, and resolution compares the agent's
	// owner against the workspace it is handed, so the change is presented the
	// way the function reads it.
	transferred := workspace
	transferred.OwnerID = second.ID

	replacement, err := aiagentidentity.ResolveWorkspaceOrigin(ctx, db, transferred)
	require.NoError(t, err)
	require.NotEqual(t, original.ID, replacement.ID, "a new owner gets a new agent")

	retired, err := db.GetAIAgentLedgerRowByID(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, entity.AIAgentStateRetired, retired.State, "the old agent is retired in the ledger")

	live, err := db.GetAIAgentLedgerRowByID(ctx, replacement.ID)
	require.NoError(t, err)
	require.Equal(t, entity.AIAgentStateActive, live.State)
	require.Equal(t, second.ID, live.OwnerID)

	// Resolution reads the ledger, so the retired agent no longer resolves.
	_, err = aiagentidentity.Resolve(ctx, db, original.ID)
	require.ErrorIs(t, err, aiagentidentity.ErrAIAgentDeleted)
}

// TestMintedKeyIsInTheLedger asserts that the credential an AI agent actually
// presents is recorded, which until now it was not: the ledger held a password
// credential nobody presents and knew nothing of the api_key every request
// carries.
//
// It also asserts the expiry, because preserving it is the whole reason the
// mirror takes a lifetime. Routing minting through the ledger without that
// would have turned a token that expires in a day into one that never does.
func TestMintedKeyIsInTheLedger(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	owner := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		OrganizationID: organization.ID,
		UserID:         owner.ID,
	})

	chatID := uuid.New()
	agentUser, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        owner.ID,
		OrganizationID: organization.ID,
		OriginType:     entity.CreationSiteTypeChat,
		OriginID:       chatID,
	})
	require.NoError(t, err)

	before := dbtime.Now()
	key, token, err := aiagentidentity.MintKey(ctx, db, agentUser.ID,
		aiagentidentity.ChatAgentProfile(chatID))
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// The mirrored row and the ledger's record of it are the same credential.
	mirrored, err := db.GetCredentialAPIKeyByKeyID(ctx, key.ID)
	require.NoError(t, err, "the ledger holds the credential the agent presents")

	row, err := db.GetCredentialLedgerRowByID(ctx, mirrored.ID)
	require.NoError(t, err)
	require.Equal(t, string(entity.TypeAIAgent), row.HolderType)
	require.Equal(t, agentUser.ID, row.HolderID)
	require.Equal(t, entity.CredentialTypeAPIKey, row.CredentialType)
	require.Equal(t, entity.CredentialStateValid, row.State)

	// The issuance is attributed to the owner, which is what creation records.
	entries, err := db.GetCredentialLifecycleJournalEntriesBySubject(ctx,
		database.GetCredentialLifecycleJournalEntriesBySubjectParams{
			Subject: mirrored.ID,
			Limit:   2,
		})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, owner.ID, entries[0].Actor)

	// The 24 hour lifetime survives the move. A mirror that wrote its
	// stand-in for never would pass every other assertion here.
	require.WithinRange(t, key.ExpiresAt, before.Add(23*time.Hour), dbtime.Now().Add(25*time.Hour))
}
