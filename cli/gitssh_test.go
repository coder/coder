package cli_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func prepareTestGitSSH(ctx context.Context, t *testing.T) (*agentsdk.Client, string, gossh.PublicKey) {
	t.Helper()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)

	ctx, cancel := context.WithCancel(ctx)
	defer t.Cleanup(cancel) // Defer so that cancel is the first cleanup.

	// get user public key
	keypair, err := client.GitSSHKey(ctx, codersdk.Me)
	require.NoError(t, err)
	//nolint:dogsled
	pubkey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(keypair.PublicKey))
	require.NoError(t, err)

	// setup template
	agentToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(agentToken),
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	// start workspace agent
	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(agentToken)
	_ = agenttest.New(t, client.URL, agentToken, func(o *agent.Options) {
		o.Client = agentClient
	})
	_ = coderdtest.AwaitWorkspaceAgents(t, client, workspace.ID)
	return agentClient, agentToken, pubkey
}

func serveSSHForGitSSH(t *testing.T, handler func(ssh.Session), pubkeys ...gossh.PublicKey) *net.TCPAddr {
	t.Helper()

	// start ssh server
	l, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	serveOpts := []ssh.Option{
		ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			for _, pubkey := range pubkeys {
				if ssh.KeysEqual(pubkey, key) {
					return true
				}
			}
			return false
		}),
	}
	errC := make(chan error, 1)
	go func() {
		// as long as we get a successful session we don't care if the server errors
		errC <- ssh.Serve(l, handler, serveOpts...)
	}()
	t.Cleanup(func() {
		_ = l.Close() // Ensure server shutdown.
		<-errC
	})

	// start ssh session
	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return addr
}

func writePrivateKeyToFile(t *testing.T, name string, key *ecdsa.PrivateKey) {
	t.Helper()

	b, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	b = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: b,
	})

	err = os.WriteFile(name, b, 0o600)
	require.NoError(t, err)
}

func TestGitSSH(t *testing.T) {
	t.Parallel()
	t.Run("Dial", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, token, pubkey := prepareTestGitSSH(ctx, t)
		var inc int64
		errC := make(chan error, 1)
		addr := serveSSHForGitSSH(t, func(s ssh.Session) {
			atomic.AddInt64(&inc, 1)
			t.Log("got authenticated session")
			select {
			case errC <- s.Exit(0):
			default:
				t.Error("error channel is full")
			}
		}, pubkey)

		// set to agent config dir
		inv, _ := clitest.New(t,
			"gitssh",
			"--agent-url", client.SDK.URL.String(),
			"--agent-token", token,
			"--",
			fmt.Sprintf("-p%d", addr.Port),
			"-o", "StrictHostKeyChecking=no",
			"-o", "IdentitiesOnly=yes",
			"127.0.0.1",
		)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.EqualValues(t, 1, inc)

		err = <-errC
		require.NoError(t, err, "error in agent execute")
	})

	t.Run("Local SSH Keys", func(t *testing.T) {
		t.Parallel()

		home := t.TempDir()
		sshdir := filepath.Join(home, ".ssh")
		err := os.MkdirAll(sshdir, 0o700)
		require.NoError(t, err)

		idFile := filepath.Join(sshdir, "id_ed25519")
		privkey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		localPubkey, err := gossh.NewPublicKey(&privkey.PublicKey)
		require.NoError(t, err)
		writePrivateKeyToFile(t, idFile, privkey)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		client, token, coderPubkey := prepareTestGitSSH(ctx, t)

		authkey := make(chan gossh.PublicKey, 1)
		addr := serveSSHForGitSSH(t, func(s ssh.Session) {
			t.Logf("authenticated with: %s", gossh.MarshalAuthorizedKey(s.PublicKey()))
			select {
			case authkey <- s.PublicKey():
			default:
				t.Error("authkey channel is full")
			}
		}, localPubkey, coderPubkey)

		// Create a new config which sets an identity file.
		config := filepath.Join(sshdir, "config")
		knownHosts := filepath.Join(sshdir, "known_hosts")
		err = os.WriteFile(config, []byte(strings.Join([]string{
			"Host mytest",
			"  HostName 127.0.0.1",
			fmt.Sprintf("  Port %d", addr.Port),
			"  StrictHostKeyChecking no",
			"  UserKnownHostsFile=" + knownHosts,
			"  IdentitiesOnly yes",
			"  IdentityFile=" + idFile,
		}, "\n")), 0o600)
		require.NoError(t, err)

		pty := ptytest.New(t)
		cmdArgs := []string{
			"gitssh",
			"--agent-url", client.SDK.URL.String(),
			"--agent-token", token,
			"--",
			"-F", config,
			"mytest",
		}
		// Test authentication via local private key.
		inv, _ := clitest.New(t, cmdArgs...)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		select {
		case key := <-authkey:
			require.Equal(t, localPubkey, key)
		case <-ctx.Done():
			t.Fatal("timeout waiting for auth")
		}

		// Delete the local private key.
		err = os.Remove(idFile)
		require.NoError(t, err)

		// With the local file deleted, the coder key should be used.
		inv, _ = clitest.New(t, cmdArgs...)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		select {
		case key := <-authkey:
			require.Equal(t, coderPubkey, key)
		case <-ctx.Done():
			t.Fatal("timeout waiting for auth")
		}
	})
}
