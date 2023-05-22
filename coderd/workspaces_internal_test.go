package coderd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func Test_calculateDeletingAt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		workspace database.Workspace
		template  database.Template
		build     codersdk.WorkspaceBuild
		expected  *time.Time
	}{
		{
			name: "InactiveWorkspace",
			workspace: database.Workspace{
				Deleted:    false,
				LastUsedAt: time.Now().Add(time.Duration(-10) * time.Hour * 24), // 10 days ago
			},
			template: database.Template{
				InactivityTTL: int64(9 * 24 * time.Hour), // 9 days
			},
			build: codersdk.WorkspaceBuild{
				Status: codersdk.WorkspaceStatusStopped,
			},
			expected: ptr.Ref(time.Now().Add(time.Duration(-1) * time.Hour * 24)), // yesterday
		},
		{
			name: "InactivityTTLUnset",
			workspace: database.Workspace{
				Deleted:    false,
				LastUsedAt: time.Now().Add(time.Duration(-10) * time.Hour * 24),
			},
			template: database.Template{
				InactivityTTL: 0,
			},
			build: codersdk.WorkspaceBuild{
				Status: codersdk.WorkspaceStatusStopped,
			},
			expected: nil,
		},
		{
			name: "ActiveWorkspace",
			workspace: database.Workspace{
				Deleted:    false,
				LastUsedAt: time.Now(),
			},
			template: database.Template{
				InactivityTTL: int64(1 * 24 * time.Hour),
			},
			build: codersdk.WorkspaceBuild{
				Status: codersdk.WorkspaceStatusRunning,
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			found := calculateDeletingAt(tc.workspace, tc.template, tc.build)
			if tc.expected == nil {
				require.Nil(t, found, "impending deletion should be nil")
			} else {
				require.NotNil(t, found)
				require.WithinDuration(t, *tc.expected, *found, time.Second, "incorrect impending deletion")
			}
		})
	}
}

func TestSortWorkspaces(t *testing.T) {
	// the correct sorting order is:
	// 1. first show workspaces that are currently running,
	// 2. then sort by user_name,
	// 3. then sort by last_used_at (descending),
	t.Parallel()

	workspaceFactory := func(t *testing.T, name string, ownerID uuid.UUID, ownerName string, status codersdk.WorkspaceStatus, lastUsedAt time.Time) codersdk.Workspace {
		t.Helper()
		return codersdk.Workspace{
			ID:        uuid.New(),
			OwnerID:   ownerID,
			OwnerName: ownerName,
			LatestBuild: codersdk.WorkspaceBuild{
				Status: status,
			},
			Name:       name,
			LastUsedAt: lastUsedAt,
		}
	}

	userAuuid := uuid.New()

	workspaceRunningUserA := workspaceFactory(t, "running-userA", userAuuid, "userA", codersdk.WorkspaceStatusRunning, time.Now())
	workspaceRunningUserB := workspaceFactory(t, "running-userB", uuid.New(), "userB", codersdk.WorkspaceStatusRunning, time.Now())
	workspacePendingUserC := workspaceFactory(t, "pending-userC", uuid.New(), "userC", codersdk.WorkspaceStatusPending, time.Now())
	workspaceRunningUserA2 := workspaceFactory(t, "running-userA2", userAuuid, "userA", codersdk.WorkspaceStatusRunning, time.Now().Add(time.Minute))
	workspaceRunningUserZ := workspaceFactory(t, "running-userZ", uuid.New(), "userZ", codersdk.WorkspaceStatusRunning, time.Now())
	workspaceRunningUserA3 := workspaceFactory(t, "running-userA3", userAuuid, "userA", codersdk.WorkspaceStatusRunning, time.Now().Add(time.Hour))

	testCases := []struct {
		name          string
		input         []codersdk.Workspace
		expectedOrder []string
	}{
		{
			name: "Running workspaces should be first",
			input: []codersdk.Workspace{
				workspaceRunningUserB,
				workspacePendingUserC,
				workspaceRunningUserA,
			},
			expectedOrder: []string{
				"running-userA",
				"running-userB",
				"pending-userC",
			},
		},
		{
			name: "then sort by owner name",
			input: []codersdk.Workspace{
				workspaceRunningUserZ,
				workspaceRunningUserA,
				workspaceRunningUserB,
			},
			expectedOrder: []string{
				"running-userA",
				"running-userB",
				"running-userZ",
			},
		},
		{
			name: "then sort by last used at (recent first)",
			input: []codersdk.Workspace{
				workspaceRunningUserA,
				workspaceRunningUserA2,
				workspaceRunningUserA3,
			},
			expectedOrder: []string{
				"running-userA3",
				"running-userA2",
				"running-userA",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			workspaces := tc.input
			sortWorkspaces(workspaces)

			var resultNames []string
			for _, workspace := range workspaces {
				resultNames = append(resultNames, workspace.Name)
			}

			require.Equal(t, tc.expectedOrder, resultNames, tc.name)
		})
	}
}
