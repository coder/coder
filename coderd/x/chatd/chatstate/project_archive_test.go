package chatstate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestArchiveProjectChats archives a project whose families have a root or a
// subagent chat in each unarchived execution state, then unarchives it. It
// checks the archived state each chat lands in, that queued messages are
// kept, that only chats archived with the project come back, and that active
// chats need StopActiveChats.
func TestArchiveProjectChats(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)

	cases := []struct {
		state    chatstate.ExecutionState
		active   bool
		archived chatstate.ExecutionState
	}{
		{state: chatstate.StateW, archived: chatstate.StateXW},
		{state: chatstate.StateE0, archived: chatstate.StateXE0},
		{state: chatstate.StateE1, archived: chatstate.StateXE1},
		{state: chatstate.StateE1P, archived: chatstate.StateXE1P},
		{state: chatstate.StateR0, active: true, archived: chatstate.StateXW},
		{state: chatstate.StateR1, active: true, archived: chatstate.StateXE1},
		{state: chatstate.StateR1P, active: true, archived: chatstate.StateXE1P},
		{state: chatstate.StateI0, active: true, archived: chatstate.StateXW},
		{state: chatstate.StateI1, active: true, archived: chatstate.StateXE1},
		{state: chatstate.StateI1P, active: true, archived: chatstate.StateXE1P},
		{state: chatstate.StateA0, active: true, archived: chatstate.StateXW},
		{state: chatstate.StateA1, active: true, archived: chatstate.StateXE1},
		{state: chatstate.StateA1P, active: true, archived: chatstate.StateXE1P},
		{state: chatstate.StateP, active: true, archived: chatstate.StateXE1P},
	}
	unarchivedState := map[chatstate.ExecutionState]chatstate.ExecutionState{
		chatstate.StateXW:   chatstate.StateW,
		chatstate.StateXE0:  chatstate.StateE0,
		chatstate.StateXE1:  chatstate.StateE1,
		chatstate.StateXE1P: chatstate.StateE1P,
	}
	for _, tc := range cases {
		t.Run(tc.state.String(), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			project := dbgen.ChatProject(t, f.DB, database.ChatProject{OrganizationID: f.Org.ID, OwnerID: f.User.ID})
			inProject := uuid.NullUUID{UUID: project.ID, Valid: true}

			// One family has the state on its root, the other on its
			// subagent chat, which has no project_id of its own.
			rootInState := seedChatInState(ctx, t, f, inProject, uuid.Nil, tc.state)
			idleChild := seedChatInState(ctx, t, f, uuid.NullUUID{}, rootInState, chatstate.StateW)
			idleRoot := seedChatInState(ctx, t, f, inProject, uuid.Nil, chatstate.StateW)
			childInState := seedChatInState(ctx, t, f, uuid.NullUUID{}, idleRoot, tc.state)
			// A family archived before the project stays archived after
			// the project is unarchived.
			archivedEarlier := seedChatInState(ctx, t, f, inProject, uuid.Nil, chatstate.StateW)
			_, err := chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: archivedEarlier, Archived: true})
			require.NoError(t, err)
			// A chat in another project is not touched.
			otherProject := dbgen.ChatProject(t, f.DB, database.ChatProject{OrganizationID: f.Org.ID, OwnerID: f.User.ID})
			outside := seedChatInState(ctx, t, f, uuid.NullUUID{UUID: otherProject.ID, Valid: true}, uuid.Nil, tc.state)

			inStateChats := []uuid.UUID{rootInState, childInState}
			idleChats := []uuid.UUID{idleChild, idleRoot}
			queueLen := map[uuid.UUID]int64{}
			for _, id := range append(append([]uuid.UUID{outside}, inStateChats...), idleChats...) {
				queueLen[id] = countQueued(ctx, t, f, id)
			}

			// Subtests share f, so each records its own publications.
			pub := newRecordingPubsub()
			_, _, err = chatstate.ArchiveProjectChats(ctx, f.DB, pub, chatstate.ArchiveProjectChatsInput{ProjectID: project.ID})
			if tc.active {
				var activeErr *chatstate.ChatProjectActiveChatsError
				require.ErrorAs(t, err, &activeErr)
				require.EqualValues(t, 2, activeErr.ActiveChatCount, "both families have an active member")
				for _, id := range inStateChats {
					require.Equal(t, tc.state, f.classify(ctx, t, id), "a refused archive changes nothing")
				}
				notArchived, getErr := f.DB.GetChatProjectByID(ctx, project.ID)
				require.NoError(t, getErr)
				require.False(t, notArchived.ArchivedAt.Valid)

				require.Empty(t, pub.channels, "a refused archive publishes nothing")
				_, _, err = chatstate.ArchiveProjectChats(ctx, f.DB, pub, chatstate.ArchiveProjectChatsInput{ProjectID: project.ID, StopActiveChats: true})
			}
			require.NoError(t, err)

			archivedProject, err := f.DB.GetChatProjectByID(ctx, project.ID)
			require.NoError(t, err)
			require.True(t, archivedProject.ArchivedAt.Valid)
			for _, id := range inStateChats {
				require.Equal(t, tc.archived, f.classify(ctx, t, id))
			}
			for _, id := range idleChats {
				require.Equal(t, chatstate.StateXW, f.classify(ctx, t, id))
			}
			for _, id := range append(append([]uuid.UUID{}, inStateChats...), idleChats...) {
				chat := f.readChat(ctx, t, id)
				require.True(t, chat.ArchivedWithProject)
				require.False(t, chat.WorkerID.Valid)
				require.False(t, chat.RunnerID.Valid)
				require.Equal(t, queueLen[id], countQueued(ctx, t, f, id), "archiving keeps queued messages")
				// A chat:update carries the archive to the chat's runner.
				pub.expectChatUpdate(t, id, chat.SnapshotVersion)
				if tc.state != chatstate.StateE1 && tc.state != chatstate.StateE1P && queueLen[id] > 0 {
					require.Equal(t, "Stopped when the project was archived.", lastErrorMessage(t, chat))
				}
			}
			require.Equal(t, tc.state, f.classify(ctx, t, outside), "a chat in another project is not archived")
			require.False(t, f.readChat(ctx, t, archivedEarlier).ArchivedWithProject)

			_, _, err = chatstate.UnarchiveProjectChats(ctx, f.DB, pub, project.ID)
			require.NoError(t, err)
			for _, id := range inStateChats {
				require.Equal(t, unarchivedState[tc.archived], f.classify(ctx, t, id))
				require.False(t, f.readChat(ctx, t, id).ArchivedWithProject)
			}
			for _, id := range idleChats {
				require.Equal(t, chatstate.StateW, f.classify(ctx, t, id))
			}
			require.Equal(t, chatstate.StateXW, f.classify(ctx, t, archivedEarlier), "a chat archived before the project stays archived")
		})
	}
}

// TestArchivedChatProjectRejectsChats checks that a chat cannot join an
// archived project, by creation or by a move, and cannot be unarchived on
// its own while its project is archived.
func TestArchivedChatProjectRejectsChats(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	project := dbgen.ChatProject(t, f.DB, database.ChatProject{OrganizationID: f.Org.ID, OwnerID: f.User.ID})
	inProject := uuid.NullUUID{UUID: project.ID, Valid: true}
	member := seedChatInState(ctx, t, f, inProject, uuid.Nil, chatstate.StateW)
	outsider := seedChatInState(ctx, t, f, uuid.NullUUID{}, uuid.Nil, chatstate.StateW)
	_, _, err := chatstate.ArchiveProjectChats(ctx, f.DB, f.Pub, chatstate.ArchiveProjectChatsInput{ProjectID: project.ID})
	require.NoError(t, err)

	_, err = chatstate.CreateChat(ctx, f.DB, f.Pub, chatstate.CreateChatInput{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		ProjectID:         inProject,
		LastModelConfigID: f.Model.ID,
		Title:             "new",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages:   []chatstate.Message{userTextMessage("hello", f.User.ID, f.Model.ID)},
	})
	require.ErrorIs(t, err, chatstate.ErrChatProjectArchived)

	_, err = chatstate.SetFamilyProject(ctx, f.DB, f.Pub, chatstate.SetFamilyProjectInput{RootID: outsider, ProjectID: inProject})
	require.ErrorIs(t, err, chatstate.ErrChatProjectArchived)
	require.False(t, f.readChat(ctx, t, outsider).ProjectID.Valid)

	_, err = chatstate.SetFamilyArchived(ctx, f.DB, f.Pub, chatstate.SetFamilyArchivedInput{RootID: member, Archived: false})
	require.ErrorIs(t, err, chatstate.ErrChatProjectArchived)
	require.True(t, f.readChat(ctx, t, member).Archived)
}

// seedChatInState inserts a chat with the status and queue that classify as
// state. A non-nil parentID makes it a subagent chat of that root.
func seedChatInState(ctx context.Context, t *testing.T, f *testFixture, projectID uuid.NullUUID, parentID uuid.UUID, state chatstate.ExecutionState) uuid.UUID {
	t.Helper()
	status := map[byte]database.ChatStatus{
		'W': database.ChatStatusWaiting,
		'E': database.ChatStatusError,
		'R': database.ChatStatusRunning,
		'I': database.ChatStatusInterrupting,
		'A': database.ChatStatusRequiresAction,
		'P': database.ChatStatusPaused,
	}[state.String()[0]]
	seed := database.Chat{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		ProjectID:         projectID,
		Status:            status,
	}
	if parentID != uuid.Nil {
		seed.ParentChatID = uuid.NullUUID{UUID: parentID, Valid: true}
		seed.RootChatID = seed.ParentChatID
	}
	chat := dbgen.Chat(t, f.DB, seed)
	name := state.String()
	queued := name == "P" || name[len(name)-1] == '1' || name[len(name)-1] == 'P'
	if queued {
		head, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
			ChatID:       chat.ID,
			Content:      json.RawMessage(`[]`),
			CreatedBy:    f.User.ID,
			BusyBehavior: database.ChatBusyBehaviorQueue,
		})
		require.NoError(t, err)
		if name == "P" || name[len(name)-1] == 'P' {
			_, err = f.DB.UpdateChatQueuedMessageEditing(ctx, database.UpdateChatQueuedMessageEditingParams{
				Editing: true,
				ID:      head.ID,
				ChatID:  chat.ID,
			})
			require.NoError(t, err)
		}
	}
	require.Equal(t, state, f.classify(ctx, t, chat.ID))
	return chat.ID
}

func countQueued(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) int64 {
	t.Helper()
	count, err := f.DB.CountChatQueuedMessages(ctx, chatID)
	require.NoError(t, err)
	return count
}

func lastErrorMessage(t *testing.T, chat database.Chat) string {
	t.Helper()
	require.True(t, chat.LastError.Valid)
	var chatErr codersdk.ChatError
	require.NoError(t, json.Unmarshal(chat.LastError.RawMessage, &chatErr))
	return chatErr.Message
}
