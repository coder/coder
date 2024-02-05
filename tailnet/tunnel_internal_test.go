package tailnet

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTunnelStore_Bidir(t *testing.T) {
	t.Parallel()
	p1 := uuid.MustParse("00000001-1111-1111-1111-111111111111")
	p2 := uuid.MustParse("00000002-1111-1111-1111-111111111111")
	uut := newTunnelStore()
	uut.add(p1, p2)
	require.Equal(t, []uuid.UUID{p1}, uut.findTunnelPeers(p2))
	require.Equal(t, []uuid.UUID{p2}, uut.findTunnelPeers(p1))
	uut.remove(p1, p2)
	require.Empty(t, uut.findTunnelPeers(p1))
	require.Empty(t, uut.findTunnelPeers(p2))
	require.Len(t, uut.byDst, 0)
	require.Len(t, uut.bySrc, 0)
}

func TestTunnelStore_RemoveAll(t *testing.T) {
	t.Parallel()
	p1 := uuid.MustParse("00000001-1111-1111-1111-111111111111")
	p2 := uuid.MustParse("00000002-1111-1111-1111-111111111111")
	p3 := uuid.MustParse("00000003-1111-1111-1111-111111111111")
	uut := newTunnelStore()
	uut.add(p1, p2)
	uut.add(p1, p3)
	uut.add(p3, p1)
	require.Len(t, uut.findTunnelPeers(p1), 2)
	require.Len(t, uut.findTunnelPeers(p2), 1)
	require.Len(t, uut.findTunnelPeers(p3), 1)
	uut.removeAll(p1)
	require.Len(t, uut.findTunnelPeers(p1), 1)
	require.Len(t, uut.findTunnelPeers(p2), 0)
	require.Len(t, uut.findTunnelPeers(p3), 1)
	uut.removeAll(p3)
	require.Len(t, uut.findTunnelPeers(p1), 0)
	require.Len(t, uut.findTunnelPeers(p2), 0)
	require.Len(t, uut.findTunnelPeers(p3), 0)
}
