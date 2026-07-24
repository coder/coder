package coderdtest_test

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestNewIsolatedHTTPClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.NewIsolatedHTTPClient(testutil.MustURL(t, "http://example.com"))
	require.NotNil(t, client.Transport)
	require.NotSame(t, http.DefaultTransport, client.Transport)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.TLSClientConfig)
}

func TestNewIsolatedHTTPSClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.NewIsolatedHTTPClient(testutil.MustURL(t, "https://example.com"))
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
	client.HTTPClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	other, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

	require.NotSame(t, client.HTTPClient, other.HTTPClient)
	require.Same(t, client.HTTPClient.Transport, other.HTTPClient.Transport)
	require.Nil(t, other.HTTPClient.CheckRedirect)
}

func TestCreateAnotherUserHTTPClientDefaultTransport(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	first := coderdtest.CreateFirstUser(t, client)
	base := codersdk.New(
		client.URL,
		codersdk.WithSessionToken(client.SessionToken()),
		codersdk.WithHTTPClient(&http.Client{Timeout: time.Second}),
	)

	other, _ := coderdtest.CreateAnotherUser(t, base, first.OrganizationID)

	require.NotSame(t, base.HTTPClient, other.HTTPClient)
	require.NotNil(t, other.HTTPClient.Transport)
	require.NotSame(t, http.DefaultTransport, other.HTTPClient.Transport)
	require.Equal(t, base.HTTPClient.Timeout, other.HTTPClient.Timeout)
}
