package chatstate_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
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

// TestEndChatArchivesChildren verifies that ending a root chat
// archives its whole family in the same transaction, including
// running children and their queued backlogs, so the
// parent-archived-implies-child-archived invariant holds at write
// time.
func TestEndChatArchivesChildren(t *testing.T) {
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
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	grandchild := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "grandchild",
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: child.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
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
	machine := chatstate.NewChatMachine(db, pub, root.ID)
	var endResult chatstate.EndChatResult
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		endResult, err = tx.EndChat(chatstate.EndChatInput{})
		return err
	}))

	for _, chatID := range []uuid.UUID{root.ID, child.ID, grandchild.ID} {
		after, err := db.GetChatByID(ctx, chatID)
		require.NoError(t, err)
		require.True(t, after.Archived, "family member must be archived")
		require.Equal(t, database.ChatStatusWaiting, after.Status)
		require.False(t, after.WorkerID.Valid, "worker ownership must be cleared")
		require.False(t, after.RunnerID.Valid, "runner ownership must be cleared")
	}
	count, err := db.CountChatQueuedMessages(ctx, child.ID)
	require.NoError(t, err)
	require.Zero(t, count, "child queue must be cleared")
	for _, chatID := range []uuid.UUID{child.ID, grandchild.ID} {
		require.Contains(t, pub.channels, coderdpubsub.ChatStateUpdateChannel(chatID),
			"ended child must publish a chat:update")
	}
	endedIDs := make([]uuid.UUID, 0, len(endResult.EndedDescendants))
	for _, desc := range endResult.EndedDescendants {
		require.True(t, desc.Archived, "returned descendant must carry the post-transition row")
		endedIDs = append(endedIDs, desc.ID)
	}
	require.ElementsMatch(t, []uuid.UUID{child.ID, grandchild.ID}, endedIDs,
		"cascade must surface newly ended descendants for caller side effects")
}

// Ending a child chat (for example via a lifecycle hook end_chat)
// archives that child and its descendants only; the root and siblings
// stay active.
func TestEndChatOnChildArchivesSubtreeOnly(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	user, org, model := seedFamilyDeps(t, db)

	root := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "root",
		Status:            database.ChatStatusRunning,
	})
	child := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "child",
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	grandchild := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "grandchild",
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: child.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
	sibling := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "sibling",
		Status:            database.ChatStatusRunning,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})

	pub := newRecordingPubsub()
	machine := chatstate.NewChatMachine(db, pub, child.ID)
	var endResult chatstate.EndChatResult
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		endResult, err = tx.EndChat(chatstate.EndChatInput{})
		return err
	}))

	for _, chatID := range []uuid.UUID{child.ID, grandchild.ID} {
		after, err := db.GetChatByID(ctx, chatID)
		require.NoError(t, err)
		require.True(t, after.Archived, "subtree member must be archived")
		require.Equal(t, database.ChatStatusWaiting, after.Status)
		require.False(t, after.WorkerID.Valid)
		require.False(t, after.RunnerID.Valid)
	}
	for _, chatID := range []uuid.UUID{root.ID, sibling.ID} {
		after, err := db.GetChatByID(ctx, chatID)
		require.NoError(t, err)
		require.False(t, after.Archived, "root and sibling must stay active")
		require.Equal(t, database.ChatStatusRunning, after.Status)
	}
	require.Contains(t, pub.channels, coderdpubsub.ChatStateUpdateChannel(grandchild.ID),
		"ended grandchild must publish a chat:update")
	endedIDs := make([]uuid.UUID, 0, len(endResult.EndedDescendants))
	for _, desc := range endResult.EndedDescendants {
		endedIDs = append(endedIDs, desc.ID)
	}
	require.ElementsMatch(t, []uuid.UUID{grandchild.ID}, endedIDs,
		"subtree cascade must surface only newly ended descendants")
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
