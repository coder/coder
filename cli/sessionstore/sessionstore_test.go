package sessionstore_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/sessionstore"
)

//nolint:paralleltest // Tests access the same target credential in the OS keyring.
func TestKeyring(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("this test isn't supported on linux due to lack of a native keyring and darwin because it's not supported yet")
	}

	// This test exercises use of the operating system keyring. As a result,
	// the operating system keyring is expected to be available.
	backend := sessionstore.NewKeyring()
	t.Cleanup(func() { _ = backend.Delete() })

	t.Run("ReadNonExistent", func(t *testing.T) {
		// Clean up any existing token first
		_ = backend.Delete()

		_, err := backend.Read()
		require.Error(t, err)
		require.True(t, os.IsNotExist(err), "expected os.ErrNotExist when reading non-existent token")
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		// Clean up any existing token first
		_ = backend.Delete()

		err := backend.Delete()
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist), "expected os.ErrNotExist when deleting non-existent token")
	})

	t.Run("WriteAndRead", func(t *testing.T) {
		dir := t.TempDir()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		expSessionFile := path.Join(dir, "session")

		// Clean up any existing token first
		_ = backend.Delete()

		// Write a token
		const inputToken = "test-keyring-token-12345"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		// Verify no session file was created (keyring stores in OS keyring, not file)
		_, err = os.Stat(expSessionFile)
		require.True(t, errors.Is(err, os.ErrNotExist), "expected session token file to not exist when using keyring")

		// Read the token back
		token, err := backend.Read()
		require.NoError(t, err)
		require.Equal(t, inputToken, token)

		// Clean up
		err = backend.Delete()
		require.NoError(t, err)
	})

	t.Run("WriteAndDelete", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		// Clean up any existing token first
		_ = backend.Delete()

		// Write a token
		const inputToken = "test-keyring-token-67890"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		// Verify the token was written
		token, err := backend.Read()
		require.NoError(t, err)
		require.Equal(t, inputToken, token)

		// Delete the token
		err = backend.Delete()
		require.NoError(t, err)

		// Verify the token is gone
		_, err = backend.Read()
		require.Error(t, err)
		require.True(t, os.IsNotExist(err), "expected os.ErrNotExist after deleting token")
	})

	t.Run("OverwriteToken", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		// Write first token
		const firstToken = "first-keyring-token"
		err = backend.Write(srvURL, firstToken)
		require.NoError(t, err)

		token, err := backend.Read()
		require.NoError(t, err)
		require.Equal(t, firstToken, token)

		// Overwrite with second token
		const secondToken = "second-keyring-token"
		err = backend.Write(srvURL, secondToken)
		require.NoError(t, err)

		token, err = backend.Read()
		require.NoError(t, err)
		require.Equal(t, secondToken, token)

		// Clean up
		err = backend.Delete()
		require.NoError(t, err)
	})
}

func TestFile(t *testing.T) {
	t.Parallel()

	t.Run("ReadNonExistent", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })

		_, err := backend.Read()
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("WriteAndRead", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })

		// Write a token
		const inputToken = "test-token-12345"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		// Verify the session file was created
		sessionFile := config.Root(dir).Session()
		require.True(t, sessionFile.Exists())

		// Read the token back
		token, err := backend.Read()
		require.NoError(t, err)
		require.Equal(t, inputToken, token)
	})

	t.Run("WriteAndDelete", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })

		// Write a token
		const inputToken = "test-token-67890"
		err = backend.Write(srvURL, inputToken)
		require.NoError(t, err)

		// Verify the token was written
		token, err := backend.Read()
		require.NoError(t, err)
		require.Equal(t, inputToken, token)

		// Delete the token
		err = backend.Delete()
		require.NoError(t, err)

		// Verify the token is gone
		_, err = backend.Read()
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })

		// Attempt to delete a non-existent token
		err := backend.Delete()
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("OverwriteToken", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		srvURL, err := url.Parse(srv.URL)
		require.NoError(t, err)

		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })

		// Write first token
		const firstToken = "first-token"
		err = backend.Write(srvURL, firstToken)
		require.NoError(t, err)

		token, err := backend.Read()
		require.NoError(t, err)
		require.Equal(t, firstToken, token)

		// Overwrite with second token
		const secondToken = "second-token"
		err = backend.Write(srvURL, secondToken)
		require.NoError(t, err)

		token, err = backend.Read()
		require.NoError(t, err)
		require.Equal(t, secondToken, token)
	})

	t.Run("WriteIgnoresURL", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		backend := sessionstore.NewFile(func() config.Root { return config.Root(dir) })

		// The File backend ignores the URL parameter (unlike Keyring)
		srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv1.Close()
		srvURL1, err := url.Parse(srv1.URL)
		require.NoError(t, err)

		srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv2.Close()
		srvURL2, err := url.Parse(srv2.URL)
		require.NoError(t, err)

		//nolint:gosec // Write with first URL test token
		const firstToken = "token-for-url1"
		err = backend.Write(srvURL1, firstToken)
		require.NoError(t, err)

		//nolint:gosec // Write with second URL - should overwrite
		const secondToken = "token-for-url2"
		err = backend.Write(srvURL2, secondToken)
		require.NoError(t, err)

		// Should have the second token (File backend doesn't differentiate by URL)
		token, err := backend.Read()
		require.NoError(t, err)
		require.Equal(t, secondToken, token)
	})
}
