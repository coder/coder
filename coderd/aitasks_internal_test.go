package coderd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

func TestDeriveTaskCurrentState_Unit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name               string
		task               database.Task
		agentLifecycle     *codersdk.WorkspaceAgentLifecycle
		appHealth          *codersdk.WorkspaceAppHealth
		latestAppStatus    *codersdk.WorkspaceAppStatus
		latestBuild        codersdk.WorkspaceBuild
		expectCurrentState bool
		expectedTimestamp  time.Time
		expectedState      codersdk.TaskState
		expectedMessage    string
	}{
		{
			name: "NoAppStatus",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle:  nil,
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStart,
				CreatedAt:  now,
			},
			expectCurrentState: false,
		},
		{
			name: "BuildStartTransition_AppStatus_NewerThanBuild",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle: nil,
			appHealth:      nil,
			latestAppStatus: &codersdk.WorkspaceAppStatus{
				State:     codersdk.WorkspaceAppStatusStateWorking,
				Message:   "Task is working",
				CreatedAt: now.Add(1 * time.Minute),
			},
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStart,
				CreatedAt:  now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now.Add(1 * time.Minute),
			expectedState:      codersdk.TaskState(codersdk.WorkspaceAppStatusStateWorking),
			expectedMessage:    "Task is working",
		},
		{
			name: "BuildStartTransition_StaleAppStatus_OlderThanBuild",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle: nil,
			appHealth:      nil,
			latestAppStatus: &codersdk.WorkspaceAppStatus{
				State:     codersdk.WorkspaceAppStatusStateComplete,
				Message:   "Previous task completed",
				CreatedAt: now.Add(-1 * time.Minute),
			},
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStart,
				CreatedAt:  now,
			},
			expectCurrentState: false,
		},
		{
			name: "BuildStopTransition",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusActive,
			},
			agentLifecycle: nil,
			appHealth:      nil,
			latestAppStatus: &codersdk.WorkspaceAppStatus{
				State:     codersdk.WorkspaceAppStatusStateComplete,
				Message:   "Task completed before stop",
				CreatedAt: now.Add(-1 * time.Minute),
			},
			latestBuild: codersdk.WorkspaceBuild{
				Transition: codersdk.WorkspaceTransitionStop,
				CreatedAt:  now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now.Add(-1 * time.Minute),
			expectedState:      codersdk.TaskState(codersdk.WorkspaceAppStatusStateComplete),
			expectedMessage:    "Task completed before stop",
		},
		{
			name: "TaskInitializing_WorkspacePending",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  nil,
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusPending,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Workspace is pending",
		},
		{
			name: "TaskInitializing_WorkspaceStarting",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  nil,
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusStarting,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Workspace is starting",
		},
		{
			name: "TaskInitializing_AgentConnecting",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  ptr.Ref(codersdk.WorkspaceAgentLifecycleCreated),
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusRunning,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Agent is connecting",
		},
		{
			name: "TaskInitializing_AgentStarting",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  ptr.Ref(codersdk.WorkspaceAgentLifecycleStarting),
			appHealth:       nil,
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusRunning,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "Agent is starting",
		},
		{
			name: "TaskInitializing_AppInitializing",
			task: database.Task{
				ID:     uuid.New(),
				Status: database.TaskStatusInitializing,
			},
			agentLifecycle:  ptr.Ref(codersdk.WorkspaceAgentLifecycleReady),
			appHealth:       ptr.Ref(codersdk.WorkspaceAppHealthInitializing),
			latestAppStatus: nil,
			latestBuild: codersdk.WorkspaceBuild{
				Status:    codersdk.WorkspaceStatusRunning,
				CreatedAt: now,
			},
			expectCurrentState: true,
			expectedTimestamp:  now,
			expectedState:      codersdk.TaskStateWorking,
			expectedMessage:    "App is initializing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ws := codersdk.Workspace{
				LatestBuild:     tt.latestBuild,
				LatestAppStatus: tt.latestAppStatus,
			}

			currentState := deriveTaskCurrentState(tt.task, ws, tt.agentLifecycle, tt.appHealth)

			if tt.expectCurrentState {
				require.NotNil(t, currentState)
				assert.Equal(t, tt.expectedTimestamp.UTC(), currentState.Timestamp.UTC())
				assert.Equal(t, tt.expectedState, currentState.State)
				assert.Equal(t, tt.expectedMessage, currentState.Message)
			} else {
				assert.Nil(t, currentState)
			}
		})
	}
}
