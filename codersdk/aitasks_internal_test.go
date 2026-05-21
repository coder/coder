package codersdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_splitTaskIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		identifier    string
		expectedOwner string
		expectedTask  string
		expectErr     bool
	}{
		{
			name:          "bare task name",
			identifier:    "mytask",
			expectedOwner: Me,
			expectedTask:  "mytask",
			expectErr:     false,
		},
		{
			name:          "owner/task format",
			identifier:    "alice/her-task",
			expectedOwner: "alice",
			expectedTask:  "her-task",
			expectErr:     false,
		},
		{
			name:          "uuid/task format",
			identifier:    "550e8400-e29b-41d4-a716-446655440000/task1",
			expectedOwner: "550e8400-e29b-41d4-a716-446655440000",
			expectedTask:  "task1",
			expectErr:     false,
		},
		{
			name:          "owner/uuid format",
			identifier:    "alice/3abe1dcf-cd87-4078-8b54-c0e2058ad2e2",
			expectedOwner: "alice",
			expectedTask:  "3abe1dcf-cd87-4078-8b54-c0e2058ad2e2",
			expectErr:     false,
		},
		{
			name:       "too many slashes",
			identifier: "owner/task/extra",
			expectErr:  true,
		},
		{
			name:          "empty parts acceptable",
			identifier:    "/task",
			expectedOwner: "",
			expectedTask:  "task",
			expectErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			owner, taskName, err := splitTaskIdentifier(tt.identifier)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedOwner, owner)
				assert.Equal(t, tt.expectedTask, taskName)
			}
		})
	}
}
