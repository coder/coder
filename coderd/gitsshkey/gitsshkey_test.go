package gitsshkey_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/cryptorand"
)

func TestGitSSHKeys(t *testing.T) {
	t.Parallel()

	verifyKeyPair := func(t *testing.T, private, public string) {
		signer, err := ssh.ParsePrivateKey([]byte(private))
		require.NoError(t, err)
		p, err := ssh.ParsePublicKey(signer.PublicKey().Marshal())
		require.NoError(t, err)
		publicKey := string(ssh.MarshalAuthorizedKey(p))
		require.Equal(t, publicKey, public)
	}

	t.Run("Ed25519", func(t *testing.T) {
		t.Parallel()
		pv, pb, err := gitsshkey.Generate(gitsshkey.AlgorithmEd25519)
		require.NoError(t, err)
		verifyKeyPair(t, pv, pb)
	})
	t.Run("ECDSA", func(t *testing.T) {
		t.Parallel()
		pv, pb, err := gitsshkey.Generate(gitsshkey.AlgorithmECDSA)
		require.NoError(t, err)
		verifyKeyPair(t, pv, pb)
	})
	t.Run("RSA4096", func(t *testing.T) {
		t.Parallel()
		pv, pb, err := gitsshkey.Generate(gitsshkey.AlgorithmRSA4096)
		require.NoError(t, err)
		verifyKeyPair(t, pv, pb)
	})
	t.Run("ParseAlgorithm", func(t *testing.T) {
		t.Parallel()
		_, err := gitsshkey.ParseAlgorithm(string(gitsshkey.AlgorithmEd25519))
		require.NoError(t, err)
		_, err = gitsshkey.ParseAlgorithm(string(gitsshkey.AlgorithmECDSA))
		require.NoError(t, err)
		_, err = gitsshkey.ParseAlgorithm(string(gitsshkey.AlgorithmRSA4096))
		require.NoError(t, err)
		r, _ := cryptorand.String(6)
		_, err = gitsshkey.ParseAlgorithm(r)
		require.Error(t, err, "random string should fail")
		_, err = gitsshkey.ParseAlgorithm("")
		require.Error(t, err, "empty string should fail")
	})
}

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Note that this is using dumbRand under the hood, so it will be
		// a lot slower in production.
		_, _, err := gitsshkey.Generate(gitsshkey.AlgorithmRSA4096)
		if err != nil {
			b.Fatal(err)
		}
	}
}
