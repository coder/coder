package chatstate_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// CreateChat tests.
//
// CreateChat is the only transition that originates from StateN and it
// is not exercised through ChatMachine.Update, so it lives outside
// TestTransitionMatrix_AllCombinations.

// TestTransitionCreate_NToR0 verifies that CreateChat lands a fresh
// chat in R0 with snapshot_version 1, the initial user message
// recorded at revision 1, queue_version still 0, and the post-commit
// publish requesting an ownership hint plus a chat:update.
func TestTransitionCreate_NToR0(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	res := createTestChat(t, f)

	require.Equal(t, database.ChatStatusRunning, res.Chat.Status)
	require.False(t, res.Chat.Archived)
	require.Equal(t, int64(1), res.Chat.SnapshotVersion, "snapshot_version starts at 1")
	require.Equal(t, int64(1), res.Chat.HistoryVersion, "history_version set by trigger after initial insert")
	require.Equal(t, int64(0), res.Chat.QueueVersion, "queue_version stays 0 when no queue rows")
	require.Equal(t, int64(0), res.Chat.GenerationAttempt)
	require.NotEmpty(t, res.InitialMessages)
	require.Equal(t, int64(1), res.InitialMessages[0].Revision)
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, res.Chat.ID))
	require.True(t, f.Pub.hasOwnership(), "newly created chat is runnable and unowned")
	f.Pub.expectChatUpdate(t, res.Chat.ID, 1)
}

// TestCreateChat_RejectsEmptyInitialMessages verifies that CreateChat
// rejects an empty InitialMessages slice with ErrTransitionNotAllowed
// and does not publish anything.
func TestCreateChat_RejectsEmptyInitialMessages(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	_, err := chatstate.CreateChat(ctx, f.DB, f.Pub, chatstate.CreateChatInput{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		ClientType:        database.ChatClientTypeApi,
		Title:             "t",
		InitialMessages:   nil,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
	require.Empty(t, f.Pub.channels, "rejected create must not publish")
}

func TestCreateChat_AllowsNoUserMessages(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	assistant := userTextMessage("oops", f.User.ID, f.Model.ID)
	assistant.Role = database.ChatMessageRoleAssistant
	res, err := chatstate.CreateChat(ctx, f.DB, f.Pub, chatstate.CreateChatInput{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "t",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages:   []chatstate.Message{assistant},
	})
	require.NoError(t, err)
	require.Len(t, res.InitialMessages, 1)
}

func TestCreateChat_AllowsNonFinalUserMessage(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	res, err := chatstate.CreateChat(ctx, f.DB, f.Pub, chatstate.CreateChatInput{
		OrganizationID:    f.Org.ID,
		OwnerID:           f.User.ID,
		LastModelConfigID: f.Model.ID,
		Title:             "t",
		ClientType:        database.ChatClientTypeApi,
		InitialMessages: []chatstate.Message{
			userTextMessage("context user", f.User.ID, f.Model.ID),
			userTextMessage("final user", f.User.ID, f.Model.ID),
		},
	})
	require.NoError(t, err)
	require.Len(t, res.InitialMessages, 2)
}

// Input-specific rejection cases.
//
// These tests cover the same matrix rows as TestTransitionMatrix_AllCombinations
// but exercise legal source states with invalid transition inputs. They are
// intentionally outside the matrix entry point so the matrix focus stays on
// positive cases and generated disallowed cases.

type setArchivedWrongDirectionCase struct {
	from        chatstate.ExecutionState
	wantArchive bool
	label       string
}

func setArchivedWrongDirectionCases() []setArchivedWrongDirectionCase {
	return []setArchivedWrongDirectionCase{
		// Non-archived states with archived=false: no-op.
		{from: chatstate.StateW, wantArchive: false, label: "W_to_W"},
		{from: chatstate.StateE0, wantArchive: false, label: "E0_to_E0"},
		{from: chatstate.StateE1, wantArchive: false, label: "E1_to_E1"},
		// Archived states with archived=true: no-op.
		{from: chatstate.StateXW, wantArchive: true, label: "XW_to_XW"},
		{from: chatstate.StateXE0, wantArchive: true, label: "XE0_to_XE0"},
		{from: chatstate.StateXE1, wantArchive: true, label: "XE1_to_XE1"},
	}
}

var invalidBusyBehaviors = []chatstate.BusyBehavior{
	chatstate.BusyBehavior(""),
	chatstate.BusyBehavior("not-a-real-mode"),
}

func runSetArchivedWrongDirectionCase(t *testing.T, tc setArchivedWrongDirectionCase) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, tc.from)
	require.Equal(t, tc.from, f.classify(ctx, t, seeded.chatID))

	base := captureBaseline(ctx, t, f, seeded)

	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, serr := tx.SetArchived(chatstate.SetArchivedInput{Archived: tc.wantArchive})
		return serr
	})
	require.Error(t, err, "SetArchived must reject when Archived matches the current value")
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed,
		"SetArchived must wrap ErrTransitionNotAllowed")
	var te *chatstate.TransitionError
	require.ErrorAs(t, err, &te,
		"SetArchived must return a typed TransitionError")
	require.Equal(t, chatstate.TransitionSetArchived, te.Transition)
	require.Equal(t, tc.from, te.From, "TransitionError records the loaded from-state")

	require.Equal(t, tc.from, f.classify(ctx, t, seeded.chatID),
		"rejected SetArchived must leave the chat in the same state")
	assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
}

func runInvalidBusyBehaviorCase(t *testing.T, from chatstate.ExecutionState, bb chatstate.BusyBehavior) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, from)
	require.Equal(t, from, f.classify(ctx, t, seeded.chatID),
		"seed must land in %s", from)
	base := captureBaseline(ctx, t, f, seeded)

	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, serr := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("invalid-bb", f.User.ID, f.Model.ID),
			BusyBehavior: bb,
		})
		return serr
	})
	require.Error(t, err, "SendMessage must reject invalid BusyBehavior")
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed,
		"SendMessage rejection must wrap ErrTransitionNotAllowed")
	var te *chatstate.TransitionError
	require.ErrorAs(t, err, &te,
		"SendMessage must return a typed TransitionError")
	require.Equal(t, chatstate.TransitionSendMessage, te.Transition)
	require.Equal(t, from, te.From,
		"TransitionError records the source state")

	require.Equal(t, from, f.classify(ctx, t, seeded.chatID),
		"rejected SendMessage must leave the chat in the same state")
	assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
}

type completeRequiresActionRejectCase struct {
	name    string
	results func(seeded seededChat) []chatstate.ToolResultInput
}

type recordRetryStateRejectCase struct {
	name       string
	retryState pqtype.NullRawMessage
}

func completeRequiresActionRejectCases() []completeRequiresActionRejectCase {
	valid := func(id string) chatstate.ToolResultInput {
		return chatstate.ToolResultInput{
			ToolCallID: id,
			Output:     json.RawMessage(`{"ok":true}`),
		}
	}
	return []completeRequiresActionRejectCase{
		{
			name:    "missing_required_tool_result",
			results: func(seeded seededChat) []chatstate.ToolResultInput { return nil },
		},
		{
			name: "extra_tool_result",
			results: func(seeded seededChat) []chatstate.ToolResultInput {
				return []chatstate.ToolResultInput{valid(seeded.pendingToolCallID), valid("call_extra")}
			},
		},
		{
			name: "duplicate_tool_call_id",
			results: func(seeded seededChat) []chatstate.ToolResultInput {
				return []chatstate.ToolResultInput{valid(seeded.pendingToolCallID), valid(seeded.pendingToolCallID)}
			},
		},
		{
			name: "mismatched_tool_call_id",
			results: func(seeded seededChat) []chatstate.ToolResultInput {
				return []chatstate.ToolResultInput{valid("call_mismatch")}
			},
		},
		{
			name: "invalid_json_output",
			results: func(seeded seededChat) []chatstate.ToolResultInput {
				return []chatstate.ToolResultInput{{ToolCallID: seeded.pendingToolCallID, Output: json.RawMessage(`{`)}}
			},
		},
	}
}

func recordRetryStateRejectCases() []recordRetryStateRejectCase {
	return []recordRetryStateRejectCase{
		{
			name: "sql_null_payload",
		},
		{
			name:       "empty_payload",
			retryState: pqtype.NullRawMessage{RawMessage: json.RawMessage(``), Valid: true},
		},
		{
			name:       "invalid_json_payload",
			retryState: pqtype.NullRawMessage{RawMessage: json.RawMessage(`{`), Valid: true},
		},
	}
}

func runCompleteRequiresActionRejectCase(t *testing.T, tc completeRequiresActionRejectCase) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedAOrA1(t, f, 0, "reject_complete_requires_action")
	require.Equal(t, chatstate.StateA0, f.classify(ctx, t, seeded.chatID))
	base := captureBaseline(ctx, t, f, seeded)

	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, cerr := tx.CompleteRequiresAction(chatstate.CompleteRequiresActionInput{
			CreatedBy:     f.User.ID,
			ModelConfigID: f.Model.ID,
			Results:       tc.results(seeded),
		})
		return cerr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
	var te *chatstate.TransitionError
	require.ErrorAs(t, err, &te)
	require.Equal(t, chatstate.TransitionCompleteRequiresAction, te.Transition)
	require.Equal(t, chatstate.StateA0, te.From)
	assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
}

func runRecordRetryStateRejectCase(t *testing.T, tc recordRetryStateRejectCase) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, chatstate.StateR0)
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, seeded.chatID))
	base := captureBaseline(ctx, t, f, seeded)

	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, rerr := tx.RecordRetryState(chatstate.RecordRetryStateInput{
			RetryState: tc.retryState,
		})
		return rerr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
	var te *chatstate.TransitionError
	require.ErrorAs(t, err, &te)
	require.Equal(t, chatstate.TransitionRecordRetryState, te.Transition)
	require.Equal(t, chatstate.StateR0, te.From)
	assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
}

// TestTransitionInputValidation groups every input-specific rejection
// test. The matrix coverage entry point in
// TestTransitionMatrix_AllCombinations intentionally focuses on
// positive cases and generated disallowed cases; rejection cases that
// exercise legal matrix rows with invalid inputs live here so the
// matrix entry point stays focused.
func TestTransitionInputValidation(t *testing.T) {
	t.Parallel()

	t.Run("SetArchived_wrong_direction", func(t *testing.T) {
		t.Parallel()
		for _, tc := range setArchivedWrongDirectionCases() {
			t.Run(tc.label, func(t *testing.T) {
				t.Parallel()
				runSetArchivedWrongDirectionCase(t, tc)
			})
		}
	})

	t.Run("SendMessage_invalid_busy_behavior", func(t *testing.T) {
		t.Parallel()
		for _, from := range chatstate.AllowedInputStates(chatstate.TransitionSendMessage) {
			for _, bb := range invalidBusyBehaviors {
				label := from.String() + "/" + string(bb)
				if bb == "" {
					label = from.String() + "/empty"
				}
				t.Run(label, func(t *testing.T) {
					t.Parallel()
					runInvalidBusyBehaviorCase(t, from, bb)
				})
			}
		}
	})

	t.Run("CompleteRequiresAction_invalid_results", func(t *testing.T) {
		t.Parallel()
		for _, tc := range completeRequiresActionRejectCases() {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				runCompleteRequiresActionRejectCase(t, tc)
			})
		}
	})

	t.Run("RecordRetryState_invalid_payload", func(t *testing.T) {
		t.Parallel()
		for _, tc := range recordRetryStateRejectCases() {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				runRecordRetryStateRejectCase(t, tc)
			})
		}
	})
}

// TestSendMessageQueueCapRejectsQueueAppend seeds a chat with the
// maximum queued messages and asserts that the next SendMessage in
// a queue-appending state returns chatstate.ErrMessageQueueFull and
// rolls back without persisting another queued row.
func TestSendMessageQueueCapRejectsQueueAppend(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	// createTestChat lands the chat in R0; SendMessage in R0 with
	// BusyBehaviorQueue queues. Fill the queue to MaxQueueSize.
	for i := 0; i < chatstate.MaxQueueSize; i++ {
		sendQueuedMessage(t, f, m, "filler")
	}
	count, err := f.DB.CountChatQueuedMessages(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.EqualValues(t, chatstate.MaxQueueSize, count)
	chatBefore := f.readChat(ctx, t, created.Chat.ID)

	// The next queue append must fail with ErrMessageQueueFull and a
	// typed wrapper that exposes the cap.
	err = m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, serr := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("overflow", f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return serr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatstate.ErrMessageQueueFull,
		"queue-append over the cap returns ErrMessageQueueFull")
	var typed *chatstate.MessageQueueFullError
	require.ErrorAs(t, err, &typed, "ErrMessageQueueFull is carried as a typed error")
	require.EqualValues(t, chatstate.MaxQueueSize, typed.Max)

	// The transaction rolled back: queue size, snapshot version,
	// and queue version are unchanged.
	countAfter, err := f.DB.CountChatQueuedMessages(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.EqualValues(t, chatstate.MaxQueueSize, countAfter,
		"queue size must not change when the cap rejects the append")
	chatAfter := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, chatBefore.SnapshotVersion, chatAfter.SnapshotVersion,
		"failed queue append must not bump snapshot_version")
	require.Equal(t, chatBefore.QueueVersion, chatAfter.QueueVersion,
		"failed queue append must not bump queue_version")
}

// TestEditMessageNonUserReturnsSentinel asserts that editing a
// non-user message returns chatstate.ErrEditedMessageNotUser via
// the TransitionError cause chain, and still matches the generic
// transition sentinel.
func TestEditMessageNonUserReturnsSentinel(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	// Insert an assistant message via CommitStep so we have a
	// non-user message to target.
	var assistantID int64
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		assistant := userTextMessage("assistant", f.User.ID, f.Model.ID)
		assistant.Role = database.ChatMessageRoleAssistant
		step, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistant},
		})
		if err != nil {
			return err
		}
		require.Len(t, step.InsertedMessages, 1)
		assistantID = step.InsertedMessages[0].ID
		return nil
	}))

	rawContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("new content"),
	})
	require.NoError(t, err)

	editErr := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, eerr := tx.EditMessage(chatstate.EditMessageInput{
			MessageID: assistantID,
			CreatedBy: f.User.ID,
			Content:   rawContent,
		})
		return eerr
	})
	require.Error(t, editErr)
	require.ErrorIs(t, editErr, chatstate.ErrEditedMessageNotUser,
		"non-user edit returns ErrEditedMessageNotUser via TransitionError cause")
	require.ErrorIs(t, editErr, chatstate.ErrTransitionNotAllowed,
		"ErrEditedMessageNotUser still matches the generic transition sentinel")
}

// TestTransitionAbandon_RejectsUnowned verifies that calling Abandon
// on a chat the runner does not own returns ErrTransitionNotAllowed
// wrapped in a TransitionError that records the loaded from-state,
// without mutating chat state or publishing anything.
func TestTransitionAbandon_RejectsUnowned(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	seeded := seededChat{chatID: created.Chat.ID, exists: true}
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	base := captureBaseline(ctx, t, f, seeded)

	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, aerr := tx.Abandon(chatstate.AbandonInput{})
		return aerr
	})
	require.Error(t, err)
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
	var te *chatstate.TransitionError
	require.ErrorAs(t, err, &te)
	require.Equal(t, chatstate.TransitionAbandon, te.Transition)
	// createTestChat lands the chat in R0; Abandon's precondition
	// rejects an unowned chat there.
	require.Equal(t, chatstate.StateR0, te.From)
	assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
}

// TestTransitionAbandon_ClearsOwnership verifies the Acquire/Abandon
// round-trip: after Acquire the chat carries a worker+runner and a
// fresh heartbeat row exists, and after Abandon both ownership fields
// are cleared. The heartbeat row is not deleted by Abandon; heartbeat
// cleanup is a separate concern.
func TestTransitionAbandon_ClearsOwnership(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	worker := uuid.New()
	runner := uuid.New()

	// Acquire writes ownership and a fresh heartbeat row.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: worker, RunnerID: runner})
		return err
	}))
	owned := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, worker, owned.WorkerID.UUID)
	require.Equal(t, runner, owned.RunnerID.UUID)
	hb, err := f.DB.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
		ChatID:   created.Chat.ID,
		RunnerID: runner,
	})
	require.NoError(t, err, "Acquire writes a fresh heartbeat row")
	require.Equal(t, runner, hb.RunnerID)

	// Abandon clears ownership but leaves the heartbeat row intact.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Abandon(chatstate.AbandonInput{})
		return err
	}))
	hb, err = f.DB.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
		ChatID:   created.Chat.ID,
		RunnerID: runner,
	})
	require.NoError(t, err, "Abandon does not delete the heartbeat row")
	abandoned := f.readChat(ctx, t, created.Chat.ID)
	require.False(t, abandoned.WorkerID.Valid, "Abandon clears worker_id")
	require.False(t, abandoned.RunnerID.Valid, "Abandon clears runner_id")
}

// TestTransitionAcquire_OverwritesFreshOwnership verifies that Acquire
// is an unconditional ownership handoff: a second worker calling
// Acquire on a chat that was *just* acquired by another worker
// successfully replaces ownership without inspecting heartbeat
// freshness. It also asserts that Acquire itself does not request an
// ownership hint, so the post-commit publish stays quiet on
// `chat:ownership` when the resulting heartbeat is fresh.
func TestTransitionAcquire_OverwritesFreshOwnership(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	firstWorker := uuid.New()
	firstRunner := uuid.New()
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: firstWorker, RunnerID: firstRunner})
		return err
	}))

	// The chat is now owned with a fresh (chat_id, firstRunner)
	// heartbeat written by the first Acquire.
	firstChat := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, firstWorker, firstChat.WorkerID.UUID)
	require.Equal(t, firstRunner, firstChat.RunnerID.UUID)
	_, err := f.DB.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
		ChatID:   created.Chat.ID,
		RunnerID: firstRunner,
	})
	require.NoError(t, err, "first Acquire wrote a fresh heartbeat")
	// Sanity check: heartbeat is not stale by the same threshold the
	// machine uses for ownership-hint decisions.
	stale, err := f.DB.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
		ChatID:       created.Chat.ID,
		RunnerID:     firstRunner,
		StaleSeconds: chatstate.HeartbeatStaleSeconds,
	})
	require.NoError(t, err)
	require.False(t, stale, "first runner's heartbeat is fresh before the second Acquire")

	// Snapshot publish counts before the takeover so we can assert
	// Acquire does not publish an ownership hint itself.
	ownershipBefore := f.Pub.ownershipPublishCount()
	beforeChat := f.readChat(ctx, t, created.Chat.ID)

	secondWorker := uuid.New()
	secondRunner := uuid.New()
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: secondWorker, RunnerID: secondRunner})
		return err
	}))

	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, secondWorker, after.WorkerID.UUID, "ownership replaced")
	require.Equal(t, secondRunner, after.RunnerID.UUID, "runner replaced")
	require.Equal(t, beforeChat.SnapshotVersion+1, after.SnapshotVersion, "snapshot bumps exactly once")
	f.Pub.expectChatUpdate(t, created.Chat.ID, after.SnapshotVersion)

	// The new (chat_id, secondRunner) heartbeat exists. The old
	// (chat_id, firstRunner) row may or may not exist; Acquire is not
	// responsible for cleaning it up.
	_, err = f.DB.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
		ChatID:   created.Chat.ID,
		RunnerID: secondRunner,
	})
	require.NoError(t, err, "second Acquire wrote a heartbeat for the new runner")

	// Acquire does not publish an ownership hint when it writes a fresh
	// heartbeat. The post-commit ownership-hint logic in Update stays
	// quiet because the new heartbeat is fresh, so no `chat:ownership`
	// notification fires.
	require.Equal(t, ownershipBefore, f.Pub.ownershipPublishCount(),
		"Acquire must not publish an ownership hint when the resulting heartbeat is fresh")
}

// TestTransitionAcquire_ExecutionStateOrthogonal verifies that Acquire
// preserves every execution-state field on the chat across
// representative valid execution states, including idle, runnable, and
// archived states. The transition only mutates ownership.
func TestTransitionAcquire_ExecutionStateOrthogonal(t *testing.T) {
	t.Parallel()

	// Each setup leaves the chat in the named state and returns the
	// chat ID for downstream assertions.
	cases := []struct {
		name  string
		state chatstate.ExecutionState
		setup func(t *testing.T, f *testFixture) uuid.UUID
	}{
		{
			name:  "R0",
			state: chatstate.StateR0,
			setup: func(t *testing.T, f *testFixture) uuid.UUID {
				return createTestChat(t, f).Chat.ID
			},
		},
		{
			name:  "W",
			state: chatstate.StateW,
			setup: func(t *testing.T, f *testFixture) uuid.UUID {
				created := createTestChat(t, f)
				ctx := testutil.Context(t, testutil.WaitShort)
				m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
					_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
					return err
				}))
				return created.Chat.ID
			},
		},
		{
			name:  "E0",
			state: chatstate.StateE0,
			setup: func(t *testing.T, f *testFixture) uuid.UUID {
				created := createTestChat(t, f)
				ctx := testutil.Context(t, testutil.WaitShort)
				m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
					_, err := tx.FinishError(chatstate.FinishErrorInput{
						LastError: pqtype.NullRawMessage{
							RawMessage: json.RawMessage(`{"message":"boom"}`),
							Valid:      true,
						},
					})
					return err
				}))
				return created.Chat.ID
			},
		},
		{
			name:  "I0",
			state: chatstate.StateI0,
			setup: func(t *testing.T, f *testFixture) uuid.UUID {
				created := createTestChat(t, f)
				ctx := testutil.Context(t, testutil.WaitShort)
				m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
					_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
					return err
				}))
				return created.Chat.ID
			},
		},
		{
			name:  "XW",
			state: chatstate.StateXW,
			setup: func(t *testing.T, f *testFixture) uuid.UUID {
				created := createTestChat(t, f)
				ctx := testutil.Context(t, testutil.WaitShort)
				m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
					_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
					return err
				}))
				require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
					_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true})
					return err
				}))
				return created.Chat.ID
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			chatID := tc.setup(t, f)
			require.Equal(t, tc.state, f.classify(ctx, t, chatID), "test setup must leave chat in %s", tc.state)

			before := f.readChat(ctx, t, chatID)
			queueBefore, err := f.DB.CountChatQueuedMessages(ctx, chatID)
			require.NoError(t, err)
			historyBefore := historyMessageIDs(ctx, t, f, chatID)

			worker := uuid.New()
			runner := uuid.New()
			m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)
			require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: worker, RunnerID: runner})
				return err
			}))

			after := f.readChat(ctx, t, chatID)
			// Ownership updated.
			require.Equal(t, worker, after.WorkerID.UUID)
			require.Equal(t, runner, after.RunnerID.UUID)
			// Execution state preserved.
			require.Equal(t, before.Status, after.Status, "status preserved")
			require.Equal(t, before.Archived, after.Archived, "archived flag preserved")
			require.Equal(t, before.RequiresActionDeadlineAt, after.RequiresActionDeadlineAt, "requires-action deadline preserved")
			require.Equal(t, before.LastError, after.LastError, "last_error preserved")
			require.Equal(t, before.HistoryVersion, after.HistoryVersion, "history_version preserved")
			require.Equal(t, before.QueueVersion, after.QueueVersion, "queue_version preserved")
			require.Equal(t, before.GenerationAttempt, after.GenerationAttempt, "generation_attempt preserved")
			// Classified state unchanged.
			require.Equal(t, tc.state, f.classify(ctx, t, chatID), "execution state preserved by Acquire")
			// Queue and history rows untouched.
			queueAfter, err := f.DB.CountChatQueuedMessages(ctx, chatID)
			require.NoError(t, err)
			require.Equal(t, queueBefore, queueAfter, "queue cardinality preserved")
			require.Equal(t, historyBefore, historyMessageIDs(ctx, t, f, chatID), "history preserved")
		})
	}
}
