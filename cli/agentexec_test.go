package cli_test

import (
	"bytes"
	"io"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clitest"
)

func TestAgentExec(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skip("agent-exec is only supported on Linux")
	}

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "agent-exec", "echo", "hello")
		inv.Environ.Set(cli.EnvProcOOMScore, "1000")
		inv.Environ.Set(cli.EnvProcNiceScore, "10")
		var buf bytes.Buffer
		wr := &syncWriter{W: &buf}
		inv.Stdout = wr
		inv.Stderr = wr
		clitest.Start(t, inv)

		require.Equal(t, "hello\n", buf.String())
	})

}

type syncWriter struct {
	W  io.Writer
	mu sync.Mutex
}

func (w *syncWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.W.Write(p)
}
