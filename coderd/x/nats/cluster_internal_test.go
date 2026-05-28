package nats //nolint:testpackage // Exercises internal cluster state.

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parsePeerAddresses(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
			" 127.0.0.1:4222 ",
			"[::1]:7222",
			"example.com:6222",
		})
		require.NoError(t, err)
		require.Equal(t, []string{
			"nats://127.0.0.1:4222",
			"nats://[::1]:7222",
			"nats://example.com:6222",
		}, routeStrings(routes))
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses(nil)
		require.NoError(t, err)
		require.Empty(t, routes)
	})

	t.Run("SortsAndDedupes", func(t *testing.T) {
		t.Parallel()
		routes, err := parsePeerAddresses([]string{
			"b.example:6222",
			"a.example:6222",
			"b.example:6222",
		})
		require.NoError(t, err)
		require.Equal(t, []string{
			"nats://a.example:6222",
			"nats://b.example:6222",
		}, routeStrings(routes))
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		for _, address := range []string{
			"",
			"   ",
			"http://127.0.0.1:4222",
			"127.0.0.1",
			":4222",
			"127.0.0.1:0",
			"127.0.0.1:bad",
			"nats://127.0.0.1",
			"nats://:4222",
			"nats://127.0.0.1:0",
			"nats://127.0.0.1:bad",
			"nats://user@127.0.0.1:4222",
			"nats://127.0.0.1:4222/path",
			"nats://127.0.0.1:4222?x=1",
			"nats://127.0.0.1:4222#frag",
		} {
			t.Run(address, func(t *testing.T) {
				t.Parallel()
				_, err := parsePeerAddresses([]string{address})
				require.Error(t, err)
			})
		}
	})
}

func Test_filterSelfRoutes(t *testing.T) {
	t.Parallel()

	routes, err := parsePeerAddresses([]string{
		"b.example:6222",
		"self.example:6222",
	})
	require.NoError(t, err)

	routes = filterSelfRoutes(routes, &url.URL{Scheme: "nats", Host: "self.example:6222"})
	require.Equal(t, []string{"nats://b.example:6222"}, routeStrings(routes))
}

// Cluster tests bind free ports and reload shared route state.
func TestPubsub_SetPeerAddresses(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
		a := newTestPubsub(t, newClusterTestOptions(t))
		b := newTestPubsub(t, newClusterTestOptions(t))
		c := newTestPubsub(t, newClusterTestOptions(t))

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.SetPeerAddresses([]string{addrC, addrB}))
		requireRoutesEqual(t, a.currentRoutes, addrB, addrC)
		waitForConfiguredRouteAddresses(t, a, addrB, addrC)

		require.NoError(t, a.SetPeerAddresses([]string{addrB, addrC}))
		requireRoutesEqual(t, a.currentRoutes, addrB, addrC)
		waitForConfiguredRouteAddresses(t, a, addrB, addrC)

		require.NoError(t, a.SetPeerAddresses(nil))
		require.Empty(t, a.currentRoutes)
		require.Empty(t, a.serverOpts.Routes)
	})

	t.Run("StandaloneConfigError", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, Options{})
		err := ps.SetPeerAddresses(nil)
		require.ErrorContains(t, err, "not started with clustering enabled")
	})

	t.Run("Closed", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.Close())
		err := ps.SetPeerAddresses(nil)
		require.True(t, errors.Is(err, errClosed), "got %v", err)
	})

	t.Run("DropsSelfRoute", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.SetPeerAddresses([]string{clusterRouteAddress(t, ps)}))
		require.Empty(t, ps.currentRoutes)
	})
}
