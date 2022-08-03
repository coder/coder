package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
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
					Description: "Hello world",
					Command:     "echo hello",
				},
				{
					Description: "Bye bye",
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
	// Replace with goleak.VerifyTestMain(m) when we enable goleak.
	os.Exit(m.Run())
	// goleak.VerifyTestMain(m)
}
