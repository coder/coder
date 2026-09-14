package chatstate

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func chatWithStatus(status database.ChatStatus, archived bool) database.Chat {
	return database.Chat{
		ID:       uuid.New(),
		Status:   status,
		Archived: archived,
		OwnerID:  uuid.New(),
	}
}

// TestClassifyExecutionState_Valid covers every valid classification:
// N (missing chat) plus every valid existing-chat state.
func TestClassifyExecutionState_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		status        database.ChatStatus
		archived      bool
		queueNonEmpty bool
		paused        bool
		exists        bool
		want          ExecutionState
	}{
		{name: "N", exists: false, want: StateN},
		{name: "W", status: database.ChatStatusWaiting, exists: true, want: StateW},
		{name: "E0", status: database.ChatStatusError, exists: true, want: StateE0},
		{name: "E1", status: database.ChatStatusError, queueNonEmpty: true, exists: true, want: StateE1},
		{name: "R0", status: database.ChatStatusRunning, exists: true, want: StateR0},
		{name: "R1", status: database.ChatStatusRunning, queueNonEmpty: true, exists: true, want: StateR1},
		{name: "I0", status: database.ChatStatusInterrupting, exists: true, want: StateI0},
		{name: "I1", status: database.ChatStatusInterrupting, queueNonEmpty: true, exists: true, want: StateI1},
		{name: "A0", status: database.ChatStatusRequiresAction, exists: true, want: StateA0},
		{name: "A1", status: database.ChatStatusRequiresAction, queueNonEmpty: true, exists: true, want: StateA1},
		{name: "XW", status: database.ChatStatusWaiting, archived: true, exists: true, want: StateXW},
		{name: "XE0", status: database.ChatStatusError, archived: true, exists: true, want: StateXE0},
		{name: "XE1", status: database.ChatStatusError, archived: true, queueNonEmpty: true, exists: true, want: StateXE1},
		{name: "P", status: database.ChatStatusPaused, queueNonEmpty: true, paused: true, exists: true, want: StateP},
		// A head under edit changes nothing outside paused.
		{name: "R1Paused", status: database.ChatStatusRunning, queueNonEmpty: true, paused: true, exists: true, want: StateR1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			chat := database.Chat{}
			if tc.exists {
				chat = chatWithStatus(tc.status, tc.archived)
			}
			queue := QueueState{HasRows: tc.queueNonEmpty, Paused: tc.paused}
			require.Equal(t, tc.want, ClassifyExecutionState(chat, queue, tc.exists))
		})
	}
}

// TestClassifyExecutionState_Invalid covers every documented invalid
// combination: legacy statuses, waiting-with-queue, and archived busy
// statuses.
func TestClassifyExecutionState_Invalid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		status        database.ChatStatus
		archived      bool
		queueNonEmpty bool
		paused        bool
	}{
		// Legacy statuses (pending/completed) are invalid.
		{name: "LegacyPending", status: "pending"},
		{name: "LegacyCompleted", status: "completed"},

		// Waiting never has rows.
		{name: "WaitingWithQueue", status: database.ChatStatusWaiting, queueNonEmpty: true},
		{name: "WaitingPaused", status: database.ChatStatusWaiting, queueNonEmpty: true, paused: true},
		{name: "WaitingArchivedWithQueue", status: database.ChatStatusWaiting, archived: true, queueNonEmpty: true},

		// Paused requires a pause condition and is never archived.
		{name: "PausedNoRows", status: database.ChatStatusPaused},
		{name: "PausedWithoutCondition", status: database.ChatStatusPaused, queueNonEmpty: true},
		{name: "PausedArchived", status: database.ChatStatusPaused, archived: true, queueNonEmpty: true, paused: true},

		// Archived busy statuses are invalid.
		{name: "ArchivedRunning", status: database.ChatStatusRunning, archived: true},
		{name: "ArchivedInterrupting", status: database.ChatStatusInterrupting, archived: true},
		{name: "ArchivedRequiresAction", status: database.ChatStatusRequiresAction, archived: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			queue := QueueState{HasRows: tc.queueNonEmpty, Paused: tc.paused}
			got := ClassifyExecutionState(chatWithStatus(tc.status, tc.archived), queue, true)
			require.Equal(t, StateInvalid, got)
		})
	}
}

// TestClassifyExecutionState_RejectsAllUnlistedCombinations enumerates
// every (status, archived, queue) tuple for an existing chat and
// asserts that exactly the declared valid states classify out of
// [StateInvalid]. Missing chats are handled separately via the N case
// in [TestClassifyExecutionState_Valid].
func TestClassifyExecutionState_RejectsAllUnlistedCombinations(t *testing.T) {
	t.Parallel()
	allStatuses := []database.ChatStatus{
		database.ChatStatusWaiting,
		database.ChatStatusError,
		database.ChatStatusRunning,
		database.ChatStatusInterrupting,
		database.ChatStatusRequiresAction,
		"pending", "paused", "completed",
	}
	validStates := map[ExecutionState]struct{}{}
	for _, status := range allStatuses {
		for _, archived := range []bool{false, true} {
			for _, queueNonEmpty := range []bool{false, true} {
				for _, paused := range []bool{false, true} {
					if paused && !queueNonEmpty {
						continue // no head to edit
					}
					queue := QueueState{HasRows: queueNonEmpty, Paused: paused}
					got := ClassifyExecutionState(chatWithStatus(status, archived), queue, true)
					if got != StateInvalid {
						validStates[got] = struct{}{}
					}
				}
			}
		}
	}
	wantValid := len(AllExecutionStates) - 2 // Exclude StateN and StateInvalid.
	require.Len(t, validStates, wantValid, "valid existing-chat states reachable from (status, archived, queue) tuples")
}

// TestAllExecutionStates_Enumeration verifies AllExecutionStates
// contains every declared execution state exactly once.
func TestAllExecutionStates_Enumeration(t *testing.T) {
	t.Parallel()
	want := map[ExecutionState]bool{
		StateN: true, StateW: true, StateE0: true, StateE1: true,
		StateR0: true, StateR1: true, StateI0: true, StateI1: true,
		StateA0: true, StateA1: true, StateP: true, StateXW: true,
		StateXE0: true, StateXE1: true, StateInvalid: true,
	}
	require.Len(t, AllExecutionStates, len(want))
	seen := make(map[ExecutionState]bool, len(want))
	for _, s := range AllExecutionStates {
		require.True(t, want[s], "unexpected state %s", s)
		require.False(t, seen[s], "duplicate state %s", s)
		seen[s] = true
	}
}

// TestExecutionState_IsRunnable covers IsRunnable for every declared
// execution state.
func TestExecutionState_IsRunnable(t *testing.T) {
	t.Parallel()

	runnable := map[ExecutionState]bool{
		StateR0: true, StateR1: true, StateI0: true, StateI1: true,
		StateA0: true, StateA1: true,
	}
	for _, s := range AllExecutionStates {
		require.Equal(t, runnable[s], s.IsRunnable(), "IsRunnable(%s)", s)
	}
}
