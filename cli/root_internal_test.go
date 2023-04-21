package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func Test_formatExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		examples    []example
		wantMatches []string
	}{
		{
			name:        "No examples",
			examples:    nil,
			wantMatches: nil,
		},
		{
			name: "Output examples",
			examples: []example{
				{
					Description: "Hello world.",
					Command:     "echo hello",
				},
				{
					Description: "Bye bye.",
					Command:     "echo bye",
				},
			},
			wantMatches: []string{
				"Hello world", "echo hello",
				"Bye bye", "echo bye",
			},
		},
		{
			name: "No description outputs commands",
			examples: []example{
				{
					Command: "echo hello",
				},
			},
			wantMatches: []string{
				"echo hello",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatExamples(tt.examples...)
			if len(tt.wantMatches) == 0 {
				require.Empty(t, got)
			} else {
				for _, want := range tt.wantMatches {
					require.Contains(t, got, want)
				}
			}
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// The lumberjack library is used by by agent and seems to leave
		// goroutines after Close(), fails TestGitSSH tests.
		// https://github.com/natefinch/lumberjack/pull/100
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).mill.func1"),
		// The pq library appears to leave around a goroutine after Close().
		goleak.IgnoreTopFunction("github.com/lib/pq.NewDialListener"),
	)
}
