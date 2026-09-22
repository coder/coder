package loadtestutil_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
)

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

	dup, err := loadtestutil.DupClientCopyingHeaders(t.Context(), sdkClient, map[string][]string{
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
