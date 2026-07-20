package chatstate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
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

// Matrix harness: spec types, scenario labels, appliers, case runners,
// and the single entry point that walks the production transition
// matrix to confirm every allowed combination has positive coverage
// and every disallowed combination surfaces the right sentinel error.

// scenario is a typed, semantic label that distinguishes positive
// matrix cases that share the same (transition, from, want) key.
// Empty scenario is fine when no label is needed. The constants
// below enumerate every label used by matrixCases().
type scenario string

const (
	// scenarioQueue marks SendMessage cases driven by
	// BusyBehaviorQueue.
	scenarioQueue scenario = "queue"
	// scenarioInterrupt marks SendMessage cases driven by
	// BusyBehaviorInterrupt.
	scenarioInterrupt scenario = "interrupt"
	// scenarioMulti marks cases seeded with multiple queued
	// messages so the post-mutation queue stays non-empty.
	scenarioMulti scenario = "multi"
	// scenarioHeadTarget marks multi-queued PromoteQueuedMessage
	// cases that target the queue head. For R1/I1 head-target is
	// reorder-only: no rows are updated, so queue order and
	// queue_version are unchanged. For E1/A1 head-target still
	// pops the head into history.
	scenarioHeadTarget scenario = "head_target"
	// scenarioNonHead marks multi-queued PromoteQueuedMessage cases
	// that target a non-head queued message so the target moves to
	// the head and queue_version advances.
	scenarioNonHead scenario = "non_head"
	// scenarioWithQueue marks ReconcileInvalidState cases seeded
	// with a non-empty queue.
	scenarioWithQueue scenario = "with_queue"
	// scenarioRejectNonDynamicOutstandingToolCall marks the
	// FinishInterruption case that exercises the precondition
	// rejecting outstanding non-dynamic tool calls.
	scenarioRejectNonDynamicOutstandingToolCall scenario = "reject_non_dynamic_outstanding_tool_call"
)

func transitionAllowed(tr chatstate.Transition, from chatstate.ExecutionState) bool {
	return slices.Contains(chatstate.AllowedExecutionTransitionsFrom(from), tr)
}

// expectedErrorForDisallowed returns the sentinel chatstate package
// returns when a transition is attempted from a state where the
// matrix forbids it. N (missing chat) becomes ErrChatNotFound;
// Invalid becomes ErrInvalidState (except for ReconcileInvalidState
// which is allowed); everything else becomes ErrTransitionNotAllowed.
func expectedErrorForDisallowed(tr chatstate.Transition, from chatstate.ExecutionState) error {
	switch from {
	case chatstate.StateN:
		if tr == chatstate.TransitionCreateChat {
			// CreateChat is not exercised through ChatMachine.Update,
			// so this branch is unused in practice. Returning the
			// not-allowed sentinel keeps the helper total.
			return chatstate.ErrTransitionNotAllowed
		}
		return chatstate.ErrChatNotFound
	case chatstate.StateInvalid:
		if tr == chatstate.TransitionReconcileInvalidState {
			return nil
		}
		return chatstate.ErrInvalidState
	}
	return chatstate.ErrTransitionNotAllowed
}

// Transition appliers
//
// Each transition has one default applier that exercises it with
// inputs derived from the seeded chat. Positive case specs reuse these
// appliers unless a case needs a different input shape (for example,
// SendMessage queue versus interrupt from the same source state).
// The disallowed coverage path also uses these defaults.

func applySetArchived(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, from chatstate.ExecutionState, _ *transitionCaseResult) error {
	t.Helper()
	// Archived states unarchive, others archive. For disallowed
	// states the value does not matter; the transition fails first.
	archived := true
	switch from {
	case chatstate.StateXW, chatstate.StateXE0, chatstate.StateXE1:
		archived = false
	}
	_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: archived})
	return err
}

func applySendMessageQueue(t *testing.T, f *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.sendMessage, err = tx.SendMessage(chatstate.SendMessageInput{
		Message:      userTextMessage("sm-queue", f.User.ID, f.Model.ID),
		BusyBehavior: chatstate.BusyBehaviorQueue,
	})
	return err
}

func applySendMessageInterrupt(t *testing.T, f *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.sendMessage, err = tx.SendMessage(chatstate.SendMessageInput{
		Message:      userTextMessage("sm-interrupt", f.User.ID, f.Model.ID),
		BusyBehavior: chatstate.BusyBehaviorInterrupt,
	})
	return err
}

func applyEditMessage(t *testing.T, f *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	content := mustMarshalParts(t, []codersdk.ChatMessagePart{codersdk.ChatMessageText("edited")})
	var err error
	result.editMessage, err = tx.EditMessage(chatstate.EditMessageInput{
		MessageID: seeded.initialUserMessageID,
		CreatedBy: f.User.ID,
		Content:   content,
	})
	return err
}

func applyDeleteQueuedMessage(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var targetQueueID int64
	if len(seeded.queuedMessageIDs) > 0 {
		targetQueueID = seeded.queuedMessageIDs[0]
	}
	var err error
	result.deleteQueuedMessage, err = tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{
		QueuedMessageID: targetQueueID,
	})
	return err
}

func applyPromoteQueuedMessage(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var targetQueueID int64
	if len(seeded.queuedMessageIDs) > 0 {
		targetQueueID = seeded.queuedMessageIDs[0]
	}
	var err error
	result.promoteQueuedMessage, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
		QueuedMessageID: targetQueueID,
	})
	return err
}

func applyInterrupt(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.interrupt, err = tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
	return err
}

func applyCompleteRequiresAction(t *testing.T, f *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var results []chatstate.ToolResultInput
	if seeded.pendingToolCallID != "" {
		results = []chatstate.ToolResultInput{{
			ToolCallID: seeded.pendingToolCallID,
			Output:     json.RawMessage(`{"ok":true}`),
			IsError:    false,
		}}
	}
	var err error
	result.completeRequiresAction, err = tx.CompleteRequiresAction(chatstate.CompleteRequiresActionInput{
		CreatedBy:     f.User.ID,
		ModelConfigID: f.Model.ID,
		Results:       results,
	})
	return err
}

func applyRecordGenerationAttempt(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.recordGenerationAttempt, err = tx.RecordGenerationAttempt(chatstate.RecordGenerationAttemptInput{})
	return err
}

func applyRecordRetryState(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.recordRetryState, err = tx.RecordRetryState(chatstate.RecordRetryStateInput{
		RetryState: pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`),
			Valid:      true,
		},
	})
	return err
}

func applyCommitStep(t *testing.T, f *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	assistant := userTextMessage("assistant", f.User.ID, f.Model.ID)
	assistant.Role = database.ChatMessageRoleAssistant
	var err error
	result.commitStep, err = tx.CommitStep(chatstate.CommitStepInput{
		Messages: []chatstate.Message{assistant},
	})
	return err
}

func applyEnterRequiresAction(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.enterRequiresAction, err = tx.EnterRequiresAction(chatstate.EnterRequiresActionInput{})
	return err
}

func applyFinishInterruption(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.finishInterruption, err = tx.FinishInterruption(chatstate.FinishInterruptionInput{})
	return err
}

func applyFinishTurn(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.finishTurn, err = tx.FinishTurn(chatstate.FinishTurnInput{})
	return err
}

func applyFinishError(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.finishError, err = tx.FinishError(chatstate.FinishErrorInput{
		LastError: pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`{"message":"finish-error"}`),
			Valid:      true,
		},
	})
	return err
}

func applyCancelRequiresAction(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.cancelRequiresAction, err = tx.CancelRequiresAction(chatstate.CancelRequiresActionInput{
		Reason: "cancel from test",
	})
	return err
}

func applyReconcileInvalidState(t *testing.T, _ *testFixture, tx *chatstate.Tx, _ seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
	t.Helper()
	var err error
	result.reconcileInvalidState, err = tx.ReconcileInvalidState(chatstate.ReconcileInvalidStateInput{})
	return err
}

// defaultApplier returns the canonical applier for tr. Used by the
// disallowed coverage path where the input shape does not matter
// because the transition fails before the inputs are consumed.
func defaultApplier(tr chatstate.Transition) applierFn {
	switch tr {
	case chatstate.TransitionSetArchived:
		return applySetArchived
	case chatstate.TransitionSendMessage:
		return applySendMessageQueue
	case chatstate.TransitionEditMessage:
		return applyEditMessage
	case chatstate.TransitionDeleteQueuedMessage:
		return applyDeleteQueuedMessage
	case chatstate.TransitionPromoteQueuedMessage:
		return applyPromoteQueuedMessage
	case chatstate.TransitionInterrupt:
		return applyInterrupt
	case chatstate.TransitionCompleteRequiresAction:
		return applyCompleteRequiresAction
	case chatstate.TransitionRecordGenerationAttempt:
		return applyRecordGenerationAttempt
	case chatstate.TransitionRecordRetryState:
		return applyRecordRetryState
	case chatstate.TransitionCommitStep:
		return applyCommitStep
	case chatstate.TransitionEnterRequiresAction:
		return applyEnterRequiresAction
	case chatstate.TransitionFinishInterruption:
		return applyFinishInterruption
	case chatstate.TransitionFinishTurn:
		return applyFinishTurn
	case chatstate.TransitionFinishError:
		return applyFinishError
	case chatstate.TransitionCancelRequiresAction:
		return applyCancelRequiresAction
	case chatstate.TransitionReconcileInvalidState:
		return applyReconcileInvalidState
	}
	return nil
}

// mustMarshalParts is a tiny test helper that fails the test on
// marshal error rather than forcing every call site to handle it.
func mustMarshalParts(t *testing.T, parts []codersdk.ChatMessagePart) pqtype.NullRawMessage {
	t.Helper()
	raw, err := chatprompt.MarshalParts(parts)
	require.NoError(t, err)
	return raw
}

// Case-level transition matrix spec.
//
// Each entry in matrixCases is one positive (transition, from, want)
// triple. The coverage key is (transition, from, want); scenario is a
// readability-and-semantic suffix for the subtest name that
// distinguishes multiple cases sharing the same coverage key.
// Disallowed combinations are enumerated separately from
// AllowedExecutionTransitionsFrom and AllowedExecutionTransitionOutputs.

type transitionCaseResult struct {
	sendMessage             chatstate.SendMessageResult
	editMessage             chatstate.EditMessageResult
	deleteQueuedMessage     chatstate.DeleteQueuedMessageResult
	promoteQueuedMessage    chatstate.PromoteQueuedMessageResult
	interrupt               chatstate.InterruptResult
	completeRequiresAction  chatstate.CompleteRequiresActionResult
	recordGenerationAttempt chatstate.RecordGenerationAttemptResult
	recordRetryState        chatstate.RecordRetryStateResult
	commitStep              chatstate.CommitStepResult
	enterRequiresAction     chatstate.EnterRequiresActionResult
	finishInterruption      chatstate.FinishInterruptionResult
	finishTurn              chatstate.FinishTurnResult
	finishError             chatstate.FinishErrorResult
	cancelRequiresAction    chatstate.CancelRequiresActionResult
	reconcileInvalidState   chatstate.ReconcileInvalidStateResult
}

type applierFn func(t *testing.T, f *testFixture, tx *chatstate.Tx, seeded seededChat, from chatstate.ExecutionState, result *transitionCaseResult) error

type assertFn func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult)

// seederFn produces a seededChat for a case. Cases that omit a custom
// seeder use seedState by default. Custom seeders are required when
// the case needs more than one queued message, an Invalid chat with a
// non-empty queue, or a transition that needs a fresh A0/A1 seed.
type seederFn func(t *testing.T, f *testFixture, from chatstate.ExecutionState) seededChat

type transitionCaseSpec struct {
	transition chatstate.Transition
	from       chatstate.ExecutionState
	want       chatstate.ExecutionState
	// scenario is a semantic label appended to the subtest name
	// when the same (transition, from, want) key needs to run more
	// than once. It is not part of the coverage key but is part of
	// the duplicate-detection key.
	scenario scenario

	seed          seederFn
	apply         applierFn
	assert        assertFn
	assertFailure func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, err error)
}

// caseKey is the unit of coverage for positive cases. scenario is
// intentionally not part of the key so cases with different scenarios
// can still satisfy the same coverage cell.
type caseKey struct {
	transition chatstate.Transition
	from       chatstate.ExecutionState
	want       chatstate.ExecutionState
}

// fullCaseKey extends caseKey with scenario. Used for duplicate
// detection: two cases must not share the same full key.
type fullCaseKey struct {
	transition chatstate.Transition
	from       chatstate.ExecutionState
	want       chatstate.ExecutionState
	scenario   scenario
}

// queueShape selects the seed variant for transition case builders.
// A typed enum is used instead of a bool to avoid the revive
// flag-parameter rule and to make call sites self-documenting.
type queueShape int

const (
	// queueShapeDefault routes through seedState, which produces the
	// canonical single-queued seed for queue-bearing states and the
	// empty queue for non-queue states.
	queueShapeDefault queueShape = iota
	// queueShapeMulti routes through seedStateMultiQueued (or
	// seedInvalidWithQueue for ReconcileInvalidState) so the
	// post-mutation queue can remain non-empty.
	queueShapeMulti
)

func (s queueShape) isMulti() bool { return s == queueShapeMulti }

func (s transitionCaseSpec) key() caseKey {
	return caseKey{transition: s.transition, from: s.from, want: s.want}
}

func (s transitionCaseSpec) fullKey() fullCaseKey {
	return fullCaseKey{
		transition: s.transition,
		from:       s.from,
		want:       s.want,
		scenario:   s.scenario,
	}
}

func (s transitionCaseSpec) subtestName() string {
	name := fmt.Sprintf("%s/%s_to_%s", s.transition, s.from, s.want)
	if s.scenario != "" {
		name += "/" + string(s.scenario)
	}
	return name
}

// disallowedCaseKey is the unit of coverage for negative cases.
type disallowedCaseKey struct {
	transition chatstate.Transition
	from       chatstate.ExecutionState
}

// remainingExcluding returns ids with the entry at exclude removed.
// The order of the surviving entries is preserved.
func remainingExcluding(ids []int64, exclude int) []int64 {
	out := make([]int64, 0, len(ids))
	for i, id := range ids {
		if i == exclude {
			continue
		}
		out = append(out, id)
	}
	return out
}

// remainingBodiesExcluding returns bodies with the entry at exclude
// removed. The order of the surviving entries is preserved.
func remainingBodiesExcluding(bodies []string, exclude int) []string {
	out := make([]string, 0, len(bodies))
	for i, b := range bodies {
		if i == exclude {
			continue
		}
		out = append(out, b)
	}
	return out
}

// Test runner

// runPositiveCase seeds the chat, runs the transition, and asserts the
// post-state plus case-specific effects.
func runPositiveCase(t *testing.T, spec transitionCaseSpec) {
	t.Helper()
	require.NotNil(t, spec.apply, "case %s missing apply", spec.subtestName())
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	seeder := spec.seed
	if seeder == nil {
		seeder = seedState
	}
	seeded := seeder(t, f, spec.from)
	if seeded.exists {
		require.Equal(t, spec.from, f.classify(ctx, t, seeded.chatID),
			"seed must land in %s", spec.from)
	}
	base := captureBaseline(ctx, t, f, seeded)

	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	var result transitionCaseResult
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		return spec.apply(t, f, tx, seeded, spec.from, &result)
	})
	if spec.assertFailure != nil {
		spec.assertFailure(ctx, t, f, seeded, base, err)
		return
	}
	require.NoError(t, err, "%s from %s must succeed", spec.transition, spec.from)
	assertSnapshotBumpedOnce(ctx, t, f, seeded.chatID, base)
	require.Equal(t, spec.want, f.classify(ctx, t, seeded.chatID),
		"%s: %s -> %s", spec.transition, spec.from, spec.want)
	if spec.assert != nil {
		spec.assert(ctx, t, f, seeded, base, result)
	}
}

// runDisallowedCase seeds the chat, runs the transition with default
// inputs, and asserts that the chatstate package surfaces the right
// sentinel error and rolled the snapshot bump back.
func runDisallowedCase(t *testing.T, tr chatstate.Transition, from chatstate.ExecutionState) {
	t.Helper()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedState(t, f, from)
	if seeded.exists {
		require.Equal(t, from, f.classify(ctx, t, seeded.chatID),
			"disallowed seed must land in %s", from)
	}
	base := captureBaseline(ctx, t, f, seeded)

	applier := defaultApplier(tr)
	require.NotNil(t, applier, "no default applier for transition %s", tr)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)
	var result transitionCaseResult
	err := m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		return applier(t, f, tx, seeded, from, &result)
	})

	if tr == chatstate.TransitionReconcileInvalidState && from != chatstate.StateN {
		// ReconcileInvalidState does not use requireFromAllowed.
		// It hits loadState successfully, sees the state is not
		// Invalid, and returns a TransitionError directly.
		require.Error(t, err)
		var te *chatstate.TransitionError
		require.ErrorAs(t, err, &te,
			"reconcile from non-invalid state must return TransitionError")
		require.Equal(t, chatstate.TransitionReconcileInvalidState, te.Transition)
		require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)
		assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
		return
	}

	expectErr := expectedErrorForDisallowed(tr, from)
	require.Error(t, err)
	require.ErrorIs(t, err, expectErr)
	assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
}

// TestTransitionMatrix_AllCombinations is the single entry point for
// the case-level transition matrix coverage. Each positive case in
// matrixCases() is one (transition, from, want) triple with a focused
// effect assertion. Disallowed combinations are enumerated from
// transition.go to confirm every non-CreateChat (transition, from)
// pair outside the allowed set surfaces the right sentinel error.
//
// After all parallel subtests complete the test verifies that the
// positive coverage matches AllowedExecutionTransitionOutputs (no
// missing key, no unexpected key) and that every disallowed
// (transition, from) pair was exercised exactly once.
//
// Input-specific rejection tests live in TestTransitionInputValidation
// and are intentionally not part of this matrix entry point so the
// matrix focus stays on positive cases and generated disallowed cases.
func TestTransitionMatrix_AllCombinations(t *testing.T) {
	t.Parallel()

	cases := matrixCases()

	// Detect duplicate full keys and duplicate subtest names. The
	// coverage key intentionally ignores scenario, so two cases may
	// share the same (transition, from, want) only when their
	// scenarios differ.
	seenFullKeys := make(map[fullCaseKey]string, len(cases))
	seenNames := make(map[string]struct{}, len(cases))
	for _, tc := range cases {
		full := tc.fullKey()
		name := tc.subtestName()
		if prev, ok := seenFullKeys[full]; ok {
			t.Fatalf("duplicate matrix case %+v: previous %s, new %s", full, prev, name)
		}
		seenFullKeys[full] = name
		if _, ok := seenNames[name]; ok {
			t.Fatalf("duplicate matrix subtest name %s", name)
		}
		seenNames[name] = struct{}{}
	}

	// Build the expected positive set from the matrix in
	// transition.go. CreateChat is intentionally excluded because
	// it is not exercised via ChatMachine.Update.
	expectedPositive := make(map[caseKey]struct{})
	for _, from := range chatstate.AllExecutionStates {
		for _, tr := range chatstate.AllowedExecutionTransitionsFrom(from) {
			if tr == chatstate.TransitionCreateChat {
				continue
			}
			for _, to := range chatstate.AllowedExecutionTransitionOutputs(from, tr) {
				expectedPositive[caseKey{transition: tr, from: from, want: to}] = struct{}{}
			}
		}
	}

	// Build the expected disallowed set: for each non-CreateChat
	// transition, every state where the transition is not allowed.
	expectedDisallowed := make(map[disallowedCaseKey]struct{})
	for _, tr := range chatstate.AllExecutionTransitions {
		if tr == chatstate.TransitionCreateChat {
			continue
		}
		for _, from := range chatstate.AllExecutionStates {
			if transitionAllowed(tr, from) {
				continue
			}
			expectedDisallowed[disallowedCaseKey{transition: tr, from: from}] = struct{}{}
		}
	}

	// Validate that every case in matrixCases describes a
	// (transition, from, want) combination that the matrix actually
	// admits. This guards against typos in matrixCases wiring up
	// nonsense cases that happen to compile.
	for _, tc := range cases {
		if tc.assertFailure != nil {
			continue
		}
		key := tc.key()
		_, ok := expectedPositive[key]
		require.True(t, ok,
			"case %s is not in the allowed (transition, from, want) set", tc.subtestName())
	}

	// actualPositive and actualDisallowed are mutated under mu from
	// parallel subtests. The final comparison runs in t.Cleanup,
	// which fires only after every parallel child finishes.
	var mu sync.Mutex
	actualPositive := make(map[caseKey]struct{}, len(expectedPositive))
	actualDisallowed := make(map[disallowedCaseKey]struct{}, len(expectedDisallowed))

	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for k := range expectedPositive {
			if _, ok := actualPositive[k]; !ok {
				t.Errorf("matrix coverage: missing positive case %+v", k)
			}
		}
		for k := range actualPositive {
			if _, ok := expectedPositive[k]; !ok {
				t.Errorf("matrix coverage: unexpected positive case %+v", k)
			}
		}
		for k := range expectedDisallowed {
			if _, ok := actualDisallowed[k]; !ok {
				t.Errorf("matrix coverage: missing disallowed case %+v", k)
			}
		}
		for k := range actualDisallowed {
			if _, ok := expectedDisallowed[k]; !ok {
				t.Errorf("matrix coverage: unexpected disallowed case %+v", k)
			}
		}
	})

	// Positive cases: one parallel subtest per case.
	t.Run("positive", func(t *testing.T) {
		t.Parallel()
		for _, tc := range cases {
			tc := tc
			t.Run(tc.subtestName(), func(t *testing.T) {
				t.Parallel()
				if tc.assertFailure == nil {
					mu.Lock()
					actualPositive[tc.key()] = struct{}{}
					mu.Unlock()
				}
				runPositiveCase(t, tc)
			})
		}
	})

	// Negative cases: one parallel subtest per (transition, from)
	// pair where the transition is not allowed. Iterate over
	// transitions in canonical order, and within each transition
	// iterate states in canonical AllExecutionStates order, so
	// subtest names are stable.
	t.Run("disallowed", func(t *testing.T) {
		t.Parallel()
		// Sort disallowed keys for deterministic subtest names.
		// AllExecutionTransitions and AllExecutionStates are
		// already canonical, so iterate in their order.
		for _, tr := range chatstate.AllExecutionTransitions {
			tr := tr
			if tr == chatstate.TransitionCreateChat {
				continue
			}
			t.Run(string(tr), func(t *testing.T) {
				t.Parallel()
				for _, from := range chatstate.AllExecutionStates {
					from := from
					if transitionAllowed(tr, from) {
						continue
					}
					t.Run(string(from), func(t *testing.T) {
						t.Parallel()
						mu.Lock()
						actualDisallowed[disallowedCaseKey{transition: tr, from: from}] = struct{}{}
						mu.Unlock()
						runDisallowedCase(t, tr, from)
					})
				}
			})
		}
	})
}

// Positive case specs.
//
// Each case asserts (at minimum) the resulting classified post-state
// matches want, plus one transition-specific effect. Helpers reused
// from other tests handle the snapshot bump and the chat:update
// publish; per-case assertions focus on what the transition meant to
// change.

func matrixCases() []transitionCaseSpec {
	return []transitionCaseSpec{
		// SetArchived cases: each archived/unarchived pair flips the
		// archived flag, preserves status, history and last_error,
		// and does not insert anything new.
		setArchivedCase(chatstate.StateW, chatstate.StateXW, database.ChatStatusWaiting),
		setArchivedCase(chatstate.StateE0, chatstate.StateXE0, database.ChatStatusError),
		setArchivedCase(chatstate.StateE1, chatstate.StateXE1, database.ChatStatusError),
		setArchivedCase(chatstate.StateXW, chatstate.StateW, database.ChatStatusWaiting),
		setArchivedCase(chatstate.StateXE0, chatstate.StateE0, database.ChatStatusError),
		setArchivedCase(chatstate.StateXE1, chatstate.StateE1, database.ChatStatusError),

		// SendMessage(queue) cases: idle states insert directly,
		// busy states append to the queue tail.
		sendMessageQueueCase(chatstate.StateW, chatstate.StateR0, true, 0),
		sendMessageQueueCase(chatstate.StateE0, chatstate.StateR0, true, 0),
		// E1 promotes the queue head and queues the new tail, so
		// the net queue delta is zero.
		sendMessageQueueCase(chatstate.StateE1, chatstate.StateR1, false, 0),
		sendMessageQueueCase(chatstate.StateR0, chatstate.StateR1, false, +1),
		sendMessageQueueCase(chatstate.StateR1, chatstate.StateR1, false, +1),
		sendMessageQueueCase(chatstate.StateI0, chatstate.StateI1, false, +1),
		sendMessageQueueCase(chatstate.StateI1, chatstate.StateI1, false, +1),
		sendMessageQueueCase(chatstate.StateA0, chatstate.StateA1, false, +1),
		sendMessageQueueCase(chatstate.StateA1, chatstate.StateA1, false, +1),

		// SendMessage(interrupt) cases. The interrupt applier runs
		// with body "sm-interrupt" so the assertion can prove the
		// interrupt input path was taken. From W/E0/E1/I0/I1 the
		// resulting (transition, from, want) coverage key is
		// identical to the queue case, but we still exercise the
		// interrupt entry point to guard against a future bug where
		// it stops routing through the correct direct-insert /
		// queue-tail / promotion paths. From the busy R0/R1/A0/A1
		// states the interrupt destination differs from the queue
		// destination so the scenario label is the only case for that key.
		sendMessageInterruptCase(chatstate.StateW, chatstate.StateR0),
		sendMessageInterruptCase(chatstate.StateE0, chatstate.StateR0),
		sendMessageInterruptCase(chatstate.StateE1, chatstate.StateR1),
		sendMessageInterruptCase(chatstate.StateR0, chatstate.StateI1),
		sendMessageInterruptCase(chatstate.StateR1, chatstate.StateI1),
		sendMessageInterruptCase(chatstate.StateI0, chatstate.StateI1),
		sendMessageInterruptCase(chatstate.StateI1, chatstate.StateI1),
		sendMessageInterruptCase(chatstate.StateA0, chatstate.StateR1),
		sendMessageInterruptCase(chatstate.StateA1, chatstate.StateR1),

		// EditMessage cases: every allowed source state lands in R0
		// with the queue cleared, last_error reset, and a
		// replacement user message in active history.
		editMessageCase(chatstate.StateW),
		editMessageCase(chatstate.StateE0),
		editMessageCase(chatstate.StateE1),
		editMessageCase(chatstate.StateR0),
		editMessageCase(chatstate.StateR1),
		editMessageCase(chatstate.StateI0),
		editMessageCase(chatstate.StateI1),
		editMessageCase(chatstate.StateA0),
		editMessageCase(chatstate.StateA1),

		// DeleteQueuedMessage cases. Empty-tail want collapses the
		// classified state (E1->E0, R1->R0, I1->I0, A1->A0). The
		// non-empty-tail cases need a multi-queued seed.
		deleteQueuedCase(chatstate.StateE1, chatstate.StateE0, queueShapeDefault),
		deleteQueuedCase(chatstate.StateE1, chatstate.StateE1, queueShapeMulti),
		deleteQueuedCase(chatstate.StateR1, chatstate.StateR0, queueShapeDefault),
		deleteQueuedCase(chatstate.StateR1, chatstate.StateR1, queueShapeMulti),
		deleteQueuedCase(chatstate.StateI1, chatstate.StateI0, queueShapeDefault),
		deleteQueuedCase(chatstate.StateI1, chatstate.StateI1, queueShapeMulti),
		deleteQueuedCase(chatstate.StateA1, chatstate.StateA0, queueShapeDefault),
		deleteQueuedCase(chatstate.StateA1, chatstate.StateA1, queueShapeMulti),

		// PromoteQueuedMessage cases. E1/A1 pop the head into
		// history; R1/I1 only reorder the queue without
		// inserting history. R1/I1 has both a head-target
		// scenario (zero rows updated, queue_version unchanged)
		// and a non-head scenario (target moves to head,
		// queue_version advances).
		promoteQueuedCase(chatstate.StateE1, chatstate.StateR0, queueShapeDefault, 0),
		promoteQueuedCase(chatstate.StateE1, chatstate.StateR1, queueShapeMulti, 0),
		promoteQueuedCase(chatstate.StateR1, chatstate.StateI1, queueShapeMulti, 0),
		promoteQueuedCase(chatstate.StateR1, chatstate.StateI1, queueShapeMulti, 1),
		promoteQueuedCase(chatstate.StateI1, chatstate.StateI1, queueShapeMulti, 1),
		promoteQueuedCase(chatstate.StateA1, chatstate.StateR0, queueShapeDefault, 0),
		promoteQueuedCase(chatstate.StateA1, chatstate.StateR1, queueShapeMulti, 0),

		// Interrupt cases.
		interruptCase(chatstate.StateR0, chatstate.StateI0),
		interruptCase(chatstate.StateR1, chatstate.StateI1),
		interruptCase(chatstate.StateA0, chatstate.StateR0),
		interruptCase(chatstate.StateA1, chatstate.StateR1),

		// CompleteRequiresAction cases: A0->R0, A1->R1.
		completeRequiresActionCase(chatstate.StateA0, chatstate.StateR0),
		completeRequiresActionCase(chatstate.StateA1, chatstate.StateR1),

		// CancelRequiresAction cases: A0->R0, A1->R1.
		cancelRequiresActionCase(chatstate.StateA0, chatstate.StateR0),
		cancelRequiresActionCase(chatstate.StateA1, chatstate.StateR1),

		// RecordGenerationAttempt cases: from-state preserved.
		recordGenerationAttemptCase(chatstate.StateR0),
		recordGenerationAttemptCase(chatstate.StateR1),

		// RecordRetryState cases: from-state preserved.
		recordRetryStateCase(chatstate.StateR0),
		recordRetryStateCase(chatstate.StateR1),

		// CommitStep cases: from-state preserved, history grows by
		// one message.
		commitStepCase(chatstate.StateR0),
		commitStepCase(chatstate.StateR1),

		// EnterRequiresAction cases. R0/R1 need a pending tool call
		// seeded; use seedForEnterRequiresAction so the precondition
		// is met.
		enterRequiresActionCase(chatstate.StateR0, chatstate.StateA0),
		enterRequiresActionCase(chatstate.StateR1, chatstate.StateA1),

		// FinishInterruption cases: I0->W, I1->R0 (head promoted into
		// history when only one queued), I1->R1 (with more than one
		// queued, the head is promoted but the queue stays
		// non-empty).
		finishInterruptionCase(chatstate.StateI0, chatstate.StateW, queueShapeDefault),
		finishInterruptionRejectsOutstandingToolCallCase(),
		finishInterruptionCase(chatstate.StateI1, chatstate.StateR0, queueShapeDefault),
		finishInterruptionCase(chatstate.StateI1, chatstate.StateR1, queueShapeMulti),

		// FinishTurn cases.
		finishTurnCase(chatstate.StateR0, chatstate.StateW, queueShapeDefault),
		finishTurnCase(chatstate.StateR1, chatstate.StateR0, queueShapeDefault),
		finishTurnCase(chatstate.StateR1, chatstate.StateR1, queueShapeMulti),

		// FinishError cases.
		finishErrorCase(chatstate.StateR0, chatstate.StateE0),
		finishErrorCase(chatstate.StateR1, chatstate.StateE1),

		// ReconcileInvalidState cases: Invalid with empty queue
		// lands in E0; Invalid with non-empty queue lands in E1.
		reconcileInvalidStateCase(chatstate.StateE0, queueShapeDefault),
		reconcileInvalidStateCase(chatstate.StateE1, queueShapeMulti),
	}
}

func setArchivedCase(from, want chatstate.ExecutionState, wantStatus database.ChatStatus) transitionCaseSpec {
	wantArchived := false
	switch want {
	case chatstate.StateXW, chatstate.StateXE0, chatstate.StateXE1:
		wantArchived = true
	}
	return transitionCaseSpec{
		transition: chatstate.TransitionSetArchived,
		from:       from,
		want:       want,
		apply:      applySetArchived,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			_ = result
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, wantArchived, after.Archived,
				"SetArchived must set archived=%v", wantArchived)
			require.Equal(t, wantStatus, after.Status,
				"SetArchived preserves chat status")
			require.Equal(t, base.chat.LastError, after.LastError,
				"SetArchived preserves last_error")
			require.Equal(t, base.historyVersion, after.HistoryVersion,
				"SetArchived does not insert history")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"SetArchived leaves history messages unchanged")
			require.Equal(t, base.queueVersion, after.QueueVersion,
				"SetArchived does not mutate queued messages")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"SetArchived leaves queued messages unchanged")
		},
	}
}

func sendMessageQueueCase(from, want chatstate.ExecutionState, directInsert bool, queueDelta int64) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       from,
		want:       want,
		scenario:   scenarioQueue,
		apply:      applySendMessageQueue,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			afterQueue, err := f.DB.CountChatQueuedMessages(ctx, seeded.chatID)
			require.NoError(t, err)
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			afterQueueIDs := queuedIDsByPosition(ctx, t, f, seeded.chatID)

			require.Equal(t, base.queueCount+queueDelta, afterQueue,
				"SendMessage(queue): unexpected queue count delta")

			switch {
			case directInsert:
				// W/E0: insert directly into history, no queue
				// mutation. result.InsertedMessages contains exactly
				// the new user message.
				require.Len(t, result.sendMessage.InsertedMessages, 1,
					"SendMessage(queue) into W/E0 inserts exactly one history message")
				require.Nil(t, result.sendMessage.QueuedMessage,
					"SendMessage(queue) into W/E0 does not queue")
				inserted := assertFetchedUserMessage(ctx, t, f, result.sendMessage.InsertedMessages[0])
				require.Equal(t, seeded.chatID, inserted.ChatID)
				assertChatMessageText(t, inserted, "sm-queue")
				require.False(t, after.LastError.Valid,
					"SendMessage(queue) clears last_error when transitioning out of an error state")
				require.Equal(t, database.ChatStatusRunning, after.Status,
					"SendMessage(queue) into W/E0 lands in running")
				require.Equal(t, base.queueIDs, afterQueueIDs,
					"SendMessage(queue) into W/E0 must not touch queued messages")
				require.Equal(t, base.queueVersion, after.QueueVersion,
					"SendMessage(queue) into W/E0 must not bump queue_version")
				require.Equal(t, append([]int64{}, base.historyIDs...), afterHistory[:len(base.historyIDs)],
					"SendMessage(queue) into W/E0 leaves the existing history prefix intact")
				require.Equal(t, []int64{inserted.ID}, newActiveMessageIDs(base, afterHistory),
					"SendMessage(queue) into W/E0 appends exactly the new user message")

			case from == chatstate.StateE1:
				// E1: the previous head is promoted into history
				// and replaced by the new tail. Net queue size
				// unchanged.
				require.NotNil(t, result.sendMessage.QueuedMessage,
					"SendMessage(queue) from E1 returns the new queued tail")
				require.Len(t, result.sendMessage.InsertedMessages, 1,
					"SendMessage(queue) from E1 promotes the previous head into history")
				promoted := assertFetchedUserMessage(ctx, t, f, result.sendMessage.InsertedMessages[0])
				require.Equal(t, seeded.chatID, promoted.ChatID)
				require.NotEmpty(t, seeded.queuedMessageBodies,
					chatstate.StateE1.String()+" seed must record the queue head body")
				assertChatMessageText(t, promoted, seeded.queuedMessageBodies[0])
				newQueued := assertFetchedQueuedMessage(ctx, t, f, seeded.chatID, *result.sendMessage.QueuedMessage)
				assertQueuedMessageText(t, newQueued, "sm-queue")
				// Previous head queued message is gone from the
				// queue and now lives in history.
				require.NotEmpty(t, base.queueIDs,
					chatstate.StateE1.String()+" seed must have a queue head")
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, base.queueIDs[0])
				require.Equal(t, []int64{newQueued.ID}, afterQueueIDs,
					chatstate.StateE1.String()+" -> "+chatstate.StateR1.String()+
						": queue must end with only the new tail")
				require.False(t, after.LastError.Valid,
					chatstate.StateE1.String()+" -> "+chatstate.StateR1.String()+
						" clears last_error")
				require.Equal(t, database.ChatStatusRunning, after.Status)
				require.Equal(t, []int64{promoted.ID}, newActiveMessageIDs(base, afterHistory),
					chatstate.StateE1.String()+" -> "+chatstate.StateR1.String()+
						" inserts only the promoted user message")
				require.Greater(t, after.QueueVersion, base.queueVersion,
					chatstate.StateE1.String()+" -> "+chatstate.StateR1.String()+
						" advances queue_version")

			default:
				// Busy states: the new user message is appended at
				// the queue tail; history is untouched.
				require.NotNil(t, result.sendMessage.QueuedMessage,
					"SendMessage(queue) from busy states returns the queued message")
				require.Empty(t, result.sendMessage.InsertedMessages,
					"SendMessage(queue) from busy states does not insert history")
				newQueued := assertFetchedQueuedMessage(ctx, t, f, seeded.chatID, *result.sendMessage.QueuedMessage)
				assertQueuedMessageText(t, newQueued, "sm-queue")
				wantQueue := append(append([]int64{}, base.queueIDs...), newQueued.ID)
				require.Equal(t, wantQueue, afterQueueIDs,
					"SendMessage(queue) from busy states appends to the queue tail")
				require.Equal(t, base.historyIDs, afterHistory,
					"SendMessage(queue) from busy states does not change history")
				require.Greater(t, after.QueueVersion, base.queueVersion,
					"SendMessage(queue) from busy states advances queue_version")
				switch from {
				case chatstate.StateA0, chatstate.StateA1:
					require.True(t, after.RequiresActionDeadlineAt.Valid,
						"SendMessage(queue) from A* preserves requires_action_deadline_at")
					require.Equal(t, base.chat.RequiresActionDeadlineAt, after.RequiresActionDeadlineAt,
						"SendMessage(queue) from A* preserves the deadline value")
					require.Equal(t, database.ChatStatusRequiresAction, after.Status)
				case chatstate.StateI0, chatstate.StateI1:
					require.Equal(t, database.ChatStatusInterrupting, after.Status)
				case chatstate.StateR0, chatstate.StateR1:
					require.Equal(t, database.ChatStatusRunning, after.Status)
				}
			}
		},
	}
}

func sendMessageInterruptCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionSendMessage,
		from:       from,
		want:       want,
		scenario:   scenarioInterrupt,
		apply:      applySendMessageInterrupt,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			afterQueue, err := f.DB.CountChatQueuedMessages(ctx, seeded.chatID)
			require.NoError(t, err)
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			afterQueueIDs := queuedIDsByPosition(ctx, t, f, seeded.chatID)

			switch from {
			case chatstate.StateW, chatstate.StateE0:
				// W/E0 with interrupt-mode behaves like the direct
				// insert from queue-mode: the new user message lands
				// directly in history, the queue is left untouched,
				// last_error is cleared, and the chat lands in R0.
				require.Equal(t, base.queueCount, afterQueue,
					"SendMessage(interrupt) into W/E0 must not queue")
				require.Nil(t, result.sendMessage.QueuedMessage,
					"SendMessage(interrupt) into W/E0 does not return a queued message")
				require.Len(t, result.sendMessage.InsertedMessages, 1,
					"SendMessage(interrupt) into W/E0 inserts exactly one history message")
				inserted := assertFetchedUserMessage(ctx, t, f, result.sendMessage.InsertedMessages[0])
				require.Equal(t, seeded.chatID, inserted.ChatID)
				assertChatMessageText(t, inserted, "sm-interrupt")
				require.False(t, after.LastError.Valid,
					"SendMessage(interrupt) into W/E0 clears last_error")
				require.Equal(t, database.ChatStatusRunning, after.Status,
					"SendMessage(interrupt) into W/E0 lands in running")
				require.Equal(t, base.queueIDs, afterQueueIDs,
					"SendMessage(interrupt) into W/E0 must not touch queued messages")
				require.Equal(t, []int64{inserted.ID}, newActiveMessageIDs(base, afterHistory),
					"SendMessage(interrupt) into W/E0 appends exactly the new user message")

			case chatstate.StateE1:
				// E1 with interrupt-mode mirrors queue-mode: the
				// previous head is promoted into history and the new
				// tail replaces it in the queue. Net queue size
				// unchanged, last_error cleared.
				require.Equal(t, base.queueCount, afterQueue,
					"SendMessage(interrupt) from E1 leaves queue size unchanged")
				require.NotNil(t, result.sendMessage.QueuedMessage,
					"SendMessage(interrupt) from E1 returns the new queued tail")
				require.Len(t, result.sendMessage.InsertedMessages, 1,
					"SendMessage(interrupt) from E1 promotes the previous head into history")
				promoted := assertFetchedUserMessage(ctx, t, f, result.sendMessage.InsertedMessages[0])
				require.Equal(t, seeded.chatID, promoted.ChatID)
				require.NotEmpty(t, seeded.queuedMessageBodies,
					chatstate.StateE1.String()+" seed must record queue head body")
				assertChatMessageText(t, promoted, seeded.queuedMessageBodies[0])
				newQueued := assertFetchedQueuedMessage(ctx, t, f, seeded.chatID, *result.sendMessage.QueuedMessage)
				assertQueuedMessageText(t, newQueued, "sm-interrupt")
				require.NotEmpty(t, base.queueIDs,
					chatstate.StateE1.String()+" seed must have a queue head")
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, base.queueIDs[0])
				require.Equal(t, []int64{newQueued.ID}, afterQueueIDs,
					chatstate.StateE1.String()+" -> "+chatstate.StateR1.String()+
						" interrupt: queue must end with only the new tail")
				require.False(t, after.LastError.Valid)
				require.Equal(t, database.ChatStatusRunning, after.Status)
				require.Equal(t, []int64{promoted.ID}, newActiveMessageIDs(base, afterHistory),
					chatstate.StateE1.String()+" -> "+chatstate.StateR1.String()+
						" interrupt inserts only the promoted user message")
				require.Greater(t, after.QueueVersion, base.queueVersion,
					chatstate.StateE1.String()+" -> "+chatstate.StateR1.String()+
						" interrupt advances queue_version")

			case chatstate.StateI0, chatstate.StateI1:
				// I*: append to queue tail, history untouched, status
				// stays interrupting.
				require.Equal(t, base.queueCount+1, afterQueue,
					"SendMessage(interrupt) from I* appends one queued message")
				require.NotNil(t, result.sendMessage.QueuedMessage,
					"SendMessage(interrupt) from I* returns the queued tail")
				require.Empty(t, result.sendMessage.InsertedMessages,
					"SendMessage(interrupt) from I* does not insert history")
				newQueued := assertFetchedQueuedMessage(ctx, t, f, seeded.chatID, *result.sendMessage.QueuedMessage)
				assertQueuedMessageText(t, newQueued, "sm-interrupt")
				wantQueue := append(append([]int64{}, base.queueIDs...), newQueued.ID)
				require.Equal(t, wantQueue, afterQueueIDs,
					"SendMessage(interrupt) from I* appends to the queue tail")
				require.Equal(t, base.historyIDs, afterHistory,
					"SendMessage(interrupt) from I* must not touch history")
				require.Equal(t, database.ChatStatusInterrupting, after.Status,
					"SendMessage(interrupt) from I* keeps status interrupting")
				require.Greater(t, after.QueueVersion, base.queueVersion,
					"SendMessage(interrupt) from I* advances queue_version")

			case chatstate.StateR0, chatstate.StateR1:
				require.Equal(t, base.queueCount+1, afterQueue,
					"SendMessage(interrupt) from R* appends one queued message")
				require.NotNil(t, result.sendMessage.QueuedMessage,
					"SendMessage(interrupt) from R* returns the queued tail")
				newQueued := assertFetchedQueuedMessage(ctx, t, f, seeded.chatID, *result.sendMessage.QueuedMessage)
				assertQueuedMessageText(t, newQueued, "sm-interrupt")
				wantQueue := append(append([]int64{}, base.queueIDs...), newQueued.ID)
				require.Equal(t, wantQueue, afterQueueIDs,
					"SendMessage(interrupt) from R* appends to the queue tail")
				require.Greater(t, after.QueueVersion, base.queueVersion,
					"SendMessage(interrupt) from R* advances queue_version")
				require.Equal(t, database.ChatStatusInterrupting, after.Status,
					"R* -> I1 sets status interrupting")
				require.Equal(t, base.historyIDs, afterHistory,
					"SendMessage(interrupt) from R* must not touch history")

			case chatstate.StateA0, chatstate.StateA1:
				require.Equal(t, base.queueCount+1, afterQueue,
					"SendMessage(interrupt) from A* appends one queued message")
				require.NotNil(t, result.sendMessage.QueuedMessage,
					"SendMessage(interrupt) from A* returns the queued tail")
				newQueued := assertFetchedQueuedMessage(ctx, t, f, seeded.chatID, *result.sendMessage.QueuedMessage)
				assertQueuedMessageText(t, newQueued, "sm-interrupt")
				wantQueue := append(append([]int64{}, base.queueIDs...), newQueued.ID)
				require.Equal(t, wantQueue, afterQueueIDs,
					"SendMessage(interrupt) from A* appends to the queue tail")
				require.Greater(t, after.QueueVersion, base.queueVersion,
					"SendMessage(interrupt) from A* advances queue_version")
				require.Equal(t, database.ChatStatusRunning, after.Status,
					"A* -> R1 cancels pending dynamic calls and resumes running")
				require.False(t, after.RequiresActionDeadlineAt.Valid,
					"A* -> R1 clears requires_action_deadline_at")
				// Cancellation messages for the pending dynamic
				// tool call should land in active history. They are
				// not returned via SendMessageResult, so we fetch
				// them by diffing the active history set.
				newIDs := newActiveMessageIDs(base, afterHistory)
				require.Len(t, newIDs, 1,
					"SendMessage(interrupt) from A* synthesizes exactly one tool-result cancellation")
				cancel := requireChatMessageByID(ctx, t, f, newIDs[0])
				assertToolResultForCall(t, cancel, seeded.pendingToolCallID)
			}
		},
	}
}

func editMessageCase(from chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionEditMessage,
		from:       from,
		want:       chatstate.StateR0,
		apply:      applyEditMessage,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusRunning, after.Status,
				"EditMessage always lands in running")
			require.False(t, after.Archived, "EditMessage clears archived")
			require.False(t, after.LastError.Valid,
				"EditMessage clears last_error")
			count, err := f.DB.CountChatQueuedMessages(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Zero(t, count, "EditMessage clears the queue")
			require.Empty(t, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"EditMessage leaves no queued messages")

			// Replacement message must be a fresh user message that
			// replaces the original target and lives in active history.
			require.NotZero(t, result.editMessage.ReplacementMessage.ID,
				"EditMessage returns the replacement message")
			replacement := assertFetchedUserMessage(ctx, t, f, result.editMessage.ReplacementMessage)
			require.Equal(t, seeded.chatID, replacement.ChatID)
			require.NotEqual(t, seeded.initialUserMessageID, replacement.ID,
				"EditMessage inserts a new replacement message")
			assertChatMessageText(t, replacement, "edited")

			// Every history message from the edited message onward,
			// inclusive, must be soft-deleted. base.historyIDs is the
			// active history in order before the transition, so the
			// expected deleted suffix is everything from the target's
			// position to the end of that slice. GetChatMessageByID
			// filters deleted=false, so it must return an error for
			// each deleted ID.
			require.NotEmpty(t, result.editMessage.DeletedMessageIDs,
				"EditMessage deletes at least the target user message")
			targetIdx := slices.Index(base.historyIDs, seeded.initialUserMessageID)
			require.GreaterOrEqual(t, targetIdx, 0,
				"baseline active history must contain the edited message")
			wantDeleted := append([]int64{}, base.historyIDs[targetIdx:]...)
			require.Equal(t, wantDeleted, result.editMessage.DeletedMessageIDs,
				"EditMessage soft-deletes the edited message and every later active history message in order")
			for _, id := range result.editMessage.DeletedMessageIDs {
				_, err := f.DB.GetChatMessageByID(ctx, id)
				require.Error(t, err,
					"EditMessage: deleted message %d must not be active", id)
			}
			// Every deleted queued message must be gone from the queue.
			for _, id := range result.editMessage.DeletedQueuedMessageIDs {
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, id)
			}
			for _, id := range base.queueIDs {
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, id)
			}
		},
	}
}

func deleteQueuedCase(from, want chatstate.ExecutionState, shape queueShape) transitionCaseSpec {
	spec := transitionCaseSpec{
		transition: chatstate.TransitionDeleteQueuedMessage,
		from:       from,
		want:       want,
		apply:      applyDeleteQueuedMessage,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			afterQueue, err := f.DB.CountChatQueuedMessages(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, base.queueCount-1, afterQueue,
				"DeleteQueuedMessage removes exactly one queued message")
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Greater(t, after.QueueVersion, base.queueVersion,
				"DeleteQueuedMessage advances queue_version")

			// The target queued message is the seeded head. It must
			// be returned in DeletedQueuedMessage, and it must no
			// longer be fetchable.
			require.NotEmpty(t, seeded.queuedMessageIDs)
			targetID := seeded.queuedMessageIDs[0]
			require.Equal(t, targetID, result.deleteQueuedMessage.DeletedQueuedMessage.ID,
				"DeletedQueuedMessage returns the targeted queued message")
			require.Equal(t, seeded.chatID, result.deleteQueuedMessage.DeletedQueuedMessage.ChatID)
			requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, targetID)

			// Remaining queue IDs are the baseline tail.
			wantRemaining := append([]int64{}, base.queueIDs[1:]...)
			require.Equal(t, wantRemaining, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"DeleteQueuedMessage preserves remaining queue order")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"DeleteQueuedMessage does not touch history")
		},
	}
	if shape.isMulti() {
		spec.scenario = scenarioMulti
		spec.seed = func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			return seedStateMultiQueued(t, f, from)
		}
	}
	return spec
}

func promoteQueuedCase(from, want chatstate.ExecutionState, shape queueShape, targetIdx int) transitionCaseSpec {
	var sc scenario
	if shape.isMulti() {
		switch targetIdx {
		case 0:
			sc = scenarioHeadTarget
		default:
			sc = scenarioNonHead
		}
	}
	apply := func(t *testing.T, _ *testFixture, tx *chatstate.Tx, seeded seededChat, _ chatstate.ExecutionState, result *transitionCaseResult) error {
		t.Helper()
		require.Less(t, targetIdx, len(seeded.queuedMessageIDs), "promote target index out of range")
		var err error
		result.promoteQueuedMessage, err = tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
			QueuedMessageID: seeded.queuedMessageIDs[targetIdx],
		})
		return err
	}
	spec := transitionCaseSpec{
		transition: chatstate.TransitionPromoteQueuedMessage,
		from:       from,
		want:       want,
		scenario:   sc,
		apply:      apply,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			afterQueueIDs := queuedIDsByPosition(ctx, t, f, seeded.chatID)
			afterQueue := int64(len(afterQueueIDs))

			require.NotEmpty(t, seeded.queuedMessageIDs)
			require.Less(t, targetIdx, len(seeded.queuedMessageIDs))
			targetID := seeded.queuedMessageIDs[targetIdx]
			require.Equal(t, targetID, result.promoteQueuedMessage.QueuedMessage.ID,
				"PromoteQueuedMessage returns the targeted queued message")

			switch from {
			case chatstate.StateE1, chatstate.StateA1:
				// Head is popped into history.
				require.Equal(t, base.queueCount-1, afterQueue,
					"E1/A1 promote pops the head into history")
				require.Equal(t, database.ChatStatusRunning, after.Status,
					"E1/A1 promote lands in running")
				require.False(t, after.LastError.Valid,
					"E1/A1 promote clears last_error")
				require.False(t, after.RequiresActionDeadlineAt.Valid,
					"E1/A1 promote clears requires_action_deadline_at")
				require.NotNil(t, result.promoteQueuedMessage.InsertedMessage,
					"E1/A1 promote inserts a user history message")
				inserted := requireChatMessageByID(ctx, t, f, result.promoteQueuedMessage.InsertedMessage.ID)
				require.Equal(t, seeded.chatID, inserted.ChatID)
				require.Equal(t, database.ChatMessageRoleUser, inserted.Role)
				require.True(t, inserted.ModelConfigID.Valid)
				require.Equal(t, f.Model.ID, inserted.ModelConfigID.UUID)
				require.Equal(t, chatprompt.CurrentContentVersion, inserted.ContentVersion)
				require.True(t, inserted.CreatedBy.Valid)
				require.Equal(t, result.promoteQueuedMessage.QueuedMessage.CreatedBy, inserted.CreatedBy.UUID,
					"promoted history message preserves queued created_by")
				if len(seeded.queuedMessageCreatedBy) > targetIdx {
					require.Equal(t, seeded.queuedMessageCreatedBy[targetIdx], inserted.CreatedBy.UUID,
						"promoted history message preserves non-owner queued creator")
				}
				require.NotEmpty(t, seeded.queuedMessageBodies,
					"E1/A1 seed must record queued message bodies")
				assertChatMessageText(t, inserted, seeded.queuedMessageBodies[targetIdx])
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, targetID)
				wantRemaining := remainingExcluding(base.queueIDs, targetIdx)
				require.Equal(t, wantRemaining, afterQueueIDs,
					"E1/A1 promote leaves the remaining queue order intact")
				assertQueueBodiesInOrder(ctx, t, f, seeded.chatID,
					remainingBodiesExcluding(seeded.queuedMessageBodies, targetIdx))
				// New active history adds exactly the inserted
				// user message plus any synthetic cancellations.
				newIDs := newActiveMessageIDs(base, afterHistory)
				require.Contains(t, newIDs, inserted.ID,
					"newly-active history contains the promoted user message")
				if from == chatstate.StateA1 {
					// A1: every outstanding tool call must be
					// canceled before the promoted user message.
					require.Len(t, result.promoteQueuedMessage.CancellationMessages, len(seeded.pendingToolCallIDs),
						"A1 promote synthesizes one tool-result cancellation per outstanding call")
					gotIDs := make(map[string]bool)
					for _, cancelMsg := range result.promoteQueuedMessage.CancellationMessages {
						cancel := requireChatMessageByID(ctx, t, f, cancelMsg.ID)
						require.Less(t, cancel.ID, inserted.ID,
							"A1 promote inserts cancellations before the promoted user message")
						parts, err := chatprompt.ParseContent(cancel)
						require.NoError(t, err)
						for _, part := range parts {
							if part.Type != codersdk.ChatMessagePartTypeToolResult {
								continue
							}
							require.True(t, part.IsError,
								"A1 promote synthetic cancellation is marked as an error")
							gotIDs[part.ToolCallID] = true
						}
					}
					for _, callID := range seeded.pendingToolCallIDs {
						require.True(t, gotIDs[callID],
							"A1 promote cancels outstanding tool call %s", callID)
					}
				} else {
					require.Empty(t, result.promoteQueuedMessage.CancellationMessages,
						"E1 promote has no synthetic cancellations")
				}
			case chatstate.StateR1, chatstate.StateI1:
				// Reorder-only: status flips to interrupting, no
				// history insert, queue cardinality unchanged.
				require.Equal(t, base.queueCount, afterQueue,
					"R1/I1 promote leaves queue cardinality unchanged")
				require.Equal(t, database.ChatStatusInterrupting, after.Status,
					"R1/I1 promote lands in interrupting")
				require.Nil(t, result.promoteQueuedMessage.InsertedMessage,
					"R1/I1 promote must not insert a history message")
				require.Empty(t, result.promoteQueuedMessage.CancellationMessages,
					"R1/I1 promote has no synthetic cancellations")
				require.Equal(t, base.historyIDs, afterHistory,
					"R1/I1 promote leaves history unchanged")
				// Target must still be present and now at the head.
				queued := requireQueuedMessageByID(ctx, t, f, seeded.chatID, targetID)
				require.Equal(t, targetID, queued.ID)
				require.NotEmpty(t, afterQueueIDs)
				require.Equal(t, targetID, afterQueueIDs[0],
					"R1/I1 promote brings the target to the queue head")
				require.NotEmpty(t, seeded.queuedMessageBodies,
					"R1/I1 seed must record queued message bodies")
				if targetIdx == 0 {
					// Head-target: zero rows updated, so the
					// queue order is unchanged and queue_version
					// stays put.
					require.Equal(t, base.queueIDs, afterQueueIDs,
						"head-target promote preserves queue order")
					require.Equal(t, base.queueVersion, after.QueueVersion,
						"head-target promote leaves queue_version unchanged")
					assertQueueBodiesInOrder(ctx, t, f, seeded.chatID, seeded.queuedMessageBodies)
				} else {
					// Non-head: target moves to the head, the rest
					// of the original order is preserved.
					wantQueue := append([]int64{targetID}, remainingExcluding(base.queueIDs, targetIdx)...)
					require.Equal(t, wantQueue, afterQueueIDs,
						"non-head promote moves the target to the head and preserves the rest")
					require.Greater(t, after.QueueVersion, base.queueVersion,
						"non-head promote advances queue_version")
					wantBodies := append([]string{seeded.queuedMessageBodies[targetIdx]},
						remainingBodiesExcluding(seeded.queuedMessageBodies, targetIdx)...)
					assertQueueBodiesInOrder(ctx, t, f, seeded.chatID, wantBodies)
				}
			}
		},
	}
	if from == chatstate.StateA1 {
		spec.seed = func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			queuedExtras := 1
			if shape.isMulti() {
				queuedExtras = 2
			}
			return seedA1WithMixedOutstandingToolCalls(t, f, queuedExtras, "seed_tool_a1_promote")
		}
	} else if shape.isMulti() {
		spec.seed = func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			return seedStateMultiQueued(t, f, from)
		}
	}
	return spec
}

func interruptCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionInterrupt,
		from:       from,
		want:       want,
		apply:      applyInterrupt,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			afterQueueIDs := queuedIDsByPosition(ctx, t, f, seeded.chatID)
			require.Equal(t, base.queueIDs, afterQueueIDs,
				"Interrupt does not touch queued messages")

			switch from {
			case chatstate.StateR0, chatstate.StateR1:
				require.Equal(t, database.ChatStatusInterrupting, after.Status,
					"Interrupt from R* sets status interrupting")
				require.Equal(t, base.historyIDs, afterHistory,
					"Interrupt from R* leaves history unchanged")
				require.Empty(t, result.interrupt.CancellationMessages,
					"Interrupt from R* does not synthesize tool cancellations")
			case chatstate.StateA0, chatstate.StateA1:
				require.Equal(t, database.ChatStatusRunning, after.Status,
					"Interrupt from A* cancels pending dynamic calls and resumes running")
				require.False(t, after.RequiresActionDeadlineAt.Valid,
					"Interrupt from A* clears requires_action_deadline_at")
				require.Len(t, result.interrupt.CancellationMessages, 1,
					"Interrupt from A* synthesizes one tool-result cancellation")
				cancel := requireChatMessageByID(ctx, t, f,
					result.interrupt.CancellationMessages[0].ID)
				assertToolResultForCall(t, cancel, seeded.pendingToolCallID)
			}
		},
	}
}

func completeRequiresActionCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	// Re-seed A0/A1 fresh per case so the pending tool call ID is
	// available on the seeded chat.
	return transitionCaseSpec{
		transition: chatstate.TransitionCompleteRequiresAction,
		from:       from,
		want:       want,
		apply:      applyCompleteRequiresAction,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusRunning, after.Status,
				"CompleteRequiresAction sets status running")
			require.False(t, after.RequiresActionDeadlineAt.Valid,
				"CompleteRequiresAction clears requires_action_deadline_at")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"CompleteRequiresAction preserves queued messages")

			// The user-submitted tool result must be inserted as a
			// tool-role message that references the seeded
			// pendingToolCallID with is_error=false.
			require.Len(t, result.completeRequiresAction.InsertedMessages, 1,
				"CompleteRequiresAction inserts one tool-result message per pending call")
			inserted := requireChatMessageByID(ctx, t, f,
				result.completeRequiresAction.InsertedMessages[0].ID)
			assertToolResultForCallNoError(t, inserted, seeded.pendingToolCallID, `{"ok":true}`)
		},
	}
}

func cancelRequiresActionCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionCancelRequiresAction,
		from:       from,
		want:       want,
		apply:      applyCancelRequiresAction,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusRunning, after.Status,
				"CancelRequiresAction sets status running")
			require.False(t, after.RequiresActionDeadlineAt.Valid,
				"CancelRequiresAction clears requires_action_deadline_at")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"CancelRequiresAction preserves queued messages")

			// One synthetic tool-result cancellation per pending call.
			require.Len(t, result.cancelRequiresAction.CancellationMessages, 1,
				"CancelRequiresAction synthesizes one tool-result per pending call")
			cancel := requireChatMessageByID(ctx, t, f,
				result.cancelRequiresAction.CancellationMessages[0].ID)
			assertToolResultForCall(t, cancel, seeded.pendingToolCallID)
		},
	}
}

func recordGenerationAttemptCase(from chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionRecordGenerationAttempt,
		from:       from,
		want:       from, // state preserved
		apply:      applyRecordGenerationAttempt,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, int64(1), after.GenerationAttempt,
				"RecordGenerationAttempt increments generation_attempt by one")
			require.Equal(t, result.recordGenerationAttempt.GenerationAttempt, after.GenerationAttempt,
				"RecordGenerationAttempt result mirrors the persisted value")
			require.Equal(t, base.historyVersion, after.HistoryVersion,
				"RecordGenerationAttempt does not change history_version")
			require.Equal(t, base.queueVersion, after.QueueVersion,
				"RecordGenerationAttempt does not change queue_version")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"RecordGenerationAttempt does not change queue order")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"RecordGenerationAttempt does not change history messages")
		},
	}
}

func recordRetryStateCase(from chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionRecordRetryState,
		from:       from,
		want:       from, // state preserved
		apply:      applyRecordRetryState,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.True(t, after.RetryState.Valid,
				"RecordRetryState stores retry_state")
			require.JSONEq(t,
				string(result.recordRetryState.Chat.RetryState.RawMessage),
				string(after.RetryState.RawMessage),
				"RecordRetryState result mirrors persisted retry_state")
			require.JSONEq(t,
				`{"attempt":1,"delay_ms":250,"error":"retry","retrying_at":"2026-05-29T00:00:00Z"}`,
				string(after.RetryState.RawMessage),
				"RecordRetryState stores the expected payload")
			require.Equal(t, after.SnapshotVersion, after.RetryStateVersion,
				"RecordRetryState sets retry_state_version to snapshot_version")
			require.Greater(t, after.RetryStateVersion, base.retryStateVersion,
				"RecordRetryState advances retry_state_version")
			require.Equal(t, base.historyVersion, after.HistoryVersion,
				"RecordRetryState does not change history_version")
			require.Equal(t, base.queueVersion, after.QueueVersion,
				"RecordRetryState does not change queue_version")
			require.Equal(t, base.generationAttempt, after.GenerationAttempt,
				"RecordRetryState does not change generation_attempt")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"RecordRetryState does not change queue order")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"RecordRetryState does not change history messages")
		},
	}
}

func commitStepCase(from chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionCommitStep,
		from:       from,
		want:       from, // state preserved
		apply:      applyCommitStep,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			require.Equal(t, len(base.historyIDs)+1, len(afterHistory),
				"CommitStep appends exactly one history message")
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Greater(t, after.HistoryVersion, base.historyVersion,
				"CommitStep advances history_version")

			require.Len(t, result.commitStep.InsertedMessages, 1,
				"CommitStep returns the inserted assistant message")
			inserted := requireChatMessageByID(ctx, t, f,
				result.commitStep.InsertedMessages[0].ID)
			require.Equal(t, seeded.chatID, inserted.ChatID)
			require.Equal(t, database.ChatMessageRoleAssistant, inserted.Role,
				"CommitStep inserts an assistant-role message")
			assertChatMessageText(t, inserted, "assistant")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"CommitStep does not change queue order")
			require.Equal(t, base.queueVersion, after.QueueVersion,
				"CommitStep does not change queue_version")
		},
	}
}

func enterRequiresActionCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionEnterRequiresAction,
		from:       from,
		want:       want,
		seed:       seedForEnterRequiresAction,
		apply:      applyEnterRequiresAction,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusRequiresAction, after.Status,
				"EnterRequiresAction sets status requires_action")
			require.True(t, after.RequiresActionDeadlineAt.Valid,
				"EnterRequiresAction populates requires_action_deadline_at")
			require.True(t, result.enterRequiresAction.RequiresActionDeadlineAt.Valid,
				"EnterRequiresAction returns the deadline")
			require.Equal(t, result.enterRequiresAction.RequiresActionDeadlineAt, after.RequiresActionDeadlineAt,
				"EnterRequiresAction returned deadline matches the persisted value")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"EnterRequiresAction preserves queued messages")
			require.Equal(t, base.queueVersion, after.QueueVersion,
				"EnterRequiresAction does not bump queue_version")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"EnterRequiresAction does not insert history")
		},
	}
}

func finishInterruptionRejectsOutstandingToolCallCase() transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionFinishInterruption,
		from:       chatstate.StateI0,
		want:       chatstate.StateI0,
		scenario:   scenarioRejectNonDynamicOutstandingToolCall,
		seed: func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			ctx := testutil.Context(t, testutil.WaitShort)
			created := createTestChat(t, f)
			m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

			nonDynCallID := "call_" + uuid.NewString()
			commitAssistantToolCall(t, f, m,
				nonDynamicAssistantToolCallMessage(t, f.Model.ID, nonDynCallID))

			require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
				_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "test"})
				return err
			}))
			return seededChat{
				chatID:                 created.Chat.ID,
				exists:                 true,
				initialUserMessageID:   firstUserMessageID(ctx, t, f, created.Chat.ID),
				assistantToolCallMsgID: firstAssistantMessageID(ctx, t, f, created.Chat.ID),
				pendingToolCallIDs:     []string{nonDynCallID},
			}
		},
		apply: applyFinishInterruption,
		assertFailure: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, err error) {
			require.Error(t, err, "FinishInterruption must reject an outstanding tool call")
			require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed,
				"rejection must wrap ErrTransitionNotAllowed")
			var te *chatstate.TransitionError
			require.ErrorAs(t, err, &te,
				"FinishInterruption must return a typed TransitionError")
			require.Equal(t, chatstate.TransitionFinishInterruption, te.Transition)
			require.Equal(t, chatstate.StateI0, te.From)
			assertNoMutationOrPublish(ctx, t, f, seeded.chatID, base)
		},
	}
}

func finishInterruptionCase(from, want chatstate.ExecutionState, shape queueShape) transitionCaseSpec {
	spec := transitionCaseSpec{
		transition: chatstate.TransitionFinishInterruption,
		from:       from,
		want:       want,
		apply:      applyFinishInterruption,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			afterQueueIDs := queuedIDsByPosition(ctx, t, f, seeded.chatID)
			switch from {
			case chatstate.StateI0:
				require.Equal(t, database.ChatStatusWaiting, after.Status,
					"FinishInterruption from I0 lands in waiting")
				require.Nil(t, result.finishInterruption.PromotedMessage,
					"FinishInterruption from I0 promotes nothing")
				require.Equal(t, base.queueIDs, afterQueueIDs,
					"FinishInterruption from I0 leaves queued messages unchanged")
				require.Equal(t, base.historyIDs, afterHistory,
					"FinishInterruption from I0 with no partial messages leaves history unchanged")
			case chatstate.StateI1:
				require.Equal(t, database.ChatStatusRunning, after.Status,
					"FinishInterruption from I1 lands in running")
				require.NotNil(t, result.finishInterruption.PromotedMessage,
					"FinishInterruption from I1 promotes the head into history")
				promoted := assertFetchedUserMessage(ctx, t, f,
					*result.finishInterruption.PromotedMessage)
				require.Equal(t, seeded.chatID, promoted.ChatID)
				require.Contains(t, newActiveMessageIDs(base, afterHistory), promoted.ID,
					"FinishInterruption from I1 inserts the promoted user message")
				require.NotEmpty(t, seeded.queuedMessageBodies,
					"I1 seed must record queued message bodies")
				assertChatMessageText(t, promoted, seeded.queuedMessageBodies[0])
				require.NotEmpty(t, base.queueIDs)
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, base.queueIDs[0])
				wantRemaining := append([]int64{}, base.queueIDs[1:]...)
				require.Equal(t, wantRemaining, afterQueueIDs,
					"FinishInterruption from I1 preserves the queue tail order")
				assertQueueBodiesInOrder(ctx, t, f, seeded.chatID,
					seeded.queuedMessageBodies[1:])
			}
		},
	}
	if shape.isMulti() {
		spec.scenario = scenarioMulti
		spec.seed = func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			return seedStateMultiQueued(t, f, from)
		}
	}
	return spec
}

func finishTurnCase(from, want chatstate.ExecutionState, shape queueShape) transitionCaseSpec {
	spec := transitionCaseSpec{
		transition: chatstate.TransitionFinishTurn,
		from:       from,
		want:       want,
		apply:      applyFinishTurn,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			afterHistory := activeHistoryIDs(ctx, t, f, seeded.chatID)
			afterQueueIDs := queuedIDsByPosition(ctx, t, f, seeded.chatID)
			switch from {
			case chatstate.StateR0:
				require.Equal(t, database.ChatStatusWaiting, after.Status,
					"FinishTurn from R0 lands in waiting")
				require.Nil(t, result.finishTurn.PromotedMessage,
					"FinishTurn from R0 promotes nothing")
				require.Equal(t, base.queueIDs, afterQueueIDs,
					"FinishTurn from R0 leaves queued messages unchanged")
				require.Equal(t, base.historyIDs, afterHistory,
					"FinishTurn from R0 leaves history unchanged")
			case chatstate.StateR1:
				require.Equal(t, database.ChatStatusRunning, after.Status,
					"FinishTurn from R1 lands in running")
				require.NotNil(t, result.finishTurn.PromotedMessage,
					"FinishTurn from R1 promotes the head into history")
				promoted := assertFetchedUserMessage(ctx, t, f,
					*result.finishTurn.PromotedMessage)
				require.Equal(t, seeded.chatID, promoted.ChatID)
				require.Contains(t, newActiveMessageIDs(base, afterHistory), promoted.ID,
					"FinishTurn from R1 inserts the promoted user message")
				require.NotEmpty(t, seeded.queuedMessageBodies,
					"R1 seed must record queued message bodies")
				assertChatMessageText(t, promoted, seeded.queuedMessageBodies[0])
				require.NotEmpty(t, base.queueIDs)
				requireQueuedMessageDeleted(ctx, t, f, seeded.chatID, base.queueIDs[0])
				wantRemaining := append([]int64{}, base.queueIDs[1:]...)
				require.Equal(t, wantRemaining, afterQueueIDs,
					"FinishTurn from R1 preserves the queue tail order")
				assertQueueBodiesInOrder(ctx, t, f, seeded.chatID,
					seeded.queuedMessageBodies[1:])
			}
		},
	}
	if shape.isMulti() {
		spec.scenario = scenarioMulti
		spec.seed = func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			return seedStateMultiQueued(t, f, from)
		}
	}
	return spec
}

func finishErrorCase(from, want chatstate.ExecutionState) transitionCaseSpec {
	return transitionCaseSpec{
		transition: chatstate.TransitionFinishError,
		from:       from,
		want:       want,
		apply:      applyFinishError,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			_ = result
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusError, after.Status,
				"FinishError sets status error")
			require.True(t, after.LastError.Valid,
				"FinishError stores last_error")
			require.JSONEq(t, `{"message":"finish-error"}`, string(after.LastError.RawMessage),
				"FinishError persists the input last_error JSON")
			require.Equal(t, base.historyVersion, after.HistoryVersion,
				"FinishError does not change history_version")
			require.Equal(t, base.queueVersion, after.QueueVersion,
				"FinishError does not change queue_version")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"FinishError preserves queued messages")
			require.Equal(t, base.historyIDs, activeHistoryIDs(ctx, t, f, seeded.chatID),
				"FinishError preserves history messages")
		},
	}
}

func reconcileInvalidStateCase(want chatstate.ExecutionState, shape queueShape) transitionCaseSpec {
	spec := transitionCaseSpec{
		transition: chatstate.TransitionReconcileInvalidState,
		from:       chatstate.StateInvalid,
		want:       want,
		apply:      applyReconcileInvalidState,
		assert: func(ctx context.Context, t *testing.T, f *testFixture, seeded seededChat, base snapshotBaseline, result transitionCaseResult) {
			after, err := f.DB.GetChatByID(ctx, seeded.chatID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusError, after.Status,
				"ReconcileInvalidState lands in error")
			require.False(t, after.Archived,
				"ReconcileInvalidState clears archived")
			require.True(t, after.LastError.Valid,
				"ReconcileInvalidState sets a default last_error")
			require.Equal(t, base.queueIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID),
				"ReconcileInvalidState preserves queued messages")
			// For the current invalid seeds there are no pending
			// dynamic tool calls, so no cancellation messages are
			// expected. Still, if any are returned we fetch them
			// to verify they were persisted as tool-role messages.
			for _, c := range result.reconcileInvalidState.CancellationMessages {
				msg := requireChatMessageByID(ctx, t, f, c.ID)
				require.Equal(t, database.ChatMessageRoleTool, msg.Role)
			}
		},
	}
	if shape.isMulti() {
		spec.scenario = scenarioWithQueue
		spec.seed = func(t *testing.T, f *testFixture, _ chatstate.ExecutionState) seededChat {
			return seedInvalidWithQueue(t, f)
		}
	}
	return spec
}
