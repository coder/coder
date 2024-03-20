package cli_test

import (
	"bytes"
	"fmt"
	"os"
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

	var root cli.RootCmd
	rootCmd, err := root.Command(root.AGPL())
	require.NoError(t, err)

	var cases []commandErrorCase

ExtractCommandPathsLoop:
	for _, cp := range extractCommandPaths(nil, rootCmd.Children) {
		name := fmt.Sprintf("coder %s", strings.Join(cp, " "))
		// space to end to not match base exp example-error
		if !strings.Contains(name, "exp example-error ") {
			continue
		}
		for _, tt := range cases {
			if tt.Name == name {
				continue ExtractCommandPathsLoop
			}
		}
		cases = append(cases, commandErrorCase{Name: name, Cmd: cp})
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			var outBuf bytes.Buffer

			rootCmd, err := root.Command(root.AGPL())
			require.NoError(t, err)

			inv, _ := clitest.NewWithCommand(t, rootCmd, tt.Cmd...)
			inv.Stderr = &outBuf
			inv.Stdout = &outBuf

			// This example expects to close stdin twice and joins
			// the error messages to create a multi-multi error.
			if tt.Name == "coder exp example-error multi-multi-error" {
				inv.Stdin = os.Stdin
			}

			err = inv.Run()

			errFormatter := cli.ExportNewPrettyErrorFormatter(&outBuf, false)
			cli.ExportFormat(errFormatter, err)

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
