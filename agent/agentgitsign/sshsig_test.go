package agentgitsign_test

import (
	"crypto/sha512"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/coder/coder/v2/agent/agentgitsign"
	"github.com/coder/coder/v2/coderd/gitsshkey"
)

func TestSignSSHSig(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not found")
	}

	for _, algo := range []gitsshkey.Algorithm{gitsshkey.AlgorithmEd25519, gitsshkey.AlgorithmECDSA, gitsshkey.AlgorithmRSA4096} {
		t.Run(string(algo), func(t *testing.T) {
			t.Parallel()
			privateKey, publicKey, err := gitsshkey.Generate(algo)
			require.NoError(t, err)
			signer, err := ssh.ParsePrivateKey([]byte(privateKey))
			require.NoError(t, err)

			message := "tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\n\nsigned commit\n"
			digest := sha512.Sum512([]byte(message))
			signature, err := agentgitsign.SignSSHSig(signer, "git", "sha512", digest[:])
			require.NoError(t, err)

			// ssh-keygen is the reference verifier, the same one git and
			// GitHub-compatible tooling rely on.
			dir := t.TempDir()
			sigFile := filepath.Join(dir, "msg.sig")
			require.NoError(t, os.WriteFile(sigFile, []byte(signature), 0o600))
			allowed := filepath.Join(dir, "allowed_signers")
			require.NoError(t, os.WriteFile(allowed, []byte("coder "+publicKey), 0o600))
			verify := exec.Command("ssh-keygen", "-Y", "verify", "-f", allowed, "-I", "coder", "-n", "git", "-s", sigFile)
			verify.Stdin = strings.NewReader(message)
			out, err := verify.CombinedOutput()
			require.NoError(t, err, string(out))

			// A signature for one namespace must not verify in another.
			verify = exec.Command("ssh-keygen", "-Y", "verify", "-f", allowed, "-I", "coder", "-n", "file", "-s", sigFile)
			verify.Stdin = strings.NewReader(message)
			require.Error(t, verify.Run())
		})
	}
}

func TestSignSSHSig_InvalidInput(t *testing.T) {
	t.Parallel()
	privateKey, _, err := gitsshkey.Generate(gitsshkey.AlgorithmEd25519)
	require.NoError(t, err)
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	require.NoError(t, err)

	_, err = agentgitsign.SignSSHSig(signer, "", "sha512", make([]byte, sha512.Size))
	require.Error(t, err)
	_, err = agentgitsign.SignSSHSig(signer, "git", "md5", make([]byte, 16))
	require.Error(t, err)
}
