package chatstate_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// These tests cover the hold from the point of view of the passive
// promotion paths. The matrix test proves each transition cell; these
// prove the end-to-end contract: a held row and everything behind it
// stay queued while rows ahead of it drain, and explicit actions go
// around the hold.

func finishTurn(t *testing.T, f *testFixture, m *chatstate.ChatMachine) chatstate.FinishTurnResult {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var res chatstate.FinishTurnResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	return res
}

func holdViaTransition(t *testing.T, f *testFixture, m *chatstate.ChatMachine, id int64, held bool) chatstate.EditQueuedMessageResult {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var res chatstate.EditQueuedMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID: id,
			Held:            &held,
		})
		return err
	}))
	return res
}

// TestHold_FinishTurnStopsAtHeldRow queues five rows, holds the third,
// and finishes turns until the chat idles. Rows one and two are
// promoted in order; the chat then lands in W with rows three to five
// still queued and the hold intact.
func TestHold_FinishTurnStopsAtHeldRow(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	chatID := created.Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	var ids []int64
	var bodies []string
	for i := 1; i <= 5; i++ {
		body := fmt.Sprintf("row-%d", i)
		sm := sendQueuedMessage(t, f, m, body)
		require.NotNil(t, sm.QueuedMessage)
		ids = append(ids, sm.QueuedMessage.ID)
		bodies = append(bodies, body)
	}
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))

	held := holdViaTransition(t, f, m, ids[2], true)
	require.True(t, held.QueuedMessage.HeldAt.Valid)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID), "a hold behind the head changes nothing yet")

	first := finishTurn(t, f, m)
	require.NotNil(t, first.PromotedMessage)
	assertChatMessageText(t, *first.PromotedMessage, bodies[0])
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))

	second := finishTurn(t, f, m)
	require.NotNil(t, second.PromotedMessage)
	assertChatMessageText(t, *second.PromotedMessage, bodies[1])
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, chatID), "held row reached the head")

	third := finishTurn(t, f, m)
	require.Nil(t, third.PromotedMessage, "a held head is not promoted")
	require.Equal(t, database.ChatStatusWaiting, third.Chat.Status)
	require.Equal(t, chatstate.StateW, f.classify(ctx, t, chatID))
	require.Equal(t, ids[2:], queuedIDsByPosition(ctx, t, f, chatID), "rows three to five stay queued")
	assertQueueBodiesInOrder(ctx, t, f, chatID, bodies[2:])
	head := requireQueuedMessageByID(ctx, t, f, chatID, ids[2])
	require.True(t, head.HeldAt.Valid, "hold survives the turn boundary")

	// W with a held head is a valid idle state; FinishTurn is not
	// allowed from W, which is what stops the runner from spinning.
	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	// Releasing the held head of an idle chat is refused: it would leave
	// W with a promotable head. Starting it is PromoteQueuedMessage's
	// job, which clears the hold, pops row three, and leaves four and
	// five promotable behind it.
	releaseHeld := false
	err = m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID: ids[2],
			Held:            &releaseHeld,
		})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrIdleHeadWouldBecomePromotable)
	require.Equal(t, chatstate.StateW, f.classify(ctx, t, chatID))

	var promoted chatstate.PromoteQueuedMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		promoted, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{QueuedMessageID: ids[2]})
		return err
	}))
	require.NotNil(t, promoted.InsertedMessage)
	assertChatMessageText(t, *promoted.InsertedMessage, bodies[2])
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))
	require.Equal(t, ids[3:], queuedIDsByPosition(ctx, t, f, chatID))
}

// TestHold_FinishInterruptionStopsAtHeldRow interrupts a running chat
// whose head is held and verifies the interruption lands in W instead
// of promoting the held row.
func TestHold_FinishInterruptionStopsAtHeldRow(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	chatID := created.Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	queued := sendInterruptMessage(t, f, m, "interrupting")
	require.NotNil(t, queued.QueuedMessage)
	require.Equal(t, chatstate.StateI1, f.classify(ctx, t, chatID))

	holdViaTransition(t, f, m, queued.QueuedMessage.ID, true)
	require.Equal(t, chatstate.StateI0, f.classify(ctx, t, chatID))

	var res chatstate.FinishInterruptionResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.FinishInterruption(chatstate.FinishInterruptionInput{})
		return err
	}))
	require.Nil(t, res.PromotedMessage)
	require.Equal(t, chatstate.StateW, f.classify(ctx, t, chatID))
	row := requireQueuedMessageByID(ctx, t, f, chatID, queued.QueuedMessage.ID)
	require.True(t, row.HeldAt.Valid)
}

// TestHold_DirectSendGoesAroundHeldQueue sends a message to an idle
// chat whose queue head is held. The new message is inserted directly
// into history and the held rows stay queued and held.
func TestHold_DirectSendGoesAroundHeldQueue(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedHeldHead(t, f, chatstate.StateW, 1)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	baseHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)

	sm := sendQueuedMessage(t, f, m, "direct")
	require.Nil(t, sm.QueuedMessage, "idle chat inserts directly")
	require.Len(t, sm.InsertedMessages, 1)
	assertChatMessageText(t, sm.InsertedMessages[0], "direct")
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, seeded.chatID), "held head still hides the queue")
	require.Equal(t, seeded.queuedMessageIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID), "held rows untouched")
	head := requireQueuedMessageByID(ctx, t, f, seeded.chatID, seeded.queuedMessageIDs[0])
	require.True(t, head.HeldAt.Valid)
	require.Len(t, activeHistoryIDs(ctx, t, f, seeded.chatID), len(baseHistory)+1)

	// The interrupting send on the now running chat appends to the
	// tail, behind the held half.
	interrupt := sendInterruptMessage(t, f, m, "interrupt")
	require.NotNil(t, interrupt.QueuedMessage)
	require.Equal(t, chatstate.StateI0, f.classify(ctx, t, seeded.chatID))
	want := append(append([]int64{}, seeded.queuedMessageIDs...), interrupt.QueuedMessage.ID)
	require.Equal(t, want, queuedIDsByPosition(ctx, t, f, seeded.chatID))
}

// TestHold_StaleChatsIgnoresHeldHead pins the GetStaleChats predicate: a
// waiting chat whose queue head is held is idle, not stranded, even with
// unheld rows behind the held one. Only a promotable head in waiting is
// reported.
func TestHold_StaleChatsIgnoresHeldHead(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedHeldHead(t, f, chatstate.StateW, 1)
	// A threshold in the future makes every chat old enough, so only the
	// queue predicate decides.
	threshold := time.Now().Add(time.Hour)

	containsChat := func(chats []database.Chat, id uuid.UUID) bool {
		for _, c := range chats {
			if c.ID == id {
				return true
			}
		}
		return false
	}

	//nolint:gocritic // GetStaleChats is a system sweep query.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	stale, err := f.DB.GetStaleChats(sysCtx, threshold)
	require.NoError(t, err)
	require.False(t, containsChat(stale, seeded.chatID), "waiting with a held head is not stranded")

	// Force the invalid shape (waiting with a promotable head) directly.
	_, err = f.DB.UpdateChatQueuedMessageHeld(ctx, database.UpdateChatQueuedMessageHeldParams{
		ChatID: seeded.chatID,
		ID:     seeded.queuedMessageIDs[0],
		Held:   false,
	})
	require.NoError(t, err)
	stale, err = f.DB.GetStaleChats(sysCtx, threshold)
	require.NoError(t, err)
	require.True(t, containsChat(stale, seeded.chatID), "waiting with a promotable head is stranded")
}

// TestHold_QueueListingFollowsProcessingOrder pins that the client-visible
// queue (GetChatQueuedMessages) is ordered by position like the state
// machine, so the paused tail a client derives from a held row matches
// what the server will actually process next. "Send now" on a row
// behind the head is the case where created_at order diverges.
func TestHold_QueueListingFollowsProcessingOrder(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	first := sendQueuedMessage(t, f, m, "first")
	second := sendQueuedMessage(t, f, m, "second")
	require.NotNil(t, first.QueuedMessage)
	require.NotNil(t, second.QueuedMessage)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
			QueuedMessageID: second.QueuedMessage.ID,
		})
		return err
	}))

	listed, err := f.DB.GetChatQueuedMessages(ctx, created.Chat.ID)
	require.NoError(t, err)
	listedIDs := make([]int64, 0, len(listed))
	for _, row := range listed {
		listedIDs = append(listedIDs, row.ID)
	}
	require.Equal(t, queuedIDsByPosition(ctx, t, f, created.Chat.ID), listedIDs)
	require.Equal(t, []int64{second.QueuedMessage.ID, first.QueuedMessage.ID}, listedIDs)
}

// TestHold_EditQueuedMessageRejectsEmptyInput pins that a PATCH with
// neither content nor held is a transition error, not a silent no-op.
func TestHold_EditQueuedMessageRejectsEmptyInput(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	queued := sendQueuedMessage(t, f, m, "queued")
	require.NotNil(t, queued.QueuedMessage)

	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID: queued.QueuedMessage.ID,
		})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	err = m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		held := true
		_, err := tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID: queued.QueuedMessage.ID + 1000,
			Held:            &held,
		})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrQueuedMessageNotFound)
}

// TestHold_ContentEditKeepsOverridesUnlessGiven verifies the override
// rules: content-only edits keep the row's model and effort; explicit
// overrides replace them.
func TestHold_ContentEditKeepsOverridesUnlessGiven(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	queued := sendQueuedMessage(t, f, m, "queued")
	require.NotNil(t, queued.QueuedMessage)
	require.True(t, queued.QueuedMessage.ModelConfigID.Valid)

	var res chatstate.EditQueuedMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID: queued.QueuedMessage.ID,
			Content:         userMessageContent(t, "edited"),
		})
		return err
	}))
	assertQueuedMessageText(t, res.QueuedMessage, "edited")
	require.Equal(t, queued.QueuedMessage.ModelConfigID, res.QueuedMessage.ModelConfigID)
	require.Equal(t, queued.QueuedMessage.ReasoningEffort, res.QueuedMessage.ReasoningEffort)

	effort := database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true}
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{
			QueuedMessageID:         queued.QueuedMessage.ID,
			Content:                 userMessageContent(t, "edited again"),
			ReasoningEffortOverride: effort,
		})
		return err
	}))
	assertQueuedMessageText(t, res.QueuedMessage, "edited again")
	require.Equal(t, effort, res.QueuedMessage.ReasoningEffort)
	require.Equal(t, queued.QueuedMessage.ModelConfigID, res.QueuedMessage.ModelConfigID)
}
