package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

// TestSubagentSpawnUnderArchivedRoot covers a spawn_agent call that is
// still in flight after its family was archived, for example from a
// runner that lost its lease and has not noticed yet. The spawn must
// fail instead of creating a running, unarchived child under the
// archived root, which would break the family archive invariant that
// ArchiveChat documents.
func TestSubagentSpawnUnderArchivedRoot(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	// The root runs a turn; its spawn_agent call reads the parent now.
	root := createInternalParentChat(ctx, t, server, db, org.ID, user.ID, model.ID, "root")
	require.Equal(t, database.ChatStatusRunning, root.Status)
	inFlightSnapshot := root

	// The turn ends (another owner finished it) and the user archives
	// the family.
	require.NoError(t, chatstate.NewChatMachine(db, ps, root.ID).Update(ctx,
		func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
			return err
		}))
	require.NoError(t, server.ArchiveChat(ctx, root))

	// The in-flight spawn_agent call continues.
	parent, err := server.loadSubagentSpawnParentChat(ctx, func() database.Chat { return inFlightSnapshot })
	require.NoError(t, err)
	require.True(t, parent.Archived, "the reloaded parent is archived")
	_, err = server.createChildSubagentChatWithOptions(ctx, parent, "late child", "", childSubagentChatOptions{})
	require.ErrorIs(t, err, chatstate.ErrChatFamilyArchived)
	// spawn_agent returns this text to the model as the tool error.
	require.ErrorContains(t, err, "cannot create a child agent because the parent chat is archived")

	familyIDs, err := db.GetChatFamilyIDsByRootID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{root.ID}, familyIDs, "no child may join the archived family")
}
