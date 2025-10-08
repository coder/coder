//go:build windows

//nolint:paralleltest // Tests access the same target credential in the OS keyring.
package sessionstore_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/danieljoos/wincred"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/sessionstore"
)

const windowsCredentialName = "coder-v2-credentials"

func TestWindowsKeyring_WriteReadDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := sessionstore.NewKeyring()

	// Verify no token exists initially
	_, err = backend.Read()
	require.ErrorIs(t, err, os.ErrNotExist)

	// Write a token
	const inputToken = "test-token-12345"
	err = backend.Write(srvURL, inputToken)
	require.NoError(t, err)

	// Verify the credential is stored in Windows Credential Manager with correct format
	winCred, err := wincred.GetGenericCredential(windowsCredentialName)
	require.NoError(t, err, "getting windows credential")

	var storedCred struct {
		CoderURL string `json:"coder_url"`
		APIToken string `json:"api_token"`
	}
	err = json.Unmarshal(winCred.CredentialBlob, &storedCred)
	require.NoError(t, err, "unmarshalling stored credential")

	// Verify the stored values
	require.Equal(t, inputToken, storedCred.APIToken)
	require.Equal(t, srvURL.Host, storedCred.CoderURL)

	// Read the token back
	token, err := backend.Read()
	require.NoError(t, err)
	require.Equal(t, inputToken, token)

	// Delete the token
	err = backend.Delete()
	require.NoError(t, err)

	// Verify token is deleted
	_, err = backend.Read()
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWindowsKeyring_MultipleServers(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv1.Close()
	srv1URL, err := url.Parse(srv1.URL)
	if err != nil {
		t.Fatal(err)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv2.Close()
	srv2URL, err := url.Parse(srv2.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := sessionstore.NewKeyring()

	// Write token for server 1
	const token1 = "token-server-1"
	err = backend.Write(srv1URL, token1)
	require.NoError(t, err)

	// Read token for server 1
	token, err := backend.Read()
	require.NoError(t, err)
	require.Equal(t, token1, token)

	// Write token for server 2 (will overwrite server 1's token in Windows keyring)
	const token2 = "token-server-2"
	err = backend.Write(srv2URL, token2)
	require.NoError(t, err)

	// Read token for server 2
	token, err = backend.Read()
	require.NoError(t, err)
	require.Equal(t, token2, token)

	// Cleanup
	err = backend.Delete()
	require.NoError(t, err)
}
