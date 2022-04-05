package gitsshkey_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/cryptorand"
)

func TestGitSSHKeys(t *testing.T) {
	verifyKeyPair := func(t *testing.T, private, public string) {
		signer, err := ssh.ParsePrivateKey([]byte(private))
		require.NoError(t, err)
		p, err := ssh.ParsePublicKey(signer.PublicKey().Marshal())
		require.NoError(t, err)
		publicKey := string(ssh.MarshalAuthorizedKey(p))
		require.Equal(t, publicKey, public)
	}

	t.Run("None", func(t *testing.T) {
		pv, pb, err := gitsshkey.GenerateKeyPair(gitsshkey.AlgorithmNone)
		require.NoError(t, err)
		require.Empty(t, pv)
		require.Empty(t, pb)
	})
	t.Run("Ed25519", func(t *testing.T) {
		pv, pb, err := gitsshkey.GenerateKeyPair(gitsshkey.AlgorithmEd25519)
		require.NoError(t, err)
		verifyKeyPair(t, pv, pb)
	})
	t.Run("ECDSA", func(t *testing.T) {
		pv, pb, err := gitsshkey.GenerateKeyPair(gitsshkey.AlgorithmECDSA)
		require.NoError(t, err)
		verifyKeyPair(t, pv, pb)
	})
	t.Run("RSA4096", func(t *testing.T) {
		pv, pb, err := gitsshkey.GenerateKeyPair(gitsshkey.AlgorithmRSA4096)
		require.NoError(t, err)
		verifyKeyPair(t, pv, pb)
	})
	t.Run("ParseAlgorithm", func(t *testing.T) {
		_, err := gitsshkey.ParseSSHKeygenAlgorithm(string(gitsshkey.AlgorithmNone))
		require.NoError(t, err)
		_, err = gitsshkey.ParseSSHKeygenAlgorithm(string(gitsshkey.AlgorithmEd25519))
		require.NoError(t, err)
		_, err = gitsshkey.ParseSSHKeygenAlgorithm(string(gitsshkey.AlgorithmECDSA))
		require.NoError(t, err)
		_, err = gitsshkey.ParseSSHKeygenAlgorithm(string(gitsshkey.AlgorithmRSA4096))
		require.NoError(t, err)
		r, _ := cryptorand.String(6)
		_, err = gitsshkey.ParseSSHKeygenAlgorithm(r)
		require.Error(t, err, "random string should fail")
		_, err = gitsshkey.ParseSSHKeygenAlgorithm("")
		require.Error(t, err, "empty string should fail")
	})
}
