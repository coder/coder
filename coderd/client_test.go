package coderd_test

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

// Issue: https://github.com/coder/coder/issues/5249
// While running tests in parallel, the web server seems to be overloaded and responds with HTTP 502.
// require.Eventually expects correct HTTP responses.

func doWithRetries(t require.TestingT, client *codersdk.Client, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	require.Eventually(t, func() bool {
		resp, err = client.HTTPClient.Do(req)
		if resp != nil && resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			return false
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
	return resp, err
}

// context-as-argument: context.Context should be the first parameter of a function (revive)
// #nosec
func requestWithRetries(t require.TestingT, client *codersdk.Client, ctx context.Context, method, path string, body interface{}, opts ...codersdk.RequestOption) (*http.Response, error) {
	var resp *http.Response
	var err error
	require.Eventually(t, func() bool {
		resp, err = client.Request(ctx, method, path, body, opts...)
		if resp != nil && resp.StatusCode == http.StatusBadGateway {
			resp.Body.Close()
			return false
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
	return resp, err
}
