package tailnet_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"

	"github.com/coder/coder/v2/testutil"
)

// TestForwarderTimeWait checks how a gVisor stack that accepts TCP through a
// tcp.Forwarder replies to segments for a 4-tuple it holds in TIME_WAIT. The
// agent netstack accepts TCP this way, and these behaviors exist only in the
// coder/gvisor fork (coder/gvisor#4) that go.mod pins. The test sends the
// peer's segments into a channel endpoint and reads the stack's replies.
func TestForwarderTimeWait(t *testing.T) {
	t.Parallel()

	// Two copies of a reopening SYN are queued on the TIME_WAIT endpoint.
	// The stack must reply with a SYN-ACK and must not reply with RST to the
	// second copy. A RST would make the client's connect fail.
	t.Run("DuplicateReopeningSYN", func(t *testing.T) {
		t.Parallel()
		p := newTimeWaitPeer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		p.toTimeWait(ctx)

		p.s.Pause()
		p.send(5000, 0, header.TCPFlagSyn)
		p.send(5000, 0, header.TCPFlagSyn)
		p.s.Resume()
		synAck := p.readUntil(ctx, header.TCPFlagSyn|header.TCPFlagAck)
		require.EqualValues(t, 5001, synAck.AckNumber())
		p.send(5001, synAck.SequenceNumber()+1, header.TCPFlagAck)
		// The forwarder handler closes the reopened connection.
		p.readUntil(ctx, header.TCPFlagFin)
	})

	// A reopening SYN is queued, then a stale SYN below the old RcvNxt. The
	// SYN-ACK must acknowledge the reopening SYN. If the stale SYN reached
	// the forwarder first, the client would receive a SYN-ACK for a
	// sequence number it did not send.
	t.Run("StaleSYN", func(t *testing.T) {
		t.Parallel()
		p := newTimeWaitPeer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		p.toTimeWait(ctx)

		p.s.Pause()
		p.send(5000, 0, header.TCPFlagSyn)
		p.send(500, 0, header.TCPFlagSyn)
		p.s.Resume()
		synAck := p.readUntil(ctx, header.TCPFlagSyn|header.TCPFlagAck)
		require.EqualValues(t, 5001, synAck.AckNumber())
		p.send(5001, synAck.SequenceNumber()+1, header.TCPFlagAck)
		p.readUntil(ctx, header.TCPFlagFin)
	})

	// A SYN-ACK does not reopen the 4-tuple. The stack must drop it and keep
	// TIME_WAIT, so it still replies to a retransmitted FIN with an ACK.
	t.Run("SynAckKeepsTimeWait", func(t *testing.T) {
		t.Parallel()
		p := newTimeWaitPeer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		fin := p.toTimeWait(ctx)

		p.send(900000, fin.SequenceNumber()+1, header.TCPFlagSyn|header.TCPFlagAck)
		p.send(1001, fin.SequenceNumber()+1, header.TCPFlagFin|header.TCPFlagAck)
		ack := p.read(ctx)
		require.Equal(t, header.TCPFlagAck, ack.Flags(), "reply to the retransmitted FIN")
		require.Equal(t, fin.SequenceNumber()+1, ack.SequenceNumber())
		require.EqualValues(t, 1002, ack.AckNumber())
	})

	// The default handler returns false for a reopening SYN. The stack must
	// reply with RST, as it does outside TIME_WAIT, so the client's connect
	// fails at once instead of waiting until TIME_WAIT expires.
	t.Run("DeclinedReopeningSYNGetsReset", func(t *testing.T) {
		t.Parallel()
		p := newTimeWaitPeer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		p.toTimeWait(ctx)

		p.decline.Store(true)
		p.send(5000, 0, header.TCPFlagSyn)
		rst := p.read(ctx)
		require.Equal(t, header.TCPFlagRst|header.TCPFlagAck, rst.Flags())
		require.EqualValues(t, 5001, rst.AckNumber())
	})
}

var (
	timeWaitPeerAddr  = tcpip.AddrFrom16([16]byte{0xfd, 0x7a, 0x11, 0x5c, 15: 1})
	timeWaitStackAddr = tcpip.AddrFrom16([16]byte{0xfd, 0x7a, 0x11, 0x5c, 15: 2})
)

const timeWaitPeerPort, timeWaitStackPort = 40000, 80

// timeWaitPeer is the remote end of a gVisor stack that accepts TCP through a
// tcp.Forwarder whose handler closes every connection at once.
type timeWaitPeer struct {
	t    *testing.T
	s    *stack.Stack
	link *channel.Endpoint
	// decline makes the stack's default handler return false.
	decline atomic.Bool
}

func newTimeWaitPeer(t *testing.T) *timeWaitPeer {
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	link := channel.New(64, 1280, "")
	p := &timeWaitPeer{t: t, s: s, link: link}
	t.Cleanup(func() {
		s.Close()
		s.Wait()
		link.Close()
	})
	fwd := tcp.NewForwarder(s, 0, 16, func(r *tcp.ForwarderRequest) {
		var wq waiter.Queue
		ep, err := r.CreateEndpoint(&wq)
		if err != nil {
			r.Complete(true)
			return
		}
		r.Complete(false)
		ep.Close()
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, func(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
		if p.decline.Load() {
			return false
		}
		return fwd.HandlePacket(id, pkt)
	})
	require.Nil(t, s.CreateNIC(1, link))
	require.Nil(t, s.AddProtocolAddress(1, tcpip.ProtocolAddress{
		Protocol:          ipv6.ProtocolNumber,
		AddressWithPrefix: timeWaitStackAddr.WithPrefix(),
	}, stack.AddressProperties{}))
	s.SetRouteTable([]tcpip.Route{{Destination: header.IPv6EmptySubnet, NIC: 1}})
	return p
}

// toTimeWait runs one connection that the stack closes first and returns the
// stack's FIN. The stack's endpoint is then in TIME_WAIT with RcvNxt 1002.
func (p *timeWaitPeer) toTimeWait(ctx context.Context) header.TCP {
	p.send(1000, 0, header.TCPFlagSyn)
	synAck := p.readUntil(ctx, header.TCPFlagSyn|header.TCPFlagAck)
	p.send(1001, synAck.SequenceNumber()+1, header.TCPFlagAck)
	fin := p.readUntil(ctx, header.TCPFlagFin)
	p.send(1001, fin.SequenceNumber()+1, header.TCPFlagFin|header.TCPFlagAck)
	ack := p.read(ctx)
	require.Equal(p.t, header.TCPFlagAck, ack.Flags(), "ACK of the peer's FIN")
	return fin
}

// send delivers one TCP segment from the peer to the stack.
func (p *timeWaitPeer) send(seq, ack uint32, flags header.TCPFlags) {
	b := make([]byte, header.IPv6MinimumSize+header.TCPMinimumSize)
	header.IPv6(b).Encode(&header.IPv6Fields{
		PayloadLength:     header.TCPMinimumSize,
		TransportProtocol: tcp.ProtocolNumber,
		HopLimit:          64,
		SrcAddr:           timeWaitPeerAddr,
		DstAddr:           timeWaitStackAddr,
	})
	th := header.TCP(b[header.IPv6MinimumSize:])
	th.Encode(&header.TCPFields{
		SrcPort:    timeWaitPeerPort,
		DstPort:    timeWaitStackPort,
		SeqNum:     seq,
		AckNum:     ack,
		DataOffset: header.TCPMinimumSize,
		Flags:      flags,
		WindowSize: 65535,
	})
	xsum := header.PseudoHeaderChecksum(tcp.ProtocolNumber, timeWaitPeerAddr, timeWaitStackAddr, header.TCPMinimumSize)
	th.SetChecksum(^th.CalculateChecksum(xsum))
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(b)})
	defer pkt.DecRef()
	p.link.InjectInbound(ipv6.ProtocolNumber, pkt)
}

// read returns the next segment the stack sends.
func (p *timeWaitPeer) read(ctx context.Context) header.TCP {
	p.t.Helper()
	pkt := p.link.ReadContext(ctx)
	require.NotNil(p.t, pkt, "no segment from the stack: %v", ctx.Err())
	defer pkt.DecRef()
	ip := header.IPv6(pkt.ToView().AsSlice())
	return header.TCP(ip.Payload())
}

// readUntil returns the next segment whose flags include want. It fails if
// the stack sends a RST before that segment.
func (p *timeWaitPeer) readUntil(ctx context.Context, want header.TCPFlags) header.TCP {
	p.t.Helper()
	for {
		seg := p.read(ctx)
		require.False(p.t, seg.Flags().Contains(header.TCPFlagRst), "RST while waiting for %v", want)
		if seg.Flags().Contains(want) {
			return seg
		}
	}
}
