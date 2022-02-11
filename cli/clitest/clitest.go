package clitest

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
)

var (
	// Used to ensure terminal output doesn't have anything crazy!
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

// CreateInitialUser creates the initial user and write's the session
// token to the config root provided.
func CreateInitialUser(t *testing.T, client *codersdk.Client, root config.Root) coderd.CreateInitialUserRequest {
	user := coderdtest.CreateInitialUser(t, client)
	resp, err := client.LoginWithPassword(context.Background(), coderd.LoginWithPasswordRequest{
		Email:    user.Email,
		Password: user.Password,
	})
	require.NoError(t, err)
	err = root.Session().Write(resp.SessionToken)
	require.NoError(t, err)
	err = root.URL().Write(client.URL.String())
	require.NoError(t, err)
	return user
}

// CreateProjectVersionSource writes the echo provisioner responses into a
// new temporary testing directory.
func CreateProjectVersionSource(t *testing.T, responses *echo.Responses) string {
	directory := t.TempDir()
	data, err := echo.Tar(responses)
	require.NoError(t, err)
	err = extractTar(data, directory)
	require.NoError(t, err)
	return directory
}

// StdoutLogs provides a writer to t.Log that strips
// all ANSI escape codes.
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
			t.Log(stripAnsi.ReplaceAllString(scanner.Text(), ""))
		}
	}()
	return writer
}

func extractTar(data []byte, directory string) error {
	reader := tar.NewReader(bytes.NewBuffer(data))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return xerrors.Errorf("read project source archive: %w", err)
		}
		path := filepath.Join(directory, header.Name)
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0600
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, mode)
			if err != nil {
				return xerrors.Errorf("mkdir: %w", err)
			}
		case tar.TypeReg:
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
			if err != nil {
				return xerrors.Errorf("create file %q: %w", path, err)
			}
			// Max file size of 10MB.
			_, err = io.CopyN(file, reader, (1<<20)*10)
			if errors.Is(err, io.EOF) {
				err = nil
			}
			if err != nil {
				_ = file.Close()
				return err
			}
			err = file.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
