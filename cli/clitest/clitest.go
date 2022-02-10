package clitest

import (
	"bufio"
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func New(t *testing.T, args ...string) (*cobra.Command, config.Root) {
	cmd := cli.Root()
	dir := t.TempDir()
	root := config.Root(dir)
	cmd.SetArgs(append([]string{"--global-config", dir}, args...))
	return cmd, root
}

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
