package tailnet

import (
	"crypto/sha256"
	"net/netip"
	"slices"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/tailnet/proto"
)

type EventSink interface {
	AddedTunnel(src, dst uuid.UUID)
	RemovedTunnel(src, dst uuid.UUID)
	SentPeerUpdate(recipient uuid.UUID, update *proto.CoordinateResponse_PeerUpdate)
}

func PeeringIDFromUUIDs(a, b uuid.UUID) []byte {
	// it's a little roundabout to construct the addrs, then convert back to slices, but I want to
	// make sure we're calling into PeeringIDFromAddrs, so that there is only one place the sorting
	// and hashing is done.
	aa := CoderServicePrefix.AddrFromUUID(a)
	ba := CoderServicePrefix.AddrFromUUID(b)
	return PeeringIDFromAddrs(aa, ba)
}

func PeeringIDFromAddrs(a, b netip.Addr) []byte {
	as := a.AsSlice()[6:]
	bs := b.AsSlice()[6:]
	h := sha256.New()
	if cmp := slices.Compare(as, bs); cmp < 0 {
		if _, err := h.Write(as); err != nil {
			panic(err)
		}
		if _, err := h.Write(bs); err != nil {
			panic(err)
		}
	} else {
		if _, err := h.Write(bs); err != nil {
			panic(err)
		}
		if _, err := h.Write(as); err != nil {
			panic(err)
		}
	}
	return h.Sum(nil)
}

type noopEventSink struct{}

func (noopEventSink) AddedTunnel(_, _ uuid.UUID)   {}
func (noopEventSink) RemovedTunnel(_, _ uuid.UUID) {}
func (noopEventSink) SentPeerUpdate(_ uuid.UUID, _ *proto.CoordinateResponse_PeerUpdate) {
}
