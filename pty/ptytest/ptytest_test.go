package ptytest_test

import (
	"fmt"
	"os/exec"
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

	// nolint:paralleltest
	t.Run("Do not hang on Intel macOS", func(t *testing.T) {
		cmd := exec.Command("sh", "-c", "echo hi, I will cause a hang")
		pty := ptytest.New(t)
		cmd.Stdin = pty.Input()
		cmd.Stdout = pty.Output()
		err := cmd.Run()
		require.NoError(t, err)
	})

	// nolint:paralleltest
	t.Run("CobraCommandWorksLinux", func(t *testing.T) {
		// Example with cobra command instead of exec. More abstractions, but
		// for some reason works on linux.
		cmd := cobra.Command{
			Use: "test",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("Hello world")
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
