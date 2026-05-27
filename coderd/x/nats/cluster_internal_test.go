package nats //nolint:testpackage // Exercises internal cluster state.

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

//nolint:paralleltest // Cluster tests bind free ports and reload shared route state.
func Test_SetPeerAddresses(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		a := newTestPubsub(t, newClusterTestOptions(t))
		b := newTestPubsub(t, newClusterTestOptions(t))
		c := newTestPubsub(t, newClusterTestOptions(t))

		addrB := clusterRouteAddress(t, b)
		addrC := clusterRouteAddress(t, c)
		require.NoError(t, a.SetPeerAddresses([]string{addrC, addrB}))
		requireRoutesEqual(t, a.currentRoutes, addrB, addrC)
		requireRoutesEqual(t, a.serverOpts.Routes, addrB, addrC)

		require.NoError(t, a.SetPeerAddresses([]string{addrB, addrC}))
		requireRoutesEqual(t, a.currentRoutes, addrB, addrC)
		requireRoutesEqual(t, a.serverOpts.Routes, addrB, addrC)

		require.NoError(t, a.SetPeerAddresses(nil))
		require.Empty(t, a.currentRoutes)
		require.Empty(t, a.serverOpts.Routes)
	})

	t.Run("StandaloneConfigError", func(t *testing.T) {
		ps := newTestPubsub(t, Options{})
		err := ps.SetPeerAddresses(nil)
		require.ErrorContains(t, err, "not started with clustering enabled")
	})

	t.Run("Closed", func(t *testing.T) {
		ps := newTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.Close())
		err := ps.SetPeerAddresses(nil)
		require.True(t, errors.Is(err, errClosed), "got %v", err)
	})

	t.Run("DropsSelfRoute", func(t *testing.T) {
		ps := newTestPubsub(t, newClusterTestOptions(t))
		require.NoError(t, ps.SetPeerAddresses([]string{clusterRouteAddress(t, ps)}))
		require.Empty(t, ps.currentRoutes)
	})
}
