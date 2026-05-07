package coderdtest_test

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
)

func TestNewIsolatedHTTPClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.NewIsolatedHTTPClient(mustParseURL(t, "http://example.com"))
	require.NotNil(t, client.Transport)
	require.NotSame(t, http.DefaultTransport, client.Transport)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.TLSClientConfig)
}

func TestNewIsolatedHTTPSClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.NewIsolatedHTTPClient(mustParseURL(t, "https://example.com"))
	require.NotSame(t, http.DefaultTransport, client.Transport)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
}

func TestNewIsolatedHTTPClientNilURL(t *testing.T) {
	t.Parallel()

	client := coderdtest.NewIsolatedHTTPClient(nil)
	require.NotNil(t, client.Transport)
	require.NotSame(t, http.DefaultTransport, client.Transport)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.TLSClientConfig)
}

func TestCreateAnotherUserHTTPClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)
	other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

	require.NotSame(t, client.HTTPClient, other.HTTPClient)
	require.Same(t, client.HTTPClient.Transport, other.HTTPClient.Transport)

	client.HTTPClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	require.Nil(t, other.HTTPClient.CheckRedirect)
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed
}
