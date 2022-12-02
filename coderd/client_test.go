package coderd_test

import (
	"net/http"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func doWithRetries(t require.TestingT, client *codersdk.Client, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	require.Eventually(t, func() bool {
		resp, err = client.HTTPClient.Do(req)
		return resp.StatusCode != http.StatusBadGateway
	}, testutil.WaitShort, testutil.IntervalFast)
	return resp, err
}
