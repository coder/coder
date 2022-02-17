package pty_test

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/coder/coder/console/pty"
	"github.com/stretchr/testify/require"
)

var (
	// Used to ensure terminal output doesn't have anything crazy!
	// See: https://stackoverflow.com/a/29497680
	stripAnsi = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
)

func TestStart(t *testing.T) {
	t.Run("Do", func(t *testing.T) {
		pty, err := pty.Run(exec.Command("powershell.exe", "echo", "test"))
		require.NoError(t, err)
		data := make([]byte, 128)
		_, err = pty.Output().Read(data)
		require.NoError(t, err)
		t.Log(fmt.Sprintf("%q", stripAnsi.ReplaceAllString(string(data), "")))
	})
}
