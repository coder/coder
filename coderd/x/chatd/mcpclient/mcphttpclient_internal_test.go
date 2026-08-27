package mcpclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMCPTransportTimeouts guards the transport hardening: MCP
// traffic must never ride a transport without dial and
// response-header bounds, or a black-holed server holds
// connections until the kernel gives up (about two minutes).
func TestMCPTransportTimeouts(t *testing.T) {
	t.Parallel()

	shared := mcpSharedTransport
	require.NotNil(t, shared.DialContext)
	require.Equal(t, responseHeaderTimeout, shared.ResponseHeaderTimeout)
	// The response-header bound must not undercut the tool-call
	// budget, or slow JSON-response tools within budget would be
	// killed at the HTTP layer.
	require.GreaterOrEqual(t, responseHeaderTimeout, toolCallTimeout)

	isolated := mcpHTTPClient()
	require.NotNil(t, isolated, "must be isolated under test")
	require.NotSame(t, shared, isolated.Transport)
}
