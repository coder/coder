package clitest

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
)

// New creates a CLI instance with a configuration pointed to a
// temporary testing directory.
func New(t *testing.T, args ...string) (*cobra.Command, config.Root) {
	return NewWithSubcommands(t, cli.AGPL(), args...)
}

func NewWithSubcommands(
	t *testing.T, subcommands []*cobra.Command, args ...string,
) (*cobra.Command, config.Root) {
	cmd := cli.Root(subcommands)
	dir := t.TempDir()
	root := config.Root(dir)
	cmd.SetArgs(append([]string{"--global-config", dir}, args...))

	// We could consider using writers
	// that log via t.Log here instead.
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	return cmd, root
}

// SetupConfig applies the URL and SessionToken of the client to the config.
func SetupConfig(t *testing.T, client *codersdk.Client, root config.Root) {
	err := root.Session().Write(client.SessionToken())
	require.NoError(t, err)
	err = root.URL().Write(client.URL.String())
	require.NoError(t, err)
}

// CreateTemplateVersionSource writes the echo provisioner responses into a
// new temporary testing directory.
func CreateTemplateVersionSource(t *testing.T, responses *echo.Responses) string {
	directory := t.TempDir()
	f, err := ioutil.TempFile(directory, "*.tf")
	require.NoError(t, err)
	_ = f.Close()
	data, err := echo.Tar(responses)
	require.NoError(t, err)
	extractTar(t, data, directory)
	return directory
}

func extractTar(t *testing.T, data []byte, directory string) {
	reader := tar.NewReader(bytes.NewBuffer(data))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if header.Name == "." || strings.Contains(header.Name, "..") {
			continue
		}
		// #nosec
		path := filepath.Join(directory, header.Name)
		mode := header.FileInfo().Mode()
		if mode == 0 {
			mode = 0o600
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
