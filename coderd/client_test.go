package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// Issue: https://github.com/coder/coder/issues/5249
// While running tests in parallel, the web server seems to be overloaded and responds with HTTP 502.
// testutil.Eventually expects correct HTTP responses.

func requestWithRetries(ctx context.Context, t testing.TB, client *codersdk.Client, method, path string, body interface{}, opts ...codersdk.RequestOption) (*http.Response, error) {
	var resp *http.Response
	var err error
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		// nolint // only requests which are not passed upstream have a body closed
		resp, err = client.Request(ctx, method, path, body, opts...)
		if resp != nil && resp.StatusCode == http.StatusBadGateway {
			if resp.Body != nil {
				resp.Body.Close()
			}
			return false
		}
		return true
	}, testutil.IntervalFast)
	return resp, err
}
