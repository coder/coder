package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/gitsshkey"
)

func TestGitSSHKey(t *testing.T) {
	t.Parallel()
	t.Run("None", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("Ed25519", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmEd25519,
		})
		_ = coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("ECDSA", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmECDSA,
		})
		_ = coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("RSA4096", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmRSA4096,
		})
		_ = coderdtest.CreateFirstUser(t, client)
		key, err := client.GitSSHKey(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, key.PublicKey)
	})
	t.Run("Regenerate", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		client := coderdtest.New(t, &coderdtest.Options{
			SSHKeygenAlgorithm: gitsshkey.AlgorithmEd25519,
		})
		_ = coderdtest.CreateFirstUser(t, client)
		key1, err := client.GitSSHKey(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, key1.PublicKey)
		key2, err := client.RegenerateGitSSHKey(ctx)
		require.NoError(t, err)
		require.Greater(t, key2.UpdatedAt, key1.UpdatedAt)
		require.NotEmpty(t, key2.PublicKey)
		require.NotEqual(t, key2.PublicKey, key1.PublicKey)
	})
}
