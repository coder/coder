package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/codersdk"
)

func Test_PrintTaskStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in       codersdk.Task
		expected string
	}{
		// Not covering the whole gamut of possible statuses
		{
			in: codersdk.Task{
				Status: codersdk.WorkspaceStatus("frobnicating"),
			},
			expected: "frobnicating, unknown\n",
		},
		{
			in: codersdk.Task{
				Status: codersdk.WorkspaceStatus("frobnicating"),
				CurrentState: &codersdk.TaskStateEntry{
					State: codersdk.TaskState("reticulating splines"),
				},
			},
			expected: "frobnicating, reticulating splines\n",
		},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()
			var sb strings.Builder
			printTaskStatus(&sb, tc.in)
			assert.Equal(t, tc.expected, sb.String())
		})
	}
}
