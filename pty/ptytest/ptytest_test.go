package ptytest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/pty/ptytest"
)

func TestPtytest(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		t.Parallel()
		pty := ptytest.New(t)
		pty.Output().Write([]byte("write"))
		pty.ExpectMatch("write")
		pty.WriteLine("read")
	})

	// See https://github.com/coder/coder/issues/2122 for the motivation
	// behind this test.
	t.Run("Cobra ptytest should not hang when output is not consumed", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name          string
			output        string
			isPlatformBug bool
		}{
			{name: "1024 is safe (does not exceed macOS buffer)", output: strings.Repeat(".", 1024)},
			{name: "1025 exceeds macOS buffer (must not hang)", output: strings.Repeat(".", 1025)},
			{name: "10241 large output", output: strings.Repeat(".", 10241)}, // 1024 * 10 + 1
		}
		for _, tt := range tests {
			tt := tt
			// nolint:paralleltest // Avoid parallel test to more easily identify the issue.
			t.Run(tt.name, func(t *testing.T) {
				cmd := cobra.Command{
					Use: "test",
					RunE: func(cmd *cobra.Command, args []string) error {
						fmt.Fprint(cmd.OutOrStdout(), tt.output)
						return nil
					},
				}

				pty := ptytest.New(t)
				cmd.SetIn(pty.Input())
				cmd.SetOut(pty.Output())
				err := cmd.Execute()
				require.NoError(t, err)
			})
		}
	})
}
