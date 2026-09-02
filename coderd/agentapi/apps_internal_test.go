package agentapi

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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
			prevState:  new(database.WorkspaceAppStatusStateWorking),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: true,
		},
		{
			name:       "WorkingToCompleteBumps",
			prevState:  new(database.WorkspaceAppStatusStateWorking),
			newState:   database.WorkspaceAppStatusStateComplete,
			shouldBump: true,
		},
		{
			name:       "CompleteToIdleNoBump",
			prevState:  new(database.WorkspaceAppStatusStateComplete),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: false,
		},
		{
			name:       "CompleteToCompleteNoBump",
			prevState:  new(database.WorkspaceAppStatusStateComplete),
			newState:   database.WorkspaceAppStatusStateComplete,
			shouldBump: false,
		},
		{
			name:       "FailureToIdleNoBump",
			prevState:  new(database.WorkspaceAppStatusStateFailure),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: false,
		},
		{
			name:       "FailureToFailureNoBump",
			prevState:  new(database.WorkspaceAppStatusStateFailure),
			newState:   database.WorkspaceAppStatusStateFailure,
			shouldBump: false,
		},
		{
			name:       "CompleteToWorkingBumps",
			prevState:  new(database.WorkspaceAppStatusStateComplete),
			newState:   database.WorkspaceAppStatusStateWorking,
			shouldBump: true,
		},
		{
			name:       "FailureToCompleteNoBump",
			prevState:  new(database.WorkspaceAppStatusStateFailure),
			newState:   database.WorkspaceAppStatusStateComplete,
			shouldBump: false,
		},
		{
			name:       "WorkingToFailureBumps",
			prevState:  new(database.WorkspaceAppStatusStateWorking),
			newState:   database.WorkspaceAppStatusStateFailure,
			shouldBump: true,
		},
		{
			name:       "IdleToIdleNoBump",
			prevState:  new(database.WorkspaceAppStatusStateIdle),
			newState:   database.WorkspaceAppStatusStateIdle,
			shouldBump: false,
		},
		{
			name:       "IdleToWorkingBumps",
			prevState:  new(database.WorkspaceAppStatusStateIdle),
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

func TestIsDuplicateAppStatus(t *testing.T) {
	t.Parallel()

	makeStatus := func(state database.WorkspaceAppStatusState, message, uri string) database.WorkspaceAppStatus {
		return database.WorkspaceAppStatus{
			ID:      uuid.UUID{1},
			State:   state,
			Message: message,
			Uri:     sql.NullString{String: uri, Valid: uri != ""},
		}
	}

	tests := []struct {
		name    string
		latest  database.WorkspaceAppStatus
		state   database.WorkspaceAppStatusState
		message string
		uri     string
		want    bool
	}{
		{
			name:    "NoPreviousStatus",
			latest:  database.WorkspaceAppStatus{},
			state:   database.WorkspaceAppStatusStateComplete,
			message: "testing",
			uri:     "https://example.com",
			want:    false,
		},
		{
			name:    "Identical",
			latest:  makeStatus(database.WorkspaceAppStatusStateComplete, "testing", "https://example.com"),
			state:   database.WorkspaceAppStatusStateComplete,
			message: "testing",
			uri:     "https://example.com",
			want:    true,
		},
		{
			name:    "IdenticalEmptyURI",
			latest:  makeStatus(database.WorkspaceAppStatusStateIdle, "", ""),
			state:   database.WorkspaceAppStatusStateIdle,
			message: "",
			uri:     "",
			want:    true,
		},
		{
			name:    "DifferentState",
			latest:  makeStatus(database.WorkspaceAppStatusStateWorking, "testing", "https://example.com"),
			state:   database.WorkspaceAppStatusStateComplete,
			message: "testing",
			uri:     "https://example.com",
			want:    false,
		},
		{
			name:    "DifferentMessage",
			latest:  makeStatus(database.WorkspaceAppStatusStateComplete, "testing", "https://example.com"),
			state:   database.WorkspaceAppStatusStateComplete,
			message: "something else",
			uri:     "https://example.com",
			want:    false,
		},
		{
			name:    "DifferentURI",
			latest:  makeStatus(database.WorkspaceAppStatusStateComplete, "testing", "https://example.com"),
			state:   database.WorkspaceAppStatusStateComplete,
			message: "testing",
			uri:     "https://other.example.com",
			want:    false,
		},
		{
			name:    "URICleared",
			latest:  makeStatus(database.WorkspaceAppStatusStateComplete, "testing", "https://example.com"),
			state:   database.WorkspaceAppStatusStateComplete,
			message: "testing",
			uri:     "",
			want:    false,
		},
		{
			name:    "URISet",
			latest:  makeStatus(database.WorkspaceAppStatusStateComplete, "testing", ""),
			state:   database.WorkspaceAppStatusStateComplete,
			message: "testing",
			uri:     "https://example.com",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isDuplicateAppStatus(tt.latest, tt.state, tt.message, tt.uri)
			require.Equal(t, tt.want, got)
		})
	}
}
