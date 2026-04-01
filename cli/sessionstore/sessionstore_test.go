package sessionstore_test

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/sessionstore"
	"github.com/coder/coder/v2/cli/sessionstore/testhelpers"
)

type storedCredentials map[string]struct {
	CoderURL string `json:"coder_url"`
	APIToken string `json:"api_token"`
}

func TestKeyring(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("linux is not supported yet")
	}

	// This test exercises use of the operating system keyring. As a result,
	// the operating system keyring is expected to be available.

	const (
		testURL  = "http://127.0.0.1:1337"
		testURL2 = "http://127.0.0.1:1338"
	)

	t.Run("ReadNonExistent", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(testhelpers.KeyringServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = backend.Delete(srvURL) })

		_, err = backend.Read(srvURL)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err), "expected os.ErrNotExist when reading non-existent token")
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(testhelpers.KeyringServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = backend.Delete(srvURL) })

		err = backend.Delete(srvURL)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist), "expected os.ErrNotExist when deleting non-existent token")
	})

	t.Run("WriteAndRead", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(testhelpers.KeyringServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = backend.Delete(srvURL) })

		dir := t.TempDir()
		expSessionFile := path.Join(dir, "session")

		const inputToken = "test-keyring-token-12345"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		// Verify no session file was created (keyring stores in OS keyring, not file)
		_, err = os.Stat(expSessionFile)
		require.True(t, errors.Is(err, os.ErrNotExist), "expected session token file to not exist when using keyring")

		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, inputToken, token)

		// Clean up
		err = backend.Delete(srvURL)
		require.NoError(t, err)
	})

	t.Run("WriteAndDelete", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(testhelpers.KeyringServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = backend.Delete(srvURL) })

		const inputToken = "test-keyring-token-67890"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, inputToken, token)

		err = backend.Delete(srvURL)
		require.NoError(t, err)

		_, err = backend.Read(srvURL)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err), "expected os.ErrNotExist after deleting token")
	})

	t.Run("OverwriteToken", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(testhelpers.KeyringServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = backend.Delete(srvURL) })

		// Write first token
		const firstToken = "first-keyring-token"
		err = backend.Write(srvURL, firstToken)
		require.NoError(t, err)

		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, firstToken, token)

		// Overwrite with second token
		const secondToken = "second-keyring-token"
		err = backend.Write(srvURL, secondToken)
		require.NoError(t, err)

		token, err = backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, secondToken, token)

		// Clean up
		err = backend.Delete(srvURL)
		require.NoError(t, err)
	})

	t.Run("MultipleServers", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(testhelpers.KeyringServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		srvURL2, err := url.Parse(testURL2)
		require.NoError(t, err)

		t.Cleanup(func() {
			_ = backend.Delete(srvURL)
			_ = backend.Delete(srvURL2)
		})

		// Write token for server 1
		const token1 = "token-for-server-1"
		err = backend.Write(srvURL, token1)
		require.NoError(t, err)

		// Write token for server 2 (should NOT overwrite server 1)
		const token2 = "token-for-server-2"
		err = backend.Write(srvURL2, token2)
		require.NoError(t, err)

		// Read server 1's credential
		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, token1, token)

		// Read server 2's credential
		token, err = backend.Read(srvURL2)
		require.NoError(t, err)
		require.Equal(t, token2, token)

		// Delete server 1's credential
		err = backend.Delete(srvURL)
		require.NoError(t, err)

		// Verify server 1's credential is gone
		_, err = backend.Read(srvURL)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))

		// Verify server 2's credential still exists
		token, err = backend.Read(srvURL2)
		require.NoError(t, err)
		require.Equal(t, token2, token)

		// Clean up remaining credentials
		err = backend.Delete(srvURL2)
		require.NoError(t, err)
	})

	t.Run("StorageFormat", func(t *testing.T) {
		t.Parallel()
		// The storage format must remain consistent to ensure we don't break
		// compatibility with other Coder related applications that may read
		// or decode the same credential.

		const testURL1 = "http://127.0.0.1:1337"
		srv1URL, err := url.Parse(testURL1)
		require.NoError(t, err)

		const testURL2 = "http://127.0.0.1:1338"
		srv2URL, err := url.Parse(testURL2)
		require.NoError(t, err)

		serviceName := testhelpers.KeyringServiceName(t)
		backend := sessionstore.NewKeyringWithService(serviceName)
		t.Cleanup(func() {
			_ = backend.Delete(srv1URL)
			_ = backend.Delete(srv2URL)
		})

		// Write token for server 1
		const token1 = "token-server-1"
		err = backend.Write(srv1URL, token1)
		require.NoError(t, err)

		// Write token for server 2 (should NOT overwrite server 1's token)
		const token2 = "token-server-2"
		err = backend.Write(srv2URL, token2)
		require.NoError(t, err)

		// Verify both credentials are stored in the raw format and can
		// be extracted through the Backend API.
		rawCredential := readRawKeychainCredential(t, serviceName)

		storedCreds := make(storedCredentials)
		err = json.Unmarshal(rawCredential, &storedCreds)
		require.NoError(t, err, "unmarshalling stored credentials")

		// Both credentials should exist
		require.Len(t, storedCreds, 2)
		require.Equal(t, token1, storedCreds[srv1URL.Host].APIToken)
		require.Equal(t, token2, storedCreds[srv2URL.Host].APIToken)

		// Read individual credentials
		token, err := backend.Read(srv1URL)
		require.NoError(t, err)
		require.Equal(t, token1, token)

		token, err = backend.Read(srv2URL)
		require.NoError(t, err)
		require.Equal(t, token2, token)

		// Cleanup
		err = backend.Delete(srv1URL)
		require.NoError(t, err)
		err = backend.Delete(srv2URL)
		require.NoError(t, err)
	})
}

func TestFile(t *testing.T) {
	const (
		testURL  = "http://127.0.0.1:1337"
		testURL2 = "http://127.0.0.1:1338"
	)

	t.Parallel()

	t.Run("ReadNonExistent", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)

		_, err = backend.Read(srvURL)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("WriteAndRead", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)

		// Write a token
		const inputToken = "test-token-12345"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		// Verify the session file was created
		sessionFile := config.Root(dir).Session()
		require.True(t, sessionFile.Exists())

		// Read the token back
		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, inputToken, token)
	})

	t.Run("WriteAndDelete", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)

		// Write a token
		const inputToken = "test-token-67890"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		// Verify the token was written
		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, inputToken, token)

		// Delete the token
		err = backend.Delete(srvURL)
		require.NoError(t, err)

		// Verify the token is gone
		_, err = backend.Read(srvURL)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)

		// Attempt to delete a non-existent token
		err = backend.Delete(srvURL)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("OverwriteToken", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)

		// Write first token
		const firstToken = "first-token"
		err = backend.Write(srvURL, firstToken)
		require.NoError(t, err)

		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, firstToken, token)

		// Overwrite with second token
		const secondToken = "second-token"
		err = backend.Write(srvURL, secondToken)
		require.NoError(t, err)

		token, err = backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, secondToken, token)
	})

	t.Run("WriteIgnoresURL", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		srvURL2, err := url.Parse(testURL2)
		require.NoError(t, err)

		//nolint:gosec // Write with first URL test token
		const firstToken = "token-for-url1"
		err = backend.Write(srvURL, firstToken)
		require.NoError(t, err)

		//nolint:gosec // Write with second URL - should overwrite
		const secondToken = "token-for-url2"
		err = backend.Write(srvURL2, secondToken)
		require.NoError(t, err)

		// Should have the second token (File backend doesn't differentiate by URL)
		token, err := backend.Read(srvURL)
		require.NoError(t, err)
		require.Equal(t, secondToken, token)
	})
}
