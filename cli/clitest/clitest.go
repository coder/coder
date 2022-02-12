package clitest

import (
	"archive/tar"
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Netflix/go-expect"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
)

var (
	// Used to ensure terminal output doesn't have anything crazy!
	// See: https://stackoverflow.com/a/29497680
	stripAnsi = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))")
)

// New creates a CLI instance with a configuration pointed to a
// temporary testing directory.
func New(t *testing.T, args ...string) (*cobra.Command, config.Root) {
	cmd := cli.Root()
	dir := t.TempDir()
	root := config.Root(dir)
	cmd.SetArgs(append([]string{"--global-config", dir}, args...))
	return cmd, root
}

// SetupConfig applies the URL and SessionToken of the client to the config.
func SetupConfig(t *testing.T, client *codersdk.Client, root config.Root) {
	err := root.Session().Write(client.SessionToken)
	require.NoError(t, err)
	err = root.URL().Write(client.URL.String())
	require.NoError(t, err)
}

// CreateProjectVersionSource writes the echo provisioner responses into a
// new temporary testing directory.
func CreateProjectVersionSource(t *testing.T, responses *echo.Responses) string {
	directory := t.TempDir()
	data, err := echo.Tar(responses)
	require.NoError(t, err)
	extractTar(t, data, directory)
	return directory
}

// NewConsole creates a new TTY bound to the command provided.
// All ANSI escape codes are stripped to provide clean output.
func NewConsole(t *testing.T, cmd *cobra.Command) *expect.Console {
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

	console, err := expect.NewConsole(expect.WithStdout(writer))
	require.NoError(t, err)
	cmd.SetIn(console.Tty())
	cmd.SetOut(console.Tty())
	return console
}

func extractTar(t *testing.T, data []byte, directory string) {
	reader := tar.NewReader(bytes.NewBuffer(data))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		// #nosec
		path := filepath.Join(directory, header.Name)
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0600
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, mode)
			require.NoError(t, err)
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
			require.NoError(t, err)
			// Max file size of 10MB.
			_, err = io.CopyN(file, reader, (1<<20)*10)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			require.NoError(t, err)
			err = file.Close()
			require.NoError(t, err)
		}
	}
}
