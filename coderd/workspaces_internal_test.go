package coderd

import (
	"testing"
	"time"

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
