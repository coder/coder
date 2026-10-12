package chatstate_test

import (
	"context"
	"database/sql"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// newAutomation seeds an enabled automation owned by the fixture user.
func (f *testFixture) newAutomation(t *testing.T) database.ChatAutomation {
	t.Helper()
	return dbgen.ChatAutomation(t, f.DB, database.ChatAutomation{
		OrganizationID: f.Org.ID,
		OwnerID:        f.User.ID,
		Enabled:        true,
	})
}

// staleReason makes queued rows of an automation fail the queue
// promotion guard.
type staleReason struct {
	name  string
	apply func(ctx context.Context, t *testing.T, f *testFixture, automationID uuid.UUID)
}

var staleReasons = []staleReason{
	{name: "Deleted", apply: func(ctx context.Context, t *testing.T, f *testFixture, id uuid.UUID) {
		require.NoError(t, f.DB.DeleteChatAutomationByID(ctx, id))
	}},
	{name: "Disabled", apply: func(ctx context.Context, t *testing.T, f *testFixture, id uuid.UUID) {
		_, err := f.SQLDB.ExecContext(ctx,
			`UPDATE chat_automations SET enabled = false, queue_generation = queue_generation + 1 WHERE id = $1`, id)
		require.NoError(t, err)
	}},
	// DisabledSameGeneration isolates the enabled check: "Disabled" also
	// bumps the generation, so it alone cannot tell the checks apart.
	{name: "DisabledSameGeneration", apply: func(ctx context.Context, t *testing.T, f *testFixture, id uuid.UUID) {
		_, err := f.SQLDB.ExecContext(ctx,
			`UPDATE chat_automations SET enabled = false WHERE id = $1`, id)
		require.NoError(t, err)
	}},
	{name: "GenerationMismatch", apply: func(ctx context.Context, t *testing.T, f *testFixture, id uuid.UUID) {
		_, err := f.SQLDB.ExecContext(ctx,
			`UPDATE chat_automations SET queue_generation = queue_generation + 1 WHERE id = $1`, id)
		require.NoError(t, err)
	}},
}

func provenanceFor(automation database.ChatAutomation) chatstate.AutomationProvenance {
	return chatstate.AutomationProvenance{
		AutomationID:    automation.ID,
		InputID:         uuid.New(),
		QueueGeneration: automation.QueueGeneration,
	}
}

// queueAutomationMessage queues one message carrying automation
// provenance through SendMessage on a busy chat.
func queueAutomationMessage(
	t *testing.T,
	f *testFixture,
	m *chatstate.ChatMachine,
	body string,
	provenance chatstate.AutomationProvenance,
) database.ChatQueuedMessage {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	message := userTextMessage(body, f.User.ID, f.Model.ID)
	message.Automation = &provenance
	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      message,
			BusyBehavior: chatstate.BusyBehaviorQueue,
			MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
		})
		return err
	}))
	require.NotNil(t, send.QueuedMessage)
	return *send.QueuedMessage
}

// insertAutomationQueuedRow inserts a queued row from automation directly
// into the store, bypassing the state machine, at the queue tail.
func insertAutomationQueuedRow(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID, automation database.ChatAutomation, body string) database.ChatQueuedMessage {
	t.Helper()
	message := userTextMessage(body, f.User.ID, f.Model.ID)
	row, err := f.DB.InsertChatQueuedMessageWithCreator(ctx, database.InsertChatQueuedMessageWithCreatorParams{
		ChatID:          chatID,
		Content:         message.Content.RawMessage,
		ModelConfigID:   message.ModelConfigID,
		CreatedBy:       f.User.ID,
		AutomationID:    uuid.NullUUID{UUID: automation.ID, Valid: true},
		InputID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
		QueueGeneration: sql.NullInt64{Int64: automation.QueueGeneration, Valid: true},
	})
	require.NoError(t, err)
	return row
}

// insertStaleHeadRow inserts a queued row from an automation, moves it to
// the queue head, and deletes the automation so the row is stale.
func insertStaleHeadRow(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) database.ChatQueuedMessage {
	t.Helper()
	automation := f.newAutomation(t)
	row := insertAutomationQueuedRow(ctx, t, f, chatID, automation, "stale")
	_, err := f.DB.ReorderChatQueuedMessageToHead(ctx, database.ReorderChatQueuedMessageToHeadParams{
		ID:     row.ID,
		ChatID: chatID,
	})
	require.NoError(t, err)
	require.NoError(t, f.DB.DeleteChatAutomationByID(ctx, automation.ID))
	return row
}

// staleQueueShape selects what staleHeadSeed leaves behind the stale
// head.
type staleQueueShape int

const (
	// staleOnly removes the seeded ordinary rows, so the stale row is
	// the whole queue.
	staleOnly staleQueueShape = iota
	// staleThenOrdinary keeps the seeded ordinary rows behind the stale
	// head.
	staleThenOrdinary
)

// staleHeadSeed seeds from with a stale automation row at the queue
// head.
func staleHeadSeed(shape staleQueueShape) seederFn {
	return func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		seeded := seedState(t, f, from)
		stale := insertStaleHeadRow(ctx, t, f, seeded.chatID)
		if shape == staleOnly {
			for _, id := range seeded.queuedMessageIDs {
				_, err := f.DB.DeleteChatQueuedMessageReturningCount(ctx, database.DeleteChatQueuedMessageReturningCountParams{
					ID:     id,
					ChatID: seeded.chatID,
				})
				require.NoError(t, err)
			}
			seeded.queuedMessageIDs = nil
			seeded.queuedMessageBodies = nil
		}
		seeded.staleQueuedMessageID = stale.ID
		return seeded
	}
}

// sendMessageStaleHeadCase covers E1 -> SendMessage -> R0: the guard
// drops the stale head, so the new message goes straight into history.
func sendMessageStaleHeadCase() transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       chatstate.StateE1,
		want:       chatstate.StateR0,
		scenario:   scenarioStaleAutomation,
		seed:       staleHeadSeed(staleOnly),
		apply:      applySendMessageQueue,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, _ snapshotBaseline, result transitionCaseResult) {
			requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, seeded.staleQueuedMessageID)
			require.Nil(t, result.sendMessage.QueuedMessage, "the promoted new message is not reported as queued")
			require.NotEmpty(t, result.sendMessage.InsertedMessages)
			promoted := assertFetchedUserMessage(ctx, t, f,
				result.sendMessage.InsertedMessages[len(result.sendMessage.InsertedMessages)-1])
			assertChatMessageText(t, promoted, "sm-queue")
			require.True(t, promoted.QueuedMessageID.Valid)
			after := f.readChat(ctx, t, seeded.chatID)
			require.False(t, after.LastError.Valid, "the promotion clears last_error")
		},
	}
}

// promoteStaleCase covers explicit promotion of a stale queue head: the
// row is deleted and nothing else changes.
func promoteStaleCase(from, want chatstate.ExecutionState, shape staleQueueShape) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionPromoteQueuedMessage,
		from:       from,
		want:       want,
		scenario:   scenarioStaleAutomation,
		seed:       staleHeadSeed(shape),
		apply: func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
			t.Helper()
			var err error
			result.promoteQueuedMessage, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
				QueuedMessageID: seeded.staleQueuedMessageID,
			})
			return err
		},
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			require.True(t, result.promoteQueuedMessage.Rejected)
			require.Nil(t, result.promoteQueuedMessage.InsertedMessage)
			require.Equal(t, seeded.staleQueuedMessageID, result.promoteQueuedMessage.QueuedMessage.ID)
			requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, seeded.staleQueuedMessageID)
			require.Equal(t, base.queueIDs[1:], queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"a rejected promotion leaves the other queued rows in order")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"a rejected promotion inserts no history")
			after := f.readChat(ctx, t, seeded.chatID)
			require.Equal(t, base.chat.Status, after.Status, "a rejected promotion keeps the status")
			require.Equal(t, base.chat.LastError, after.LastError)
			require.Equal(t, base.chat.RequiresActionDeadlineAt.Valid, after.RequiresActionDeadlineAt.Valid)
		},
	}
}

// finishStaleQueueCase covers FinishTurn (R1) and FinishInterruption
// (I1) when the guard drops every queued row: both land in waiting.
func finishStaleQueueCase(tr chatstate.Transition, from chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: tr,
		from:       from,
		want:       chatstate.StateW,
		scenario:   scenarioStaleAutomation,
		seed:       staleHeadSeed(staleOnly),
		apply:      defaultApplier(tr),
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, seeded.staleQueuedMessageID)
			require.Empty(t, queuedIDsByPosition(ctx, t, f, seeded.chatID))
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"landing in waiting inserts no history")
			require.Nil(t, result.finishTurn.PromotedMessage)
			require.Nil(t, result.finishInterruption.PromotedMessage)
		},
	}
}

// deleteStaleArchivedCase covers XE1 -> DeleteQueuedMessage of a stale
// automation row: the row is deleted and the chat stays archived.
func deleteStaleArchivedCase(want chatstate.ExecutionState, shape staleQueueShape) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionDeleteQueuedMessage,
		from:       chatstate.StateXE1,
		want:       want,
		scenario:   scenarioStaleAutomation,
		seed:       staleHeadSeed(shape),
		apply: func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
			t.Helper()
			var err error
			result.deleteQueuedMessage, err = tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
				QueuedMessageID: seeded.staleQueuedMessageID,
			})
			return err
		},
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			require.Equal(t, seeded.staleQueuedMessageID, result.deleteQueuedMessage.DeletedQueuedMessage.ID)
			requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, seeded.staleQueuedMessageID)
			require.Equal(t, base.queueIDs[1:], queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"only the stale row is deleted")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"deleting a queued row inserts no history")
			after := f.readChat(ctx, t, seeded.chatID)
			require.True(t, after.Archived, "the chat stays archived")
			require.Equal(t, database.ChatStatusError, after.Status)
			require.Equal(t, base.chat.LastError, after.LastError)
		},
	}
}

// TestDeleteQueuedMessage_ArchivedKeepsPromotableRows covers the refusal
// half of DeleteQueuedMessage from XE1: a row that passes the queue
// promotion guard is not deleted from an archived chat.
func TestDeleteQueuedMessage_ArchivedKeepsPromotableRows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// target returns the queued row to delete.
		target func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat) int64
	}{
		{name: "Ordinary", target: func(_ context.Context, t *testing.T, _ *testFixture, seeded seededChat) int64 {
			require.NotEmpty(t, seeded.queuedMessageIDs)
			return seeded.queuedMessageIDs[0]
		}},
		{name: "LiveAutomation", target: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat) int64 {
			return insertAutomationQueuedRow(ctx, t, f, seeded.chatID, f.newAutomation(t), "live").ID
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			seeded := seedState(t, f, chatstate.StateXE1)
			target := tc.target(ctx, t, f, seeded)
			base := captureBaseline(ctx, t, f, seeded)

			m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
			err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				_, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{QueuedMessageID: target})
				return err
			})
			var te *chatstate.TransitionError
			require.ErrorAs(t, err, &te)
			require.Equal(t, chatstate.TransitionDeleteQueuedMessage, te.Transition)
			require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
			assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID))
			require.Equal(t, chatstate.StateXE1, f.classify(ctx, t, seeded.chatID))
		})
	}
}

// TestSendMessage_QueueCapIgnoresStaleRows covers the queue cap: rows that
// fail the queue promotion guard do not fill the queue. At the cap they
// are deleted and the new message is admitted, while a queue full of
// passing rows still refuses it.
func TestSendMessage_QueueCapIgnoresStaleRows(t *testing.T) {
	t.Parallel()
	const maxQueueSize = 3

	// fill queues maxQueueSize rows on the running chat and returns the
	// ids of the rows that are stale after it returns.
	type fillFn func(ctx context.Context, t *testing.T, f *testFixture, m *chatstate.ChatMachine) (stale []int64)
	staleFill := func(reason staleReason) fillFn {
		return func(ctx context.Context, t *testing.T, f *testFixture, m *chatstate.ChatMachine) []int64 {
			automations := []database.ChatAutomation{f.newAutomation(t), f.newAutomation(t)}
			var stale []int64
			for i := range maxQueueSize {
				stale = append(stale, queueAutomationMessage(t, f, m, "stale", provenanceFor(automations[i%2])).ID)
			}
			for _, a := range automations {
				reason.apply(ctx, t, f, a.ID)
			}
			return stale
		}
	}
	type capCase struct {
		name string
		from chatstate.ExecutionState
		fill fillFn
		full bool
	}
	var cases []capCase
	for _, from := range []chatstate.ExecutionState{chatstate.StateR1, chatstate.StateE1} {
		for _, reason := range staleReasons {
			cases = append(cases, capCase{name: string(from) + "/" + reason.name, from: from, fill: staleFill(reason)})
		}
	}
	cases = append(cases,
		capCase{name: "MixedKeepsPassingRows", from: chatstate.StateR1, fill: func(ctx context.Context, t *testing.T, f *testFixture, m *chatstate.ChatMachine) []int64 {
			stale := f.newAutomation(t)
			staleRow := queueAutomationMessage(t, f, m, "stale", provenanceFor(stale))
			sendQueuedMessage(t, f, m, "ordinary")
			queueAutomationMessage(t, f, m, "live", provenanceFor(f.newAutomation(t)))
			require.NoError(t, f.DB.DeleteChatAutomationByID(ctx, stale.ID))
			return []int64{staleRow.ID}
		}},
		capCase{name: "FullOrdinary", from: chatstate.StateR1, full: true, fill: func(_ context.Context, t *testing.T, f *testFixture, m *chatstate.ChatMachine) []int64 {
			for range maxQueueSize {
				sendQueuedMessage(t, f, m, "ordinary")
			}
			return nil
		}},
		capCase{name: "FullLiveAutomation", from: chatstate.StateR1, full: true, fill: func(_ context.Context, t *testing.T, f *testFixture, m *chatstate.ChatMachine) []int64 {
			live := f.newAutomation(t)
			for range maxQueueSize {
				queueAutomationMessage(t, f, m, "live", provenanceFor(live))
			}
			return nil
		}},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			chat := createTestChat(t, f)
			m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
			stale := tc.fill(ctx, t, f, m)
			if tc.from == chatstate.StateE1 {
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					_, err := tx.FinishError(chatstate.FinishErrorInput{})
					return err
				}))
			}
			require.Equal(t, tc.from, f.classify(ctx, t, chat.Chat.ID))
			before := queuedIDsByPosition(ctx, t, f, chat.Chat.ID)
			require.Len(t, before, maxQueueSize)

			var result chatstate.SendMessageResult
			err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				var err error
				result, err = tx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage("new", f.User.ID, f.Model.ID),
					BusyBehavior: chatstate.BusyBehaviorQueue,
					MaxQueueSize: maxQueueSize,
				})
				return err
			})
			if tc.full {
				var full *chatstate.MessageQueueFullError
				require.ErrorAs(t, err, &full)
				require.EqualValues(t, maxQueueSize, full.Max)
				require.Equal(t, before, queuedIDsByPosition(ctx, t, f, chat.Chat.ID), "a refused message deletes nothing")
				return
			}
			require.NoError(t, err)
			for _, id := range stale {
				requireQueuedMessageDeleted(ctx, t, f, chat.Chat.ID, id)
			}
			after := queuedIDsByPosition(ctx, t, f, chat.Chat.ID)
			if tc.from == chatstate.StateE1 {
				// Every older row was stale, so E1 promotes the new
				// message straight into history.
				require.Nil(t, result.QueuedMessage)
				require.Empty(t, after)
				require.Equal(t, chatstate.StateR0, f.classify(ctx, t, chat.Chat.ID))
				return
			}
			require.NotNil(t, result.QueuedMessage)
			var want []int64
			for _, id := range before {
				if !slices.Contains(stale, id) {
					want = append(want, id)
				}
			}
			want = append(want, result.QueuedMessage.ID)
			require.Equal(t, want, after, "only the stale rows are deleted and the new message is queued")
		})
	}
}

func requireAutomationProvenance(t *testing.T, msg database.ChatMessage, want chatstate.AutomationProvenance) {
	t.Helper()
	require.Equal(t, uuid.NullUUID{UUID: want.AutomationID, Valid: true}, msg.AutomationID)
	require.Equal(t, uuid.NullUUID{UUID: want.InputID, Valid: true}, msg.InputID)
}

// TestQueuePromotionGuard_HeadPromotion covers every head-promoting
// transition: stale heads are deleted, the next passing automation row is
// promoted with its provenance, and the rows behind it are untouched.
func TestQueuePromotionGuard_HeadPromotion(t *testing.T) {
	t.Parallel()

	type promotionPath struct {
		name string
		// enter moves the seeded R1 chat into the state the path
		// promotes from.
		enter func(ctx context.Context, tx *chatstate.Tx, store database.Store, chatID uuid.UUID) error
		// promote runs the transition and returns the promoted message.
		promote func(t *testing.T, f *testFixture, tx *chatstate.Tx) (*database.ChatMessage, error)
		// tail lists bodies the transition itself queues.
		tail []string
	}
	paths := []promotionPath{
		{
			name: "FinishTurn",
			promote: func(_ *testing.T, _ *testFixture, tx *chatstate.Tx) (*database.ChatMessage, error) {
				res, err := tx.FinishTurn(chatstate.FinishTurnInput{})
				return res.PromotedMessage, err
			},
		},
		{
			name: "FinishInterruption",
			enter: func(ctx context.Context, tx *chatstate.Tx, store database.Store, chatID uuid.UUID) error {
				// An unowned chat finishes the interruption inside
				// Interrupt, before the head is made stale.
				if err := ownChat(ctx, tx, store, chatID); err != nil {
					return err
				}
				_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
				return err
			},
			promote: func(_ *testing.T, _ *testFixture, tx *chatstate.Tx) (*database.ChatMessage, error) {
				res, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{})
				return res.PromotedMessage, err
			},
		},
		{
			name: "SendMessageE1",
			enter: func(_ context.Context, tx *chatstate.Tx, _ database.Store, _ uuid.UUID) error {
				_, err := tx.FinishError(chatstate.FinishErrorInput{})
				return err
			},
			promote: func(_ *testing.T, f *testFixture, tx *chatstate.Tx) (*database.ChatMessage, error) {
				res, err := tx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage("tail", f.User.ID, f.Model.ID),
					BusyBehavior: chatstate.BusyBehaviorQueue,
					MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
				})
				if err != nil || len(res.InsertedMessages) == 0 {
					return nil, err
				}
				return &res.InsertedMessages[len(res.InsertedMessages)-1], nil
			},
			tail: []string{"tail"},
		},
	}
	for _, path := range paths {
		for _, reason := range staleReasons {
			t.Run(path.name+"/"+reason.name, func(t *testing.T) {
				t.Parallel()
				f := newTestFixture(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				chat := createTestChat(t, f)
				m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)

				stale := f.newAutomation(t)
				live := f.newAutomation(t)
				staleRow := queueAutomationMessage(t, f, m, "stale", provenanceFor(stale))
				liveProvenance := provenanceFor(live)
				liveRow := queueAutomationMessage(t, f, m, "live", liveProvenance)
				sendQueuedMessage(t, f, m, "ordinary")
				// A stale row behind the promoted head must survive:
				// the guard deletes only rejected heads.
				queueAutomationMessage(t, f, m, "stale behind", provenanceFor(stale))
				if path.enter != nil {
					require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
						return path.enter(ctx, tx, store, chat.Chat.ID)
					}))
				}
				reason.apply(ctx, t, f, stale.ID)

				var promoted *database.ChatMessage
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					var err error
					promoted, err = path.promote(t, f, tx)
					return err
				}))
				require.NotNil(t, promoted)
				got := requireChatMessageByID(ctx, t, f, promoted.ID)
				assertChatMessageText(t, got, "live")
				requireQueuedMessageLink(t, got, liveRow.ID)
				requireAutomationProvenance(t, got, liveProvenance)
				requireQueuedMessageDeleted(ctx, t, f, chat.Chat.ID, staleRow.ID)
				assertQueueBodiesInOrder(ctx, t, f, chat.Chat.ID, append([]string{"ordinary", "stale behind"}, path.tail...))
				require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chat.Chat.ID))
			})
		}
	}
}

// TestQueuePromotionGuard_UnownedInterrupt covers the paths that finish
// the interruption of an unowned running or interrupting chat inline:
// they promote through the guard, and the chat stays unowned.
func TestQueuePromotionGuard_UnownedInterrupt(t *testing.T) {
	t.Parallel()

	interrupt := func(ctx context.Context, t *testing.T, m *chatstate.ChatMachine) {
		t.Helper()
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
			return err
		}))
	}

	// The chat starts unowned in R1, or unowned in I1 after a worker
	// interrupted and then abandoned it.
	starts := []struct {
		name string
		prep func(t *testing.T, f *testFixture, m *chatstate.ChatMachine, chatID uuid.UUID)
	}{
		{name: "Running", prep: func(*testing.T, *testFixture, *chatstate.ChatMachine, uuid.UUID) {}},
		{name: "Interrupting", prep: func(t *testing.T, f *testFixture, m *chatstate.ChatMachine, chatID uuid.UUID) {
			interruptAndAbandon(t, f, m, chatID)
			require.Equal(t, chatstate.StateI1, f.classify(testutil.Context(t, testutil.WaitShort), t, chatID))
		}},
	}

	for _, start := range starts {
		for _, reason := range staleReasons {
			t.Run(start.name+"/InterruptPromotesLiveRow/"+reason.name, func(t *testing.T) {
				t.Parallel()
				f := newTestFixture(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				chat := createTestChat(t, f)
				m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
				stale := f.newAutomation(t)
				live := f.newAutomation(t)
				staleRow := queueAutomationMessage(t, f, m, "stale", provenanceFor(stale))
				liveProvenance := provenanceFor(live)
				liveRow := queueAutomationMessage(t, f, m, "live", liveProvenance)
				sendQueuedMessage(t, f, m, "ordinary")
				start.prep(t, f, m, chat.Chat.ID)
				reason.apply(ctx, t, f, stale.ID)

				interrupt(ctx, t, m)

				history := historyMessageIDs(ctx, t, f, chat.Chat.ID)
				require.NotEmpty(t, history)
				got := requireChatMessageByID(ctx, t, f, history[len(history)-1])
				assertChatMessageText(t, got, "live")
				requireQueuedMessageLink(t, got, liveRow.ID)
				requireAutomationProvenance(t, got, liveProvenance)
				requireQueuedMessageDeleted(ctx, t, f, chat.Chat.ID, staleRow.ID)
				assertQueueBodiesInOrder(ctx, t, f, chat.Chat.ID, []string{"ordinary"})
				require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chat.Chat.ID))
				require.False(t, f.readChat(ctx, t, chat.Chat.ID).WorkerID.Valid)
			})

			t.Run(start.name+"/InterruptAllStaleLandsInWaiting/"+reason.name, func(t *testing.T) {
				t.Parallel()
				f := newTestFixture(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				chat := createTestChat(t, f)
				m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
				stale := f.newAutomation(t)
				first := queueAutomationMessage(t, f, m, "stale one", provenanceFor(stale))
				second := queueAutomationMessage(t, f, m, "stale two", provenanceFor(stale))
				before := historyMessageIDs(ctx, t, f, chat.Chat.ID)
				start.prep(t, f, m, chat.Chat.ID)
				reason.apply(ctx, t, f, stale.ID)

				interrupt(ctx, t, m)

				requireQueuedMessageDeleted(ctx, t, f, chat.Chat.ID, first.ID)
				requireQueuedMessageDeleted(ctx, t, f, chat.Chat.ID, second.ID)
				require.Empty(t, queuedIDsByPosition(ctx, t, f, chat.Chat.ID))
				require.Equal(t, before, historyMessageIDs(ctx, t, f, chat.Chat.ID),
					"landing in waiting inserts no history")
				require.Equal(t, chatstate.StateW, f.classify(ctx, t, chat.Chat.ID))
				require.False(t, f.readChat(ctx, t, chat.Chat.ID).WorkerID.Valid)
			})

			t.Run(start.name+"/SendMessageInterruptPromotesNewMessage/"+reason.name, func(t *testing.T) {
				t.Parallel()
				f := newTestFixture(t)
				ctx := testutil.Context(t, testutil.WaitShort)
				chat := createTestChat(t, f)
				m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
				stale := f.newAutomation(t)
				staleRow := queueAutomationMessage(t, f, m, "stale", provenanceFor(stale))
				start.prep(t, f, m, chat.Chat.ID)
				reason.apply(ctx, t, f, stale.ID)

				var result chatstate.SendMessageResult
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					var err error
					result, err = tx.SendMessage(chatstate.SendMessageInput{
						Message:      userTextMessage("new", f.User.ID, f.Model.ID),
						BusyBehavior: chatstate.BusyBehaviorInterrupt,
						MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
					})
					return err
				}))

				require.Nil(t, result.QueuedMessage, "the promoted new message is not reported as queued")
				require.NotEmpty(t, result.InsertedMessages)
				promoted := requireChatMessageByID(ctx, t, f,
					result.InsertedMessages[len(result.InsertedMessages)-1].ID)
				assertChatMessageText(t, promoted, "new")
				require.True(t, promoted.QueuedMessageID.Valid)
				requireQueuedMessageDeleted(ctx, t, f, chat.Chat.ID, staleRow.ID)
				require.Empty(t, queuedIDsByPosition(ctx, t, f, chat.Chat.ID))
				require.Equal(t, chatstate.StateR0, f.classify(ctx, t, chat.Chat.ID))
				require.False(t, f.readChat(ctx, t, chat.Chat.ID).WorkerID.Valid)
			})
		}
	}
}

// TestPromoteQueuedMessage_GuardsOnlyTarget covers explicit promotion of
// a passing automation row: it is promoted with its provenance even when
// a stale row sits ahead of it, because explicit promotion guards only
// the requested row.
func TestPromoteQueuedMessage_GuardsOnlyTarget(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chat := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
	live := f.newAutomation(t)
	liveProvenance := provenanceFor(live)
	liveRow := queueAutomationMessage(t, f, m, "live", liveProvenance)
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{})
		return err
	}))
	staleRow := insertStaleHeadRow(ctx, t, f, chat.Chat.ID)

	var result chatstate.PromoteQueuedMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		result, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{QueuedMessageID: liveRow.ID})
		return err
	}))
	require.False(t, result.Rejected)
	require.NotNil(t, result.InsertedMessage)
	got := requireChatMessageByID(ctx, t, f, result.InsertedMessage.ID)
	requireAutomationProvenance(t, got, liveProvenance)
	require.Equal(t, []int64{staleRow.ID}, queuedIDsByPosition(ctx, t, f, chat.Chat.ID))
}

// TestSendMessage_AdmitInTx covers the admission seam on SendMessage:
// the callback runs for the locked chat, its provenance lands on the
// history or queued row, and its error rolls everything back.
func TestSendMessage_AdmitInTx(t *testing.T) {
	t.Parallel()
	errAdmit := xerrors.New("admission refused")

	for _, tc := range []struct {
		name string
		// busy queues the message on a running chat instead of
		// sending it to a waiting chat.
		busy bool
		fail bool
	}{
		{name: "IdleAdmitted"},
		{name: "BusyAdmitted", busy: true},
		{name: "IdleRefused", fail: true},
		{name: "BusyRefused", busy: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			seeded := seedState(t, f, chatstate.StateW)
			if tc.busy {
				seeded = seedState(t, f, chatstate.StateR0)
			}
			base := captureBaseline(ctx, t, f, seeded)
			automation := f.newAutomation(t)
			provenance := provenanceFor(automation)

			var calledWith uuid.UUID
			m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
			var result chatstate.SendMessageResult
			err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				var err error
				result, err = tx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage("automation", f.User.ID, f.Model.ID),
					BusyBehavior: chatstate.BusyBehaviorQueue,
					MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
					AdmitInTx: func(ctx context.Context, store database.Store, chatID uuid.UUID) (chatstate.AutomationProvenance, error) {
						calledWith = chatID
						if _, err := chatstate.LockAutomations(ctx, store, []uuid.UUID{automation.ID}); err != nil {
							return chatstate.AutomationProvenance{}, err
						}
						if tc.fail {
							return chatstate.AutomationProvenance{}, errAdmit
						}
						return provenance, nil
					},
				})
				return err
			})
			require.Equal(t, seeded.chatID, calledWith)
			if tc.fail {
				require.ErrorIs(t, err, errAdmit)
				assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
				require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID))
				require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID))
				require.Equal(t, base.chat.Status, f.readChat(ctx, t, seeded.chatID).Status)
				return
			}
			require.NoError(t, err)
			if tc.busy {
				require.NotNil(t, result.QueuedMessage)
				queued := requireQueuedMessageByID(ctx, t, f, seeded.chatID, result.QueuedMessage.ID)
				require.Equal(t, uuid.NullUUID{UUID: provenance.AutomationID, Valid: true}, queued.AutomationID)
				require.Equal(t, uuid.NullUUID{UUID: provenance.InputID, Valid: true}, queued.InputID)
				require.Equal(t, sql.NullInt64{Int64: provenance.QueueGeneration, Valid: true}, queued.QueueGeneration)
				return
			}
			require.Len(t, result.InsertedMessages, 1)
			requireAutomationProvenance(t, requireChatMessageByID(ctx, t, f, result.InsertedMessages[0].ID), provenance)
		})
	}
}

// TestSendMessage_AdmitInTxRejectsIncompleteProvenance covers the
// defensive check on what an admission callback returns.
func TestSendMessage_AdmitInTxRejectsIncompleteProvenance(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, chatstate.StateW)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	for _, provenance := range []chatstate.AutomationProvenance{
		{InputID: uuid.New(), QueueGeneration: 1},
		{AutomationID: uuid.New(), QueueGeneration: 1},
		{AutomationID: uuid.New(), InputID: uuid.New()},
	} {
		err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.SendMessage(chatstate.SendMessageInput{
				Message:      userTextMessage("automation", f.User.ID, f.Model.ID),
				BusyBehavior: chatstate.BusyBehaviorQueue,
				MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
				AdmitInTx: func(context.Context, database.Store, uuid.UUID) (chatstate.AutomationProvenance, error) {
					return provenance, nil
				},
			})
			return err
		})
		require.ErrorContains(t, err, "automation provenance")
	}
	require.Equal(t, chatstate.StateW, f.classify(ctx, t, seeded.chatID))
}

// TestCreateChat_AdmitInTx covers the admission seam on chat creation.
func TestCreateChat_AdmitInTx(t *testing.T) {
	t.Parallel()
	errAdmit := xerrors.New("admission refused")

	for _, fail := range []bool{false, true} {
		name := "Admitted"
		if fail {
			name = "Refused"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			automation := f.newAutomation(t)
			provenance := provenanceFor(automation)
			chatID := uuid.New()
			result, err := chatstate.CreateChatWithID(ctx, f.DB, f.Pub, chatID, chatstate.CreateChatInput{
				OrganizationID:    f.Org.ID,
				OwnerID:           f.User.ID,
				LastModelConfigID: f.Model.ID,
				Title:             "automation",
				ClientType:        database.ChatClientTypeApi,
				InitialMessages: []chatstate.Message{
					systemTextMessage("system", f.Model.ID),
					userTextMessage("automation", f.User.ID, f.Model.ID),
				},
				AdmitInTx: func(ctx context.Context, store database.Store, gotID uuid.UUID) (chatstate.AutomationProvenance, error) {
					require.Equal(t, chatID, gotID)
					// The chat row exists in the transaction.
					_, err := store.GetChatByID(ctx, gotID)
					require.NoError(t, err)
					if fail {
						return chatstate.AutomationProvenance{}, errAdmit
					}
					return provenance, nil
				},
			})
			if fail {
				require.ErrorIs(t, err, errAdmit)
				_, err := f.DB.GetChatByID(ctx, chatID)
				require.ErrorIs(t, err, sql.ErrNoRows)
				require.Empty(t, f.Pub.channels)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.InitialMessages, 2)
			require.False(t, result.InitialMessages[0].AutomationID.Valid, "system messages carry no provenance")
			requireAutomationProvenance(t, result.InitialMessages[1], provenance)
		})
	}
}

func systemTextMessage(text string, modelConfigID uuid.UUID) chatstate.Message {
	m := userTextMessage(text, uuid.Nil, modelConfigID)
	m.Role = database.ChatMessageRoleSystem
	m.CreatedBy = uuid.NullUUID{}
	return m
}

// TestCreateChat_AdmitInTxRequiresOneUserMessage covers the shape check:
// the provenance must have exactly one user message to land on.
func TestCreateChat_AdmitInTxRequiresOneUserMessage(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	admit := func(context.Context, database.Store, uuid.UUID) (chatstate.AutomationProvenance, error) {
		t.Fatal("admission must not run for an invalid initial history")
		return chatstate.AutomationProvenance{}, nil
	}
	for _, messages := range [][]chatstate.Message{
		{systemTextMessage("system", f.Model.ID)},
		{userTextMessage("a", f.User.ID, f.Model.ID), userTextMessage("b", f.User.ID, f.Model.ID)},
	} {
		_, err := chatstate.CreateChat(ctx, f.DB, f.Pub, chatstate.CreateChatInput{
			OrganizationID:    f.Org.ID,
			OwnerID:           f.User.ID,
			LastModelConfigID: f.Model.ID,
			Title:             "automation",
			ClientType:        database.ChatClientTypeApi,
			InitialMessages:   messages,
			AdmitInTx:         admit,
		})
		require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
	}
}

// TestLockAutomations_AsChatd covers that the lock primitive sees
// automations through dbauthz regardless of the caller, so an
// authorization filter can never make a live automation look missing
// and get its queued rows dropped.
func TestLockAutomations_AsChatd(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	first := f.newAutomation(t)
	second := f.newAutomation(t)
	acs := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var agpl dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	acs.Store(&agpl)
	authzDB := dbauthz.New(f.DB, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), acs)

	var locked map[uuid.UUID]database.ChatAutomation
	// The context carries no actor: the lock must bring its own. The
	// chat row lock is irrelevant here, so a bare transaction is enough.
	require.NoError(t, authzDB.InTx(func(store database.Store) error {
		var err error
		locked, err = chatstate.LockAutomations(ctx, store, []uuid.UUID{second.ID, first.ID, second.ID, uuid.New()})
		return err
	}, nil))
	require.Len(t, locked, 2)
	require.Equal(t, first.ID, locked[first.ID].ID)
	require.Equal(t, second.ID, locked[second.ID].ID)
}

// deadlockStore injects a PostgreSQL deadlock abort into the first
// failures calls that lock automations, and counts the real deadlock
// aborts those calls hit.
type deadlockStore struct {
	database.Store
	failures  *atomic.Int32
	deadlocks *atomic.Int32
}

func newDeadlockStore(store database.Store, failures int32) *deadlockStore {
	s := &deadlockStore{Store: store, failures: &atomic.Int32{}, deadlocks: &atomic.Int32{}}
	s.failures.Store(failures)
	return s
}

func (s *deadlockStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(&deadlockStore{Store: tx, failures: s.failures, deadlocks: s.deadlocks})
	}, opts)
}

func (s *deadlockStore) GetChatAutomationsByIDsForUpdate(ctx context.Context, ids []uuid.UUID) ([]database.ChatAutomation, error) {
	if s.failures.Add(-1) >= 0 {
		return nil, &pq.Error{Code: "40P01", Message: "deadlock detected"}
	}
	rows, err := s.Store.GetChatAutomationsByIDsForUpdate(ctx, ids)
	if database.IsDeadlockError(err) {
		s.deadlocks.Add(1)
	}
	return rows, err
}

// TestChatMachine_Update_RetriesAutomationDeadlocks covers the bounded
// retry of an Update whose automation locks hit a deadlock abort, and
// that Updates without automation locks are never retried.
func TestChatMachine_Update_RetriesAutomationDeadlocks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		failures     int32
		wantAttempts int
		wantErr      bool
	}{
		{name: "RecoversAfterOneDeadlock", failures: 1, wantAttempts: 2},
		{name: "GivesUpAfterThreeAttempts", failures: 10, wantAttempts: 3, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			chat := createTestChat(t, f)
			m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
			provenance := provenanceFor(f.newAutomation(t))
			queued := queueAutomationMessage(t, f, m, "queued", provenance)

			retrying := chatstate.NewChatMachine(newDeadlockStore(f.DB, tc.failures), f.Pub, chat.Chat.ID)
			attempts := 0
			var promoted *database.ChatMessage
			err := retrying.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				attempts++
				res, err := tx.FinishTurn(chatstate.FinishTurnInput{})
				promoted = res.PromotedMessage
				return err
			})
			require.Equal(t, tc.wantAttempts, attempts)
			if tc.wantErr {
				require.True(t, database.IsDeadlockError(err), "got %v", err)
				require.Equal(t, []int64{queued.ID}, queuedIDsByPosition(ctx, t, f, chat.Chat.ID))
				return
			}
			require.NoError(t, err)
			require.NotNil(t, promoted)
			requireQueuedMessageLink(t, *promoted, queued.ID)
		})
	}

	t.Run("NoRetryWithoutAutomationLocks", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		chat := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
		attempts := 0
		err := m.Update(ctx, func(*chatstate.Tx, database.Store) error {
			attempts++
			return &pq.Error{Code: "40P01", Message: "deadlock detected"}
		})
		require.True(t, database.IsDeadlockError(err))
		require.Equal(t, 1, attempts)
	})
}

var errNotAdmitted = xerrors.New("automation not admitted")

// admitLockingQueue admits a message from own the way production
// admission does: it locks own together with every automation already
// queued on the chat in one LockAutomations call, and refuses with
// errNotAdmitted when own is missing or disabled.
func admitLockingQueue(own database.ChatAutomation) chatstate.AdmitFunc {
	return func(ctx context.Context, store database.Store, chatID uuid.UUID) (chatstate.AutomationProvenance, error) {
		queue, err := store.GetChatQueuedMessagesByPosition(ctx, chatID)
		if err != nil {
			return chatstate.AutomationProvenance{}, err
		}
		ids := []uuid.UUID{own.ID}
		for _, row := range queue {
			if row.AutomationID.Valid {
				ids = append(ids, row.AutomationID.UUID)
			}
		}
		locked, err := chatstate.LockAutomations(ctx, store, ids)
		if err != nil {
			return chatstate.AutomationProvenance{}, err
		}
		current, ok := locked[own.ID]
		if !ok || !current.Enabled {
			return chatstate.AutomationProvenance{}, errNotAdmitted
		}
		return chatstate.AutomationProvenance{
			AutomationID:    own.ID,
			InputID:         uuid.New(),
			QueueGeneration: current.QueueGeneration,
		}, nil
	}
}

// TestQueuePromotionGuard_Concurrency runs admissions, promotions, and
// disables against chats whose queues mix rows from several automations
// in no particular id order. Every transaction takes the chat lock first
// and automation locks in ascending id order, so no deadlock abort may
// happen at all. The test counts aborts below Update because its retry
// would otherwise hide a lock order regression.
func TestQueuePromotionGuard_Concurrency(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		chatCount       = 4
		automationCount = 4
		rounds          = 8
	)
	automations := make([]database.ChatAutomation, automationCount)
	for i := range automations {
		automations[i] = f.newAutomation(t)
	}
	chatIDs := make([]uuid.UUID, chatCount)
	for i := range chatIDs {
		chat := createTestChat(t, f)
		chatIDs[i] = chat.Chat.ID
		m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
		// Queue rows from every automation, rotated per chat so queue
		// order and automation id order disagree.
		for j := range automations {
			a := automations[(i+j)%automationCount]
			queueAutomationMessage(t, f, m, "seed", provenanceFor(a))
		}
	}

	// tolerated reports errors that are legitimate outcomes of the race,
	// such as a promotion after the queue drained.
	tolerated := func(err error) bool {
		return err == nil ||
			xerrors.Is(err, errNotAdmitted) ||
			xerrors.Is(err, chatstate.ErrTransitionNotAllowed) ||
			xerrors.Is(err, chatstate.ErrQueuedMessageNotFound) ||
			xerrors.Is(err, chatstate.ErrMessageQueueFull)
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	record := func(err error) {
		if tolerated(err) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		errs = append(errs, err)
	}
	counting := newDeadlockStore(f.DB, 0)
	for i, chatID := range chatIDs {
		m := chatstate.NewChatMachine(counting, f.Pub, chatID)
		wg.Add(3)
		go func() {
			defer wg.Done()
			for r := range rounds {
				own := automations[(i+r)%automationCount]
				record(m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					_, err := tx.SendMessage(chatstate.SendMessageInput{
						Message:      userTextMessage("admitted", f.User.ID, f.Model.ID),
						BusyBehavior: chatstate.BusyBehaviorQueue,
						MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
						AdmitInTx:    admitLockingQueue(own),
					})
					return err
				}))
			}
		}()
		go func() {
			defer wg.Done()
			for range rounds {
				record(m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
					return err
				}))
			}
		}()
		go func() {
			defer wg.Done()
			for range rounds {
				queue, err := f.DB.GetChatQueuedMessagesByPosition(ctx, chatID)
				if err != nil || len(queue) == 0 {
					record(err)
					continue
				}
				record(m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
					_, err := tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{QueuedMessageID: queue[len(queue)-1].ID})
					return err
				}))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for r := range rounds {
			a := automations[r%automationCount]
			_, err := f.SQLDB.ExecContext(ctx,
				`UPDATE chat_automations SET enabled = NOT enabled, queue_generation = queue_generation + 1 WHERE id = $1`, a.ID)
			record(err)
		}
	}()
	wg.Wait()
	require.Empty(t, errs)
	require.Zero(t, counting.deadlocks.Load(), "ordered automation locking must never deadlock")
	for _, chatID := range chatIDs {
		require.NotEqual(t, chatstate.StateInvalid, f.classify(ctx, t, chatID))
	}
}

// TestSendMessage_QueueCapPurgeRace covers concurrent sends to a chat
// whose queue is full of stale automation rows. People and live
// automations race for the freed capacity: exactly the cap is admitted,
// the rest are refused as queue full, and no stale row survives. The
// purge locks the queue's automations under the chat lock, so the test
// also counts deadlock aborts below Update, whose retry would hide them.
func TestSendMessage_QueueCapPurgeRace(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	const (
		maxQueueSize = 4
		senders      = maxQueueSize + 4
	)
	chat := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, chat.Chat.ID)
	stale := make([]int64, 0, maxQueueSize)
	for _, reason := range staleReasons {
		automation := f.newAutomation(t)
		stale = append(stale, queueAutomationMessage(t, f, m, "stale", provenanceFor(automation)).ID)
		reason.apply(ctx, t, f, automation.ID)
	}
	require.Len(t, stale, maxQueueSize, "one stale row per reason fills the queue")
	live := []database.ChatAutomation{f.newAutomation(t), f.newAutomation(t)}

	counting := newDeadlockStore(f.DB, 0)
	racing := chatstate.NewChatMachine(counting, f.Pub, chat.Chat.ID)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		admitted []int64
		full     int
		errs     []error
	)
	start := make(chan struct{})
	for i := range senders {
		var admit chatstate.AdmitFunc
		if i%2 == 1 {
			admit = admitLockingQueue(live[(i/2)%len(live)])
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var result chatstate.SendMessageResult
			err := racing.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				var err error
				result, err = tx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage("racer", f.User.ID, f.Model.ID),
					BusyBehavior: chatstate.BusyBehaviorQueue,
					MaxQueueSize: maxQueueSize,
					AdmitInTx:    admit,
				})
				return err
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil && result.QueuedMessage != nil:
				admitted = append(admitted, result.QueuedMessage.ID)
			case xerrors.Is(err, chatstate.ErrMessageQueueFull):
				full++
			case err == nil:
				errs = append(errs, xerrors.New("a send to a running chat was not queued"))
			default:
				errs = append(errs, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	require.Empty(t, errs)
	require.Zero(t, counting.deadlocks.Load(), "the purge must lock automations in order")
	require.Len(t, admitted, maxQueueSize, "the freed queue takes exactly the cap")
	require.Equal(t, senders-maxQueueSize, full)
	slices.Sort(admitted)
	require.Equal(t, admitted, queuedIDsByPosition(ctx, t, f, chat.Chat.ID),
		"the queue holds the admitted rows in order and no stale row")
	for _, id := range stale {
		requireQueuedMessageDeleted(ctx, t, f, chat.Chat.ID, id)
	}
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chat.Chat.ID))
}
