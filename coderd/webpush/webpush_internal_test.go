package webpush

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSRFSafeHTTPClientLimits(t *testing.T) {
	t.Parallel()
	client := newSSRFSafeHTTPClient()
	require.Equal(t, pushRequestTimeout, client.Timeout)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Equal(t, int64(maxResponseHeaderBytes), transport.MaxResponseHeaderBytes)
}
