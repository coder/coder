//go:build windows

package sessionstore_test

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"

	"github.com/danieljoos/wincred"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/sessionstore"
)

func TestWindowsKeyring_WriteReadDelete(t *testing.T) {
	t.Parallel()

	const testURL = "http://127.0.0.1:1337"
	srvURL, err := url.Parse(testURL)
	require.NoError(t, err)

	serviceName := keyringTestServiceName(t)
	backend := sessionstore.NewKeyringWithService(serviceName)
	t.Cleanup(func() { _ = backend.Delete(srvURL) })

	// Verify no token exists initially
	_, err = backend.Read(srvURL)
	require.ErrorIs(t, err, os.ErrNotExist)

	// Write a token
	const inputToken = "test-token-12345"
	err = backend.Write(srvURL, inputToken)
	require.NoError(t, err)

	// Verify the credential is stored in Windows Credential Manager with correct format
	winCred, err := wincred.GetGenericCredential(serviceName)
	require.NoError(t, err, "getting windows credential")

	var storedCreds map[string]struct {
		CoderURL string `json:"coder_url"`
		APIToken string `json:"api_token"`
	}
	err = json.Unmarshal(winCred.CredentialBlob, &storedCreds)
	require.NoError(t, err, "unmarshalling stored credentials")

	// Verify the stored values
	require.Len(t, storedCreds, 1)
	cred, ok := storedCreds[srvURL.Host]
	require.True(t, ok, "credential for URL should exist")
	require.Equal(t, inputToken, cred.APIToken)
	require.Equal(t, srvURL.Host, cred.CoderURL)

	// Read the token back
	token, err := backend.Read(srvURL)
	require.NoError(t, err)
	require.Equal(t, inputToken, token)

	// Delete the token
	err = backend.Delete(srvURL)
	require.NoError(t, err)

	// Verify token is deleted
	_, err = backend.Read(srvURL)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWindowsKeyring_MultipleServers(t *testing.T) {
	t.Parallel()

	const testURL1 = "http://127.0.0.1:1337"
	srv1URL, err := url.Parse(testURL1)
	require.NoError(t, err)

	const testURL2 = "http://127.0.0.1:1338"
	srv2URL, err := url.Parse(testURL2)
	require.NoError(t, err)

	serviceName := keyringTestServiceName(t)
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

	// Verify both credentials are stored in Windows Credential Manager
	winCred, err := wincred.GetGenericCredential(serviceName)
	require.NoError(t, err, "getting windows credential")

	var storedCreds map[string]struct {
		CoderURL string `json:"coder_url"`
		APIToken string `json:"api_token"`
	}
	err = json.Unmarshal(winCred.CredentialBlob, &storedCreds)
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
}
