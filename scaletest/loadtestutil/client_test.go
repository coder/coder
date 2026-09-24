package loadtestutil_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

type mutableHeaderProvider struct {
	headers http.Header
	err     error
}

func (p *mutableHeaderProvider) Headers(context.Context) (http.Header, error) {
	return p.headers, p.err
}

func TestDupClientCopyingHeadersDynamic(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	source := &mutableHeaderProvider{headers: http.Header{
		"Authorization": {"Bearer first"},
		"X-Override":    {"source"},
		"X-Removed":     {"old"},
	}}
	client := codersdk.New(&url.URL{Scheme: "https", Host: "coder.example.com"})
	client.HTTPClient.Transport = &codersdk.HeaderTransport{
		Transport: http.DefaultTransport,
		Provider:  source,
	}
	dup, err := loadtestutil.DupClientCopyingHeaders(client, http.Header{"X-Override": {"extra"}})
	require.NoError(t, err)
	transport, ok := dup.HTTPClient.Transport.(*codersdk.HeaderTransport)
	require.True(t, ok)
	headers, err := transport.Provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, "Bearer first", headers.Get("Authorization"))
	require.Equal(t, "extra", headers.Get("X-Override"))
	headers["Authorization"][0] = "mutated"
	require.Equal(t, "Bearer first", source.headers.Get("Authorization"))

	source.headers = http.Header{"Authorization": {"Bearer second"}}
	headers, err = transport.Provider.Headers(ctx)
	require.NoError(t, err)
	require.Equal(t, "Bearer second", headers.Get("Authorization"))
	require.Equal(t, "extra", headers.Get("X-Override"))
	require.Empty(t, headers.Get("X-Removed"))

	source.err = xerrors.New("credentials unavailable")
	headers, err = transport.Provider.Headers(ctx)
	require.ErrorIs(t, err, source.err)
	require.Nil(t, headers)
}

func TestDupClientCopyingHeaders(t *testing.T) {
	t.Parallel()
	httpClient := &http.Client{
		Transport: &codersdk.HeaderTransport{
			Transport: &codersdk.HeaderTransport{
				Transport: http.DefaultTransport,
				Provider: codersdk.StaticHeaderProvider{Header: http.Header{
					"X-Coder-Test":  {"foo"},
					"X-Coder-Test3": {"socks"},
					"X-Coder-Test5": {"ninjas"},
				}},
			},
			Provider: codersdk.StaticHeaderProvider{Header: http.Header{
				"X-Coder-Test":  {"bar"},
				"X-Coder-Test2": {"baz"},
			}},
		},
	}
	serverURL, err := url.Parse("http://coder.example.com")
	require.NoError(t, err)
	sdkClient := codersdk.New(serverURL,
		codersdk.WithSessionToken("test-token"), codersdk.WithHTTPClient(httpClient))

	dup, err := loadtestutil.DupClientCopyingHeaders(sdkClient, map[string][]string{
		"X-Coder-Test3": {"clocks"},
		"X-Coder-Test4": {"bears"},
	})
	require.NoError(t, err)
	require.Equal(t, "http://coder.example.com", dup.URL.String())
	require.Equal(t, "test-token", dup.SessionToken())
	ht, ok := dup.HTTPClient.Transport.(*codersdk.HeaderTransport)
	require.True(t, ok)
	headers, err := ht.Provider.Headers(t.Context())
	require.NoError(t, err)
	require.Equal(t, "bar", headers.Get("X-Coder-Test"))
	require.Equal(t, "baz", headers.Get("X-Coder-Test2"))
	require.Equal(t, "clocks", headers.Get("X-Coder-Test3"))
	require.Equal(t, "bears", headers.Get("X-Coder-Test4"))
	require.Equal(t, "ninjas", headers.Get("X-Coder-Test5"))
	require.NotEqual(t, http.DefaultTransport, ht.Transport)
}
