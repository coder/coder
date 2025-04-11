package workspacesdk

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"

	"tailscale.com/net/tsaddr"

	"github.com/coder/coder/v2/tailnet"
)

func TestClient_IsCoderConnectRunning(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workspaceagents/connection", r.URL.Path)
		httpapi.Write(ctx, rw, http.StatusOK, AgentConnectionInfo{
			HostnameSuffix: "test",
		})
	}))
	defer srv.Close()

	apiURL, err := url.Parse(srv.URL)
	require.NoError(t, err)
	sdkClient := codersdk.New(apiURL)
	client := New(sdkClient)

	// Right name, right IP
	expectedName := fmt.Sprintf(tailnet.IsCoderConnectEnabledFmtString, "test")
	client.resolver = &fakeResolver{t: t, hostMap: map[string][]net.IP{
		expectedName: {net.ParseIP(tsaddr.CoderServiceIPv6().String())},
	}}

	result, err := client.IsCoderConnectRunning(ctx, CoderConnectQueryOptions{})
	require.NoError(t, err)
	require.True(t, result)

	// Wrong name
	result, err = client.IsCoderConnectRunning(ctx, CoderConnectQueryOptions{HostnameSuffix: "coder"})
	require.NoError(t, err)
	require.False(t, result)

	// Not found
	client.resolver = &fakeResolver{t: t, err: &net.DNSError{IsNotFound: true}}
	result, err = client.IsCoderConnectRunning(ctx, CoderConnectQueryOptions{})
	require.NoError(t, err)
	require.False(t, result)

	// Some other error
	client.resolver = &fakeResolver{t: t, err: xerrors.New("a bad thing happened")}
	_, err = client.IsCoderConnectRunning(ctx, CoderConnectQueryOptions{})
	require.Error(t, err)

	// Right name, wrong IP
	client.resolver = &fakeResolver{t: t, hostMap: map[string][]net.IP{
		expectedName: {net.ParseIP("2001::34")},
	}}
	result, err = client.IsCoderConnectRunning(ctx, CoderConnectQueryOptions{})
	require.NoError(t, err)
	require.False(t, result)
}

type fakeResolver struct {
	t       testing.TB
	hostMap map[string][]net.IP
	err     error
}

func (f *fakeResolver) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	assert.Equal(f.t, "ip6", network)
	return f.hostMap[host], f.err
}
