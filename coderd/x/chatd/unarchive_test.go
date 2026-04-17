package chatd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/testutil"
)

// TestUnarchiveChildChatAtomic covers the happy path, the
// parent-archived reject path, the no-op idempotent path, and the
// non-child reject path. The atomicity guarantee (that a concurrent
// ArchiveChatByID cascade serializes behind our row lock) is
// reasoned about in the helper's doc comment; asserting it in a
// reproducible test would require an artificial pause hook inside
// the transaction that does not exist in this codebase.
func TestUnarchiveChildChatAtomic(t *testing.T) {
	t.Parallel()

	t.Run("ChildWithActiveParentUnarchives", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)
		user, org, model := seedChatDependencies(ctx, t, db)

		parent, child := insertParentWithArchivedChild(ctx, t, db, user, org, model)

		updated, err := chatd.UnarchiveChildChatAtomic(ctx, db, child)
		require.NoError(t, err)
		require.Len(t, updated, 1)
		assert.Equal(t, child.ID, updated[0].ID)
		assert.False(t, updated[0].Archived, "returned row reflects unarchived state")

		dbChild, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
		require.NoError(t, err)
		assert.False(t, dbChild.Archived, "child should be unarchived in DB")

		dbParent, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), parent.ID)
		require.NoError(t, err)
		assert.False(t, dbParent.Archived, "parent should stay active")
	})

	t.Run("ChildWithArchivedParentRejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)
		user, org, model := seedChatDependencies(ctx, t, db)

		parent, child := insertParentWithArchivedChild(ctx, t, db, user, org, model)
		// Archive the parent so the family is consistently
		// archived, then try to unarchive the child alone.
		_, err := db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), parent.ID)
		require.NoError(t, err)

		updated, err := chatd.UnarchiveChildChatAtomic(ctx, db, child)
		require.ErrorIs(t, err, chatd.ErrChildUnarchiveParentArchived)
		assert.Empty(t, updated)

		dbChild, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
		require.NoError(t, err)
		assert.True(t, dbChild.Archived, "child should remain archived")
	})

	t.Run("AlreadyActiveChildNoOp", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)
		user, org, model := seedChatDependencies(ctx, t, db)

		_, child := insertParentWithActiveChild(ctx, t, db, user, org, model)

		updated, err := chatd.UnarchiveChildChatAtomic(ctx, db, child)
		require.NoError(t, err)
		assert.Empty(t, updated, "already-active child should produce no updates")
	})

	t.Run("NotAChildRejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		root := database.Chat{ID: uuid.New()}
		_, err := chatd.UnarchiveChildChatAtomic(ctx, db, root)
		require.Error(t, err)
		assert.NotErrorIs(t, err, chatd.ErrChildUnarchiveParentArchived)
	})
}

// insertParentWithActiveChild creates a parent chat and an active
// child chat linked to it. Both are returned in their initial
// (active) state. The named return values make it explicit which
// chat is which at the call site.
func insertParentWithActiveChild(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	user database.User,
	org database.Organization,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()
	var err error
	parent, err = db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		LastModelConfigID: model.ID,
		Title:             "parent",
	})
	require.NoError(t, err)
	child, err = db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		LastModelConfigID: model.ID,
		Title:             "child",
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
	})
	require.NoError(t, err)
	return parent, child
}

// insertParentWithArchivedChild creates an active parent and an
// individually-archived child. The returned child reflects its
// current (archived) state in the DB. The named return values make
// it explicit which chat is which at the call site.
func insertParentWithArchivedChild(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	user database.User,
	org database.Organization,
	model database.ChatModelConfig,
) (parent database.Chat, child database.Chat) {
	t.Helper()
	parent, child = insertParentWithActiveChild(ctx, t, db, user, org, model)
	_, err := db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
	require.NoError(t, err)
	child, err = db.GetChatByID(dbauthz.AsSystemRestricted(ctx), child.ID)
	require.NoError(t, err)
	return parent, child
}
