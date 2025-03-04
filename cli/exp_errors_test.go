package cli_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/serpent"
)

type commandErrorCase struct {
	Name string
	Cmd  []string
}

// TestErrorExamples will test the help output of the
// coder exp example-error using golden files.
func TestErrorExamples(t *testing.T) {
	t.Parallel()

	coderRootCmd := getRoot(t)

	var exampleErrorRootCmd *serpent.Command
	coderRootCmd.Walk(func(command *serpent.Command) {
		if command.Name() == "example-error" {
			// cannot abort early, but list is small
			exampleErrorRootCmd = command
		}
	})
	require.NotNil(t, exampleErrorRootCmd, "example-error command not found")

	var cases []commandErrorCase

ExtractCommandPathsLoop:
	for _, cp := range extractCommandPaths(nil, exampleErrorRootCmd.Children) {
		cmd := append([]string{"exp", "example-error"}, cp...)
		name := fmt.Sprintf("coder %s", strings.Join(cmd, " "))
		for _, tt := range cases {
			if tt.Name == name {
				continue ExtractCommandPathsLoop
			}
		}
		cases = append(cases, commandErrorCase{Name: name, Cmd: cmd})
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			var outBuf bytes.Buffer

			coderRootCmd := getRoot(t)

			inv, _ := clitest.NewWithCommand(t, coderRootCmd, tt.Cmd...)
			inv.Stderr = &outBuf
			inv.Stdout = &outBuf

			err := inv.Run()

			errFormatter := cli.NewPrettyErrorFormatter(&outBuf, false)
			errFormatter.Format(err)

			clitest.TestGoldenFile(t, tt.Name, outBuf.Bytes(), nil)
		})
	}
}

func extractCommandPaths(cmdPath []string, cmds []*serpent.Command) [][]string {
	var cmdPaths [][]string
	for _, c := range cmds {
		cmdPath := append(cmdPath, c.Name())
		cmdPaths = append(cmdPaths, cmdPath)
		cmdPaths = append(cmdPaths, extractCommandPaths(cmdPath, c.Children)...)
	}
	return cmdPaths
}

// Must return a fresh instance of cmds each time.
func getRoot(t *testing.T) *serpent.Command {
	t.Helper()

	var root cli.RootCmd
	rootCmd, err := root.Command(root.AGPL())
	require.NoError(t, err)

	return rootCmd
}
