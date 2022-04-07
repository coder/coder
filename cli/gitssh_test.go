package cli_test

import (
	"bytes"
	"testing"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/pty/ptytest"
	"github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/require"
)

func TestGitSSH(t *testing.T) {
	t.Parallel()
	t.Run("SSH", func(t *testing.T) {
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)

		cmd, root := clitest.New(t, "publickey")
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		cmd.SetOutput(buf)
		err := cmd.Execute()
		require.NoError(t, err)
		publicKey := buf.String()

		l, err := ssh.NewAgentListener()
		require.NoError(t, err)
		defer l.Close()
		publicKeyOption := ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			t.Log(string(key.Marshal()))
			t.Log(publicKey)
			return string(key.Marshal()) == publicKey
		})
		go func() {
			ssh.Serve(l, func(s ssh.Session) {
				_, _ = s.Write([]byte("yay"))
				_ = s.Exit(0)
			}, publicKeyOption)
		}()

		cmd, root = clitest.New(t, "gitssh", l.Addr().String())
		clitest.SetupConfig(t, client, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())
		go func() {
			defer close(doneChan)
			err := cmd.Execute()
			require.NoError(t, err)
		}()
		pty.WriteLine("exit")
		<-doneChan
	})
}
