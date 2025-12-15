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
	"github.com/coder/coder/v2/cli/sessionstore/testhelpers"
)

func readRawKeychainCredential(t *testing.T, serviceName string) []byte {
	t.Helper()

	winCred, err := wincred.GetGenericCredential(serviceName)
	if err != nil {
		t.Fatal(err)
	}
	return winCred.CredentialBlob
}

func TestWindowsKeyring_WriteReadDelete(t *testing.T) {
	t.Parallel()

	const testURL = "http://127.0.0.1:1337"
	srvURL, err := url.Parse(testURL)
	require.NoError(t, err)

	serviceName := testhelpers.KeyringServiceName(t)
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

	storedCreds := make(storedCredentials)
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
