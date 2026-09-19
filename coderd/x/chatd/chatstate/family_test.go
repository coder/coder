package chatstate_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestSetFamilyArchivedRejectsChildChat asserts the chatstate helper
// rejects calls that target a child chat. Family archive flows must
// always start at the root.
func TestSetFamilyArchivedRejectsChildChat(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	root := dbgen.Chat(t, f.DB, database.Chat{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "root",
	})
	child := dbgen.Chat(t, f.DB, database.Chat{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "child",
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	_, err := chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: child.ID, Archived: true})
	require.ErrorIs(t, err, chatstate.ErrChatNotRoot)

	require.False(t, f.readChat(ctx, t, root.ID).Archived,
		"failed family archive must not touch the root")
	require.False(t, f.readChat(ctx, t, child.ID).Archived,
		"failed family archive must not touch the child")
}

// TestSetFamilyArchivedRollsBackWhenMemberCannotArchive verifies that
// SetFamilyArchived is atomic: when one family member is in a state
// that cannot satisfy the SetArchived transition, the whole cascade
// rolls back and no publications reach the inner publisher.
func TestSetFamilyArchivedRollsBackWhenMemberCannotArchive(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user, org, model := seedFamilyDeps(t, db)

	// Root chat: waiting is archive-eligible (state W).
	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
	})
	// Child chat: running with no queue is R0 and NOT archive
	// eligible per the chatstate transition matrix.
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	pub := newRecordingPubsub()
	_, err := chatstate.SetFamilyArchived(ctx, db, pub, chatstate.SetFamilyArchivedInput{RootID: root.ID, Archived: true})
	require.Error(t, err, "child in "+chatstate.StateR0.String()+" must reject SetArchived")
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	rootAfter, err := db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.False(t, rootAfter.Archived, "root archive must roll back when a child cannot archive")
	childAfter, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.False(t, childAfter.Archived, "child must not be archived in the rolled-back cascade")

	require.Empty(t, pub.channels,
		"rolled-back family archive must publish nothing through the inner publisher")
}

// TestSetFamilyArchivedRejectsInvalidStateEvenWhenAlreadyDesired
// verifies that invalid-state detection is never bypassed: a family
// member in StateInvalid causes the cascade to fail with
// ErrInvalidState even when that member's archived flag already
// matches the desired value.
func TestSetFamilyArchivedRejectsInvalidStateEvenWhenAlreadyDesired(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user, org, model := seedFamilyDeps(t, db)

	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
	})
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		// status=waiting, archived=true; we will add a queued message
		// to produce the chatstate-invalid combination (archived chat
		// with a queued backlog is outside the valid state model).
		Status:       database.ChatStatusWaiting,
		Archived:     true,
		ParentChatID: uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:   uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	// Seed a queued message under the child to push it into the
	// chatstate-invalid combination.
	rawContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("queued"),
	})
	require.NoError(t, err)
	_, err = db.InsertChatQueuedMessage(ctx, database.InsertChatQueuedMessageParams{
		ChatID:        child.ID,
		Content:       rawContent.RawMessage,
		ModelConfigID: uuid.NullUUID{},
	})
	require.NoError(t, err)

	pub := newRecordingPubsub()
	_, err = chatstate.SetFamilyArchived(ctx, db, pub, chatstate.SetFamilyArchivedInput{
		RootID:   root.ID,
		Archived: true,
	})
	require.ErrorIs(t, err, chatstate.ErrInvalidState,
		"invalid-state child blocks the cascade even when archived flag already matches")

	// Root must not be archived because the cascade rolled back.
	rootAfter, err := db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.False(t, rootAfter.Archived, "root must roll back when a child is in StateInvalid")

	require.Empty(t, pub.channels,
		"rolled-back cascade must not publish anything")
}

// TestSetFamilyArchivedAcceptsAlreadyDesiredMembers verifies that an
// individually archived child does not block a root archive cascade.
// The cascade converges to the desired state even when some family
// members already match it.
func TestSetFamilyArchivedAcceptsAlreadyDesiredMembers(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user, org, model := seedFamilyDeps(t, db)

	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusWaiting,
	})
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		Status:            database.ChatStatusWaiting,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
		Archived:          true,
	})

	pub := newRecordingPubsub()
	family, err := chatstate.SetFamilyArchived(ctx, db, pub, chatstate.SetFamilyArchivedInput{RootID: root.ID, Archived: true})
	require.NoError(t, err,
		"already archived members must not block the cascade")
	require.Len(t, family, 2)

	rootAfter, err := db.GetChatByID(ctx, root.ID)
	require.NoError(t, err)
	require.True(t, rootAfter.Archived)
	childAfter, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.True(t, childAfter.Archived)
}

func seedFamilyDeps(t *testing.T, db database.Store) (database.User, database.Organization, database.ChatModelConfig) {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "openai",
		DisplayName: "openai",
		BaseUrl:     "http://example.invalid",
	})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		IsDefault: true,
	})
	return user, org, model
}

// TestSetFamilyArchivedTree covers the tree rules: archive cascades over
// named descendants and their subagents atomically, the tree root cannot
// be archived, unarchive is blocked under an archived parent, and
// unarchive restores the chat and its subagents but not named children.
func TestSetFamilyArchivedTree(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	newChat := func(title string, kind database.ChatKind, parent uuid.NullUUID, root uuid.NullUUID, status database.ChatStatus) database.Chat {
		return dbgen.Chat(t, f.DB, database.Chat{
			OrganizationID:    f.Org.ID,
			OwnerID:           f.User.ID,
			LastModelConfigID: f.Model.ID,
			Title:             title,
			Kind:              kind,
			ParentChatID:      parent,
			RootChatID:        root,
			Status:            status,
		})
	}
	ref := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }

	treeRoot := newChat("root", database.ChatKindRoot, uuid.NullUUID{}, uuid.NullUUID{}, database.ChatStatusWaiting)
	top := newChat("top", database.ChatKindChat, ref(treeRoot.ID), uuid.NullUUID{}, database.ChatStatusWaiting)
	child := newChat("child", database.ChatKindChat, ref(top.ID), uuid.NullUUID{}, database.ChatStatusWaiting)
	grandchild := newChat("grandchild", database.ChatKindChat, ref(child.ID), uuid.NullUUID{}, database.ChatStatusRunning)
	topSubagent := newChat("top subagent", database.ChatKindSubagent, ref(top.ID), ref(top.ID), database.ChatStatusWaiting)
	childSubagent := newChat("child subagent", database.ChatKindSubagent, ref(child.ID), ref(child.ID), database.ChatStatusWaiting)

	_, err := chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: treeRoot.ID, Archived: true})
	require.ErrorIs(t, err, chatstate.ErrChatTreeRootArchive)

	// A running grandchild blocks the whole cascade.
	_, err = chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: top.ID, Archived: true})
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
	for _, id := range []uuid.UUID{top.ID, child.ID, grandchild.ID, topSubagent.ID, childSubagent.ID} {
		require.False(t, f.readChat(ctx, t, id).Archived)
	}

	_, err = f.DB.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
		ID:     grandchild.ID,
		Status: database.ChatStatusWaiting,
	})
	require.NoError(t, err)

	archived, err := chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: top.ID, Archived: true})
	require.NoError(t, err)
	require.Len(t, archived, 5)
	require.Equal(t, top.ID, archived[0].ID, "the top chat comes first")
	for _, id := range []uuid.UUID{top.ID, child.ID, grandchild.ID, topSubagent.ID, childSubagent.ID} {
		require.True(t, f.readChat(ctx, t, id).Archived)
	}
	require.False(t, f.readChat(ctx, t, treeRoot.ID).Archived)

	// Unarchiving below an archived parent is rejected.
	_, err = chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: child.ID, Archived: false})
	require.ErrorIs(t, err, chatstate.ErrChatParentArchived)

	// Unarchiving the top restores it and its subagent only.
	restored, err := chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: top.ID, Archived: false})
	require.NoError(t, err)
	require.Len(t, restored, 2)
	require.False(t, f.readChat(ctx, t, top.ID).Archived)
	require.False(t, f.readChat(ctx, t, topSubagent.ID).Archived)
	require.True(t, f.readChat(ctx, t, child.ID).Archived)
	require.True(t, f.readChat(ctx, t, grandchild.ID).Archived)
	require.True(t, f.readChat(ctx, t, childSubagent.ID).Archived)

	// Now the child can be unarchived on its own.
	_, err = chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: child.ID, Archived: false})
	require.NoError(t, err)
	require.False(t, f.readChat(ctx, t, child.ID).Archived)
	require.True(t, f.readChat(ctx, t, grandchild.ID).Archived)
}
