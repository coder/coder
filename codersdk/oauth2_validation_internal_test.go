package codersdk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsLoopbackAddress(t *testing.T) {
	t.Parallel()

	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		require.True(t, isLoopbackAddress(host), host)
	}
	// Callers pass url.URL.Hostname(), which strips IPv6 brackets.
	for _, host := range []string{"", "[::1]", "app.localhost", "127.0.0.2", "0.0.0.0", "example.com"} {
		require.False(t, isLoopbackAddress(host), host)
	}
}
