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
		name     string
		status   database.ChatStatus
		archived bool
		hasRows  bool
		paused   bool
		exists   bool
		want     ExecutionState
	}{
		{name: "N", exists: false, want: StateN},
		{name: "W", status: database.ChatStatusWaiting, exists: true, want: StateW},
		{name: "E0", status: database.ChatStatusError, exists: true, want: StateE0},
		{name: "E1", status: database.ChatStatusError, hasRows: true, exists: true, want: StateE1},
		{name: "E1P", status: database.ChatStatusError, hasRows: true, paused: true, exists: true, want: StateE1P},
		{name: "R0", status: database.ChatStatusRunning, exists: true, want: StateR0},
		{name: "R1", status: database.ChatStatusRunning, hasRows: true, exists: true, want: StateR1},
		{name: "R1P", status: database.ChatStatusRunning, hasRows: true, paused: true, exists: true, want: StateR1P},
		{name: "I0", status: database.ChatStatusInterrupting, exists: true, want: StateI0},
		{name: "I1", status: database.ChatStatusInterrupting, hasRows: true, exists: true, want: StateI1},
		{name: "I1P", status: database.ChatStatusInterrupting, hasRows: true, paused: true, exists: true, want: StateI1P},
		{name: "A0", status: database.ChatStatusRequiresAction, exists: true, want: StateA0},
		{name: "A1", status: database.ChatStatusRequiresAction, hasRows: true, exists: true, want: StateA1},
		{name: "A1P", status: database.ChatStatusRequiresAction, hasRows: true, paused: true, exists: true, want: StateA1P},
		{name: "XW", status: database.ChatStatusWaiting, archived: true, exists: true, want: StateXW},
		{name: "XE0", status: database.ChatStatusError, archived: true, exists: true, want: StateXE0},
		{name: "XE1", status: database.ChatStatusError, archived: true, hasRows: true, exists: true, want: StateXE1},
		{name: "XE1P", status: database.ChatStatusError, archived: true, hasRows: true, paused: true, exists: true, want: StateXE1P},
		{name: "P", status: database.ChatStatusPaused, hasRows: true, paused: true, exists: true, want: StateP},
		{name: "R0IgnoresPausedWithoutRows", status: database.ChatStatusRunning, paused: true, exists: true, want: StateR0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			chat := database.Chat{}
			if tc.exists {
				chat = chatWithStatus(tc.status, tc.archived)
			}
			queue := QueueState{HasRows: tc.hasRows, Paused: tc.paused}
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
		name     string
		status   database.ChatStatus
		archived bool
		hasRows  bool
		paused   bool
	}{
		// Legacy statuses (pending/completed) are invalid.
		{name: "LegacyPending", status: "pending"},
		{name: "LegacyCompleted", status: "completed"},

		// Waiting never has rows.
		{name: "WaitingWithQueue", status: database.ChatStatusWaiting, hasRows: true},
		{name: "WaitingPaused", status: database.ChatStatusWaiting, hasRows: true, paused: true},
		{name: "WaitingArchivedWithQueue", status: database.ChatStatusWaiting, archived: true, hasRows: true},

		// Paused requires a pause condition and is never archived.
		{name: "PausedNoRows", status: database.ChatStatusPaused},
		{name: "PausedWithoutCondition", status: database.ChatStatusPaused, hasRows: true},
		{name: "PausedArchived", status: database.ChatStatusPaused, archived: true, hasRows: true, paused: true},

		// Archived busy statuses are invalid.
		{name: "ArchivedRunning", status: database.ChatStatusRunning, archived: true},
		{name: "ArchivedInterrupting", status: database.ChatStatusInterrupting, archived: true},
		{name: "ArchivedRequiresAction", status: database.ChatStatusRequiresAction, archived: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			queue := QueueState{HasRows: tc.hasRows, Paused: tc.paused}
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
		database.ChatStatusPaused,
		"pending", "completed",
	}
	tuplesByState := map[ExecutionState]int{}
	for _, status := range allStatuses {
		for _, archived := range []bool{false, true} {
			for _, hasRows := range []bool{false, true} {
				for _, paused := range []bool{false, true} {
					chat := chatWithStatus(status, archived)
					queue := QueueState{HasRows: hasRows, Paused: paused}
					tuplesByState[ClassifyExecutionState(chat, queue, true)]++
				}
			}
		}
	}
	// Each valid state maps from one (status, archived, rows) tuple. The
	// states with rows read the pause condition, so each of them is
	// reached once; the states without rows ignore it and are reached
	// with it both set and unset.
	require.Zero(t, tuplesByState[StateN], "N requires a missing chat")
	for _, state := range AllExecutionStates {
		switch state {
		case StateN, StateInvalid:
		case StateE1, StateE1P, StateR1, StateR1P, StateI1, StateI1P, StateA1, StateA1P, StateP, StateXE1, StateXE1P:
			require.Equal(t, 1, tuplesByState[state], "%s", state)
		default:
			require.Equal(t, 2, tuplesByState[state], "%s", state)
		}
	}
}

// TestAllExecutionStates_Enumeration verifies AllExecutionStates
// contains every declared execution state exactly once.
func TestAllExecutionStates_Enumeration(t *testing.T) {
	t.Parallel()
	want := map[ExecutionState]bool{
		StateN: true, StateW: true, StateE0: true, StateE1: true, StateE1P: true,
		StateR0: true, StateR1: true, StateR1P: true, StateI0: true, StateI1: true, StateI1P: true,
		StateA0: true, StateA1: true, StateA1P: true, StateP: true, StateXW: true,
		StateXE0: true, StateXE1: true, StateXE1P: true, StateInvalid: true,
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
		StateR0: true, StateR1: true, StateR1P: true,
		StateI0: true, StateI1: true, StateI1P: true,
		StateA0: true, StateA1: true, StateA1P: true,
	}
	for _, s := range AllExecutionStates {
		require.Equal(t, runnable[s], s.IsRunnable(), "IsRunnable(%s)", s)
	}
}
