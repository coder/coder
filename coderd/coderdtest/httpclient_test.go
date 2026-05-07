package coderdtest_test

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/testutil"
)

func TestCoderdtestNewHTTPClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	require.NotNil(t, client.HTTPClient.Transport)
	require.NotSame(t, http.DefaultTransport, client.HTTPClient.Transport)

	transport, ok := client.HTTPClient.Transport.(*http.Transport)
	require.True(t, ok)
	if transport.TLSClientConfig != nil {
		require.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	}
}

func TestCoderdtestNewHTTPSClient(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		TLSCertificates: []tls.Certificate{
			testutil.GenerateTLSCertificate(t, "localhost"),
		},
	})
	transport, ok := client.HTTPClient.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
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
