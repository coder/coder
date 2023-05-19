package coderd

import (
	"fmt"
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
	t.Parallel()

	t.Run("Running First", func(t *testing.T) {
		t.Parallel()
		workspaces := []codersdk.Workspace{
			WorkspaceFactory(t, "test-workspace-sort-1", uuid.New(), "user1", codersdk.WorkspaceStatusPending),
			WorkspaceFactory(t, "test-workspace-sort-2", uuid.New(), "user2", codersdk.WorkspaceStatusRunning),
			WorkspaceFactory(t, "test-workspace-sort-3", uuid.New(), "user2", codersdk.WorkspaceStatusRunning),
			WorkspaceFactory(t, "test-workspace-sort-4", uuid.New(), "user1", codersdk.WorkspaceStatusPending),
		}

		sortWorkspaces(workspaces)

		require.Equal(t, workspaces[0].LatestBuild.Status, codersdk.WorkspaceStatusRunning)
		require.Equal(t, workspaces[1].LatestBuild.Status, codersdk.WorkspaceStatusRunning)
		require.Equal(t, workspaces[2].LatestBuild.Status, codersdk.WorkspaceStatusPending)
		require.Equal(t, workspaces[3].LatestBuild.Status, codersdk.WorkspaceStatusPending)
	})

	t.Run("Then sort by owner Name", func(t *testing.T) {
		t.Parallel()
		workspaces := []codersdk.Workspace{
			WorkspaceFactory(t, "test-workspace-sort-1", uuid.New(), "userZ", codersdk.WorkspaceStatusRunning),
			WorkspaceFactory(t, "test-workspace-sort-2", uuid.New(), "userA", codersdk.WorkspaceStatusRunning),
			WorkspaceFactory(t, "test-workspace-sort-3", uuid.New(), "userB", codersdk.WorkspaceStatusRunning),
		}

		sortWorkspaces(workspaces)

		t.Log("uuid ", uuid.New().String())
		t.Log("uuid ", uuid.New().String())

		require.Equal(t, "userA", workspaces[0].OwnerName)
		require.Equal(t, "userB", workspaces[1].OwnerName)
		require.Equal(t, "userZ", workspaces[2].OwnerName)
	})

	t.Run("Then sort by last used at (recent first)", func(t *testing.T) {
		t.Parallel()
		var workspaces []codersdk.Workspace

		useruuid := uuid.New()

		for i := 0; i < 4; i++ {
			workspaces = append(workspaces, WorkspaceFactory(t, fmt.Sprintf("test-workspace-sort-%d", i+1), useruuid, "user2", codersdk.WorkspaceStatusRunning))
		}

		sortWorkspaces(workspaces)

		// in this case, the last used at is the creation time
		require.Equal(t, workspaces[0].Name, "test-workspace-sort-4")
		require.Equal(t, workspaces[1].Name, "test-workspace-sort-3")
		require.Equal(t, workspaces[2].Name, "test-workspace-sort-2")
		require.Equal(t, workspaces[3].Name, "test-workspace-sort-1")
	})
}

func WorkspaceFactory(t *testing.T, name string, ownerID uuid.UUID, ownerName string, status codersdk.WorkspaceStatus) codersdk.Workspace {
	t.Helper()
	return codersdk.Workspace{
		ID:                                   uuid.New(),
		CreatedAt:                            time.Time{},
		UpdatedAt:                            time.Time{},
		OwnerID:                              ownerID,
		OwnerName:                            ownerName,
		OrganizationID:                       [16]byte{},
		TemplateID:                           [16]byte{},
		TemplateName:                         name,
		TemplateDisplayName:                  name,
		TemplateIcon:                         "",
		TemplateAllowUserCancelWorkspaceJobs: false,
		LatestBuild: codersdk.WorkspaceBuild{
			ID:                  uuid.New(),
			CreatedAt:           time.Time{},
			UpdatedAt:           time.Time{},
			WorkspaceID:         [16]byte{},
			WorkspaceName:       name,
			WorkspaceOwnerID:    [16]byte{},
			WorkspaceOwnerName:  ownerName,
			TemplateVersionID:   [16]byte{},
			TemplateVersionName: name,
			BuildNumber:         0,
			Transition:          "",
			InitiatorID:         [16]byte{},
			InitiatorUsername:   name,
			Job:                 codersdk.ProvisionerJob{},
			Reason:              "",
			Resources:           []codersdk.WorkspaceResource{},
			Deadline:            codersdk.NullTime{},
			MaxDeadline:         codersdk.NullTime{},
			Status:              status,
			DailyCost:           0,
		},
		Outdated:          false,
		Name:              name,
		AutostartSchedule: new(string),
		TTLMillis:         new(int64),
		LastUsedAt:        time.Now(),
		DeletingAt:        &time.Time{},
	}
}
