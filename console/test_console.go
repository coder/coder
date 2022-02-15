package console

import (
	"bufio"
	"io"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

var (
	// Used to ensure terminal output doesn't have anything crazy!
	// See: https://stackoverflow.com/a/29497680
	stripAnsi = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
)

// New creates a new TTY bound to the command provided.
// All ANSI escape codes are stripped to provide clean output.
func New(t *testing.T, cmd *cobra.Command) *Console {
	reader, writer := io.Pipe()
	scanner := bufio.NewScanner(reader)
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	go func() {
		for scanner.Scan() {
			if scanner.Err() != nil {
				return
			}
			t.Log(stripAnsi.ReplaceAllString(scanner.Text(), ""))
		}
	}()

	console, err := NewConsole(WithStdout(writer))
	require.NoError(t, err)
	t.Cleanup(func() {
		console.Close()
	})
	cmd.SetIn(console.InTty())
	cmd.SetOut(console.OutTty())
	return console
}
