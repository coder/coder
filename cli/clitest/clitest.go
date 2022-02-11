package clitest

import (
	"bufio"
	"io"
	"testing"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/config"
)

func New(t *testing.T, args ...string) (*cobra.Command, config.Root) {
	cmd := cli.Root()
	dir := t.TempDir()
	root := config.Root(dir)
	cmd.SetArgs(append([]string{"--global-config", dir}, args...))
	return cmd, root
}

func StdoutLogs(t *testing.T) io.Writer {
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
			t.Log(scanner.Text())
		}
	}()
	return writer
}
