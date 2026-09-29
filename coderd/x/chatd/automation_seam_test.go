package chatd_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestServerAdmitInTx covers that chatd forwards the admission seam on
// chat creation and message sends, and that a refused admission rolls
// back everything the call would have written.
func TestServerAdmitInTx(t *testing.T) {
	t.Parallel()
	errAdmit := xerrors.New("admission refused")
	refuse := func(called *uuid.UUID) chatstate.AdmitFunc {
		return func(_ context.Context, _ database.Store, chatID uuid.UUID) (chatstate.AutomationProvenance, error) {
			*called = chatID
			return chatstate.AutomationProvenance{}, errAdmit
		}
	}

	t.Run("CreateRefused", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)

		var called uuid.UUID
		_, err := server.CreateChat(ctx, chatd.CreateOptions{
			OrganizationID:     org.ID,
			OwnerID:            user.ID,
			Title:              "automation",
			ModelConfigID:      model.ID,
			InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
			AdmitInTx:          refuse(&called),
		})
		require.ErrorIs(t, err, errAdmit)
		require.NotEqual(t, uuid.Nil, called)
		_, err = db.GetChatByID(ctx, called)
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("SendIdle", func(t *testing.T) {
		t.Parallel()
		db, ps := dbtestutil.NewDB(t)
		server := newTestServer(t, db, ps, uuid.New())
		ctx := testutil.Context(t, testutil.WaitLong)
		user, org, model := seedChatDependencies(t, db)
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "automation",
			Status:            database.ChatStatusWaiting,
		})
		send := func(admit chatstate.AdmitFunc) (chatd.SendMessageResult, error) {
			return server.SendMessage(ctx, chatd.SendMessageOptions{
				ChatID:    chat.ID,
				CreatedBy: user.ID,
				Content:   []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
				AdmitInTx: admit,
			})
		}

		var called uuid.UUID
		_, err := send(refuse(&called))
		require.ErrorIs(t, err, errAdmit)
		require.Equal(t, chat.ID, called)
		require.Empty(t, chatMessages(ctx, t, db, chat.ID))
		current, err := db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, database.ChatStatusWaiting, current.Status)

		provenance := chatstate.AutomationProvenance{AutomationID: uuid.New(), InputID: uuid.New(), QueueGeneration: 1}
		result, err := send(func(context.Context, database.Store, uuid.UUID) (chatstate.AutomationProvenance, error) {
			return provenance, nil
		})
		require.NoError(t, err)
		require.False(t, result.Queued)
		require.Equal(t, uuid.NullUUID{UUID: provenance.AutomationID, Valid: true}, result.Message.AutomationID)
		require.Equal(t, uuid.NullUUID{UUID: provenance.InputID, Valid: true}, result.Message.InputID)
	})
}

// TestPromoteQueuedRejectsStaleAutomationRow covers the promote endpoint
// contract for a row that fails the queue promotion guard: the caller
// gets not found, the row is gone, and nothing else changed.
func TestPromoteQueuedRejectsStaleAutomationRow(t *testing.T) {
	t.Parallel()
	db, ps := dbtestutil.NewDB(t)
	server := newTestServer(t, db, ps, uuid.New())
	ctx := testutil.Context(t, testutil.WaitLong)
	user, org, model := seedChatDependencies(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "automation",
		Status:            database.ChatStatusError,
	})
	ordinary := insertQueuedMessage(ctx, t, db, chat.ID, user.ID, model.ID, "ordinary")
	automation := dbgen.ChatAutomation(t, db, database.ChatAutomation{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		Enabled:        true,
	})
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText("stale")})
	require.NoError(t, err)
	stale, err := db.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:          chat.ID,
		Content:         content.RawMessage,
		ModelConfigID:   uuid.NullUUID{UUID: model.ID, Valid: true},
		CreatedBy:       user.ID,
		AutomationID:    uuid.NullUUID{UUID: automation.ID, Valid: true},
		InputID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
		QueueGeneration: sql.NullInt64{Int64: automation.QueueGeneration, Valid: true},
	})
	require.NoError(t, err)
	require.NoError(t, db.DeleteChatAutomationByID(ctx, automation.ID))
	before, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)

	_, err = server.PromoteQueued(ctx, chatd.PromoteQueuedOptions{
		ChatID:          chat.ID,
		QueuedMessageID: stale.ID,
	})
	require.ErrorIs(t, err, chatstate.ErrQueuedMessageNotFound)

	rows, err := db.GetChatQueuedMessages(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the stale row is deleted")
	require.Equal(t, ordinary.ID, rows[0].ID, "the other row is not reordered or promoted")
	require.Empty(t, chatMessages(ctx, t, db, chat.ID))
	after, err := db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, before.Status, after.Status)
	require.Equal(t, before.LastError, after.LastError)
}
