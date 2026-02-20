package agentapi

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

func TestShouldBump(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prevState  *database.WorkspaceAppStatusState // nil means no previous state
		newState   database.WorkspaceAppStatusState
		shouldBump bool
	}{
		{
			name:       "FirstStatusBumps",
			prevState:  nil,
			newState:   database.WorkspaceAppStatusStateWorking,
			shouldBump: true,
		},
		{
			name:       "WorkingToIdleBumps",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateWorking),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: true,
		},
		{
			name:       "WorkingToCompleteBumps",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateWorking),
			newState:   database.WorkspaceAppStatusStateComplete,
			shouldBump: true,
		},
		{
			name:       "CompleteToIdleNoBump",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateComplete),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: false,
		},
		{
			name:       "CompleteToCompleteNoBump",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateComplete),
			newState:   database.WorkspaceAppStatusStateComplete,
			shouldBump: false,
		},
		{
			name:       "FailureToIdleNoBump",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateFailure),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: false,
		},
		{
			name:       "FailureToFailureNoBump",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateFailure),
			newState:   database.WorkspaceAppStatusStateFailure,
			shouldBump: false,
		},
		{
			name:       "CompleteToWorkingBumps",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateComplete),
			newState:   database.WorkspaceAppStatusStateWorking,
			shouldBump: true,
		},
		{
			name:       "FailureToCompleteNoBump",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateFailure),
			newState:   database.WorkspaceAppStatusStateComplete,
			shouldBump: false,
		},
		{
			name:       "WorkingToFailureBumps",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateWorking),
			newState:   database.WorkspaceAppStatusStateFailure,
			shouldBump: true,
		},
		{
			name:       "IdleToIdleNoBump",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateIdle),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: false,
		},
		{
			name:       "IdleToWorkingBumps",
			prevState:  ptr.Ref(database.WorkspaceAppStatusStateIdle),
			newState:   database.WorkspaceAppStatusStateWorking,
			shouldBump: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var prevAppStatus database.WorkspaceAppStatus
			// If there's a previous state, report it first.
			if tt.prevState != nil {
				prevAppStatus.ID = uuid.UUID{1}
				prevAppStatus.State = *tt.prevState
			}

			didBump := shouldBump(tt.newState, prevAppStatus)
			if tt.shouldBump {
				require.True(t, didBump, "wanted deadline to bump but it didn't")
			} else {
				require.False(t, didBump, "wanted deadline not to bump but it did")
			}
		})
	}
}
