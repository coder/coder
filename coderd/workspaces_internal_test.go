package coderd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/util/ptr"
)

func Test_calculateDeletingAt(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		workspace database.Workspace
		template  database.Template
		expected  *time.Time
	}{
		{
			name: "DeletingAt",
			workspace: database.Workspace{
				Deleted:    false,
				LastUsedAt: time.Now().Add(time.Duration(-10) * time.Hour * 24), // 10 days ago
			},
			template: database.Template{
				InactivityTTL: int64(9 * 24 * time.Hour), // 9 days
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
			expected: nil,
		},
		{
			name: "DeletedWorkspace",
			workspace: database.Workspace{
				Deleted:    true,
				LastUsedAt: time.Now().Add(time.Duration(-10) * time.Hour * 24),
			},
			template: database.Template{
				InactivityTTL: int64(9 * 24 * time.Hour),
			},
			expected: nil,
		},
		{
			name: "ActiveWorkspace",
			workspace: database.Workspace{
				Deleted:    true,
				LastUsedAt: time.Now().Add(time.Duration(-5) * time.Hour), // 5 hours ago
			},
			template: database.Template{
				InactivityTTL: int64(1 * 24 * time.Hour), // 1 day
			},
			expected: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			found := calculateDeletingAt(tc.workspace, tc.template)
			if tc.expected == nil {
				require.Nil(t, found, "impending deletion should be nil")
			} else {
				require.NotNil(t, found)
				require.WithinDuration(t, *tc.expected, *found, time.Second, "incorrect impending deletion")
			}
		})
	}
}
