package tailnet

import (
	"context"
	"net/netip"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/tailnet/proto"
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

func TestTunnelStore_TunnelExists(t *testing.T) {
	t.Parallel()
	p1 := uuid.UUID{1}
	p2 := uuid.UUID{2}
	uut := newTunnelStore()
	require.False(t, uut.tunnelExists(p1, p2))
	require.False(t, uut.tunnelExists(p2, p1))
	uut.add(p1, p2)
	require.True(t, uut.tunnelExists(p1, p2))
	require.True(t, uut.tunnelExists(p2, p1))
	uut.remove(p1, p2)
	require.False(t, uut.tunnelExists(p1, p2))
	require.False(t, uut.tunnelExists(p2, p1))
}

func TestAgentCoordinateeAuth_Authorize_AllowedIPs(t *testing.T) {
	t.Parallel()
	agentID := uuid.MustParse("00000001-1111-1111-1111-111111111111")
	auth := AgentCoordinateeAuth{ID: agentID}

	validTailscale := TailscaleServicePrefix.PrefixFromUUID(agentID).String()
	validCoder := CoderServicePrefix.PrefixFromUUID(agentID).String()
	victim := TailscaleServicePrefix.PrefixFromUUID(uuid.MustParse("00000002-2222-2222-2222-222222222222"))

	updateSelf := func(n *proto.Node) *proto.CoordinateRequest {
		return &proto.CoordinateRequest{
			UpdateSelf: &proto.CoordinateRequest_UpdateSelf{Node: n},
		}
	}

	t.Run("ValidAllowedIPs", func(t *testing.T) {
		t.Parallel()
		err := auth.Authorize(context.Background(), updateSelf(&proto.Node{
			Addresses:  []string{validTailscale, validCoder},
			AllowedIps: []string{validTailscale, validCoder},
		}))
		require.NoError(t, err)
	})

	t.Run("ForeignAllowedIP", func(t *testing.T) {
		t.Parallel()
		// The self-address is valid, but AllowedIPs claims a victim agent's /128.
		err := auth.Authorize(context.Background(), updateSelf(&proto.Node{
			Addresses:  []string{validTailscale},
			AllowedIps: []string{victim.String()},
		}))
		require.ErrorIs(t, err, InvalidNodeAddressError{Addr: victim.Addr().String()})
	})

	t.Run("AllowedIPWrongBits", func(t *testing.T) {
		t.Parallel()
		err := auth.Authorize(context.Background(), updateSelf(&proto.Node{
			Addresses:  []string{validTailscale},
			AllowedIps: []string{netip.PrefixFrom(TailscaleServicePrefix.AddrFromUUID(agentID), 64).String()},
		}))
		require.ErrorIs(t, err, InvalidAddressBitsError{Bits: 64})
	})
}
