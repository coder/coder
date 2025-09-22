package testutil

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// RequireEventuallyResponseOK makes HTTP GET requests to the given endpoint until it returns
// 200 OK with a valid JSON response that can be decoded into target, or until the context
// times out. This is useful for waiting for HTTP servers to become ready during tests,
// especially for metadata endpoints that may not be immediately available.
func RequireEventuallyResponseOK(ctx context.Context, t testing.TB, endpoint string, target interface{}) {
	t.Helper()

	ok := Eventually(ctx, t, func(ctx context.Context) (done bool) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return false
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return false
		}
		return true
	}, IntervalFast)

	require.True(t, ok, "endpoint %s not ready in time", endpoint)
}
