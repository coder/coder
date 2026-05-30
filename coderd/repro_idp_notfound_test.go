package coderd_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionerd"
	provisionerdproto "github.com/coder/coder/v2/provisionerd/proto"
)

// TestFakeIDPNotFoundPortReuse verifies that the FakeIDP's NotFound
// handler does not fail the owning test when a stale provisionerd
// reconnect hits the IDP's port after OS port reuse.
//
// This test uses real production code paths:
//   - provisionerd.New with its real connect loop and retry backoff
//   - codersdk.Client.ServeProvisionerDaemon as the dialer
//   - FakeIDP.httpHandler mux.NotFound (the bug site)
func TestFakeIDPNotFoundPortReuse(t *testing.T) {
	t.Parallel()

	// Start an HTTP server on a random port (simulates a coderd
	// server from a concurrent test).
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	addr := fmt.Sprintf("http://localhost:%d", port)

	srv := &http.Server{
		Handler:           http.NotFoundHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go srv.Serve(listener) //nolint:errcheck

	// Start provisionerd with a real dialer that calls
	// codersdk.Client.ServeProvisionerDaemon.
	orgID := uuid.New()
	parsedURL, err := url.Parse(addr)
	require.NoError(t, err)
	client := codersdk.New(parsedURL)
	client.SetSessionToken("fake-token-for-reproduction")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	daemon := provisionerd.New(
		func(dialCtx context.Context) (provisionerdproto.DRPCProvisionerDaemonClient, error) {
			return client.ServeProvisionerDaemon(dialCtx, codersdk.ServeProvisionerDaemonRequest{
				Organization: orgID,
				Name:         "repro-daemon",
				Provisioners: []codersdk.ProvisionerType{codersdk.ProvisionerTypeEcho},
			})
		},
		&provisionerd.Options{
			UpdateInterval:      time.Second,
			ForceCancelInterval: time.Second,
		},
	)
	t.Cleanup(func() {
		cancel()
		_ = daemon.Close()
	})

	// Give provisionerd time to start its connect loop.
	time.Sleep(200 * time.Millisecond)

	// Close the HTTP server, freeing the port.
	_ = srv.Shutdown(ctx)

	// Start a FakeIDP on the same port. The provisionerd reconnect
	// loop will hit the IDP's NotFound handler.
	issuer := fmt.Sprintf("http://localhost:%d", port)
	_ = oidctest.NewFakeIDP(t,
		oidctest.WithServing(),
		oidctest.WithIssuer(issuer),
	)

	// Wait for the provisionerd reconnect to reach the IDP.
	// After the fix, t.Failed() should remain false because the
	// NotFound handler should not t.Errorf for non-IDP paths.
	time.Sleep(2 * time.Second)

	// The test should NOT have failed. If it did, the NotFound
	// handler incorrectly failed the test for a cross-test request.
	require.False(t, t.Failed(), "IDP NotFound handler should not fail the test for non-IDP request paths")
}
