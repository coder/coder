package sessionstore_test

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/sessionstore"
)

// Generate a test service name for use with the OS keyring. It uses a combination
// of the test name and a nanosecond timestamp to prevent collisions.
func keyringTestServiceName(t *testing.T) string {
	t.Helper()
	return t.Name() + "_" + fmt.Sprintf("%v", time.Now().UnixNano())
}

func TestKeyring(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("linux and darwin are not supported yet")
	}

	// This test exercises use of the operating system keyring. As a result,
	// the operating system keyring is expected to be available.

	const (
		testURL  = "http://127.0.0.1:1337"
		testURL2 = "http://127.0.0.1:1338"
	)

	t.Run("ReadNonExistent", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(keyringTestServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = backend.Delete(srvURL) })

		_, err = backend.Read(srvURL)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err), "expected os.ErrNotExist when reading non-existent token")
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(keyringTestServiceName(t))
		srvURL, err := url.Parse(testURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = backend.Delete(srvURL) })

		err = backend.Delete(srvURL)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist), "expected os.ErrNotExist when deleting non-existent token")
	})

	t.Run("WriteAndRead", func(t *testing.T) {
		t.Parallel()

		backend := sessionstore.NewKeyringWithService(keyringTestServiceName(t))
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

		backend := sessionstore.NewKeyringWithService(keyringTestServiceName(t))
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

		backend := sessionstore.NewKeyringWithService(keyringTestServiceName(t))
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

		backend := sessionstore.NewKeyringWithService(keyringTestServiceName(t))
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
