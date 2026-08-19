package mcpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type staticResponseRoundTripper struct {
	body string
}

func (r *staticResponseRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader(r.body)),
		ContentLength: -1,
		Header:        make(http.Header),
	}, nil
}

func TestValidatePrivateMCPToolDefinitionsRejectsNil(t *testing.T) {
	t.Parallel()

	err := validatePrivateMCPToolDefinitions([]*mcp.Tool{nil})
	require.ErrorContains(t, err, "null tool definition")
}

func TestPrivateMCPHTTPClient(t *testing.T) {
	t.Parallel()

	transport := &staticResponseRoundTripper{
		body: strings.Repeat("x", maxPrivateMCPHTTPResponseBytes+1),
	}
	base := &http.Client{
		Transport: transport,
		Timeout:   time.Second,
	}

	client := privateMCPHTTPClient(base)
	require.NotSame(t, base, client)
	require.Same(t, transport, base.Transport)
	require.Equal(t, time.Second, base.Timeout)
	require.Zero(t, client.Timeout)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com/mcp", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.ErrorIs(t, err, errPrivateMCPResponseTooLarge)
	require.Len(t, body, maxPrivateMCPHTTPResponseBytes)
}
