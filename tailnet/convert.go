package tailnet

import (
	"net/netip"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/tailnet/proto"
)

func UUIDToByteSlice(u uuid.UUID) []byte {
	b := [16]byte(u)
	o := make([]byte, 16)
	copy(o, b[:]) // copy so that we can't mutate the original
	return o
}

func NodeToProto(n *Node) (*proto.Node, error) {
	k, err := n.Key.MarshalBinary()
	if err != nil {
		return nil, err
	}
	disco, err := n.DiscoKey.MarshalText()
	if err != nil {
		return nil, err
	}
	derpForcedWebsocket := make(map[int32]string)
	for i, s := range n.DERPForcedWebsocket {
		derpForcedWebsocket[int32(i)] = s
	}
	addresses := make([]string, len(n.Addresses))
	for i, prefix := range n.Addresses {
		s, err := prefix.MarshalText()
		if err != nil {
			return nil, err
		}
		addresses[i] = string(s)
	}
	allowedIPs := make([]string, len(n.AllowedIPs))
	for i, prefix := range n.AllowedIPs {
		s, err := prefix.MarshalText()
		if err != nil {
			return nil, err
		}
		allowedIPs[i] = string(s)
	}
	return &proto.Node{
		Id:                  int64(n.ID),
		AsOf:                timestamppb.New(n.AsOf),
		Key:                 k,
		Disco:               string(disco),
		PreferredDerp:       int32(n.PreferredDERP),
		DerpLatency:         n.DERPLatency,
		DerpForcedWebsocket: derpForcedWebsocket,
		Addresses:           addresses,
		AllowedIps:          allowedIPs,
		Endpoints:           n.Endpoints,
	}, nil
}

func ProtoToNode(p *proto.Node) (*Node, error) {
	k := key.NodePublic{}
	err := k.UnmarshalBinary(p.GetKey())
	if err != nil {
		return nil, err
	}
	disco := key.DiscoPublic{}
	err = disco.UnmarshalText([]byte(p.GetDisco()))
	if err != nil {
		return nil, err
	}
	derpForcedWebsocket := make(map[int]string)
	for i, s := range p.GetDerpForcedWebsocket() {
		derpForcedWebsocket[int(i)] = s
	}
	addresses := make([]netip.Prefix, len(p.GetAddresses()))
	for i, prefix := range p.GetAddresses() {
		err = addresses[i].UnmarshalText([]byte(prefix))
		if err != nil {
			return nil, err
		}
	}
	allowedIPs := make([]netip.Prefix, len(p.GetAllowedIps()))
	for i, prefix := range p.GetAllowedIps() {
		err = allowedIPs[i].UnmarshalText([]byte(prefix))
		if err != nil {
			return nil, err
		}
	}
	return &Node{
		ID:                  tailcfg.NodeID(p.GetId()),
		AsOf:                p.GetAsOf().AsTime(),
		Key:                 k,
		DiscoKey:            disco,
		PreferredDERP:       int(p.GetPreferredDerp()),
		DERPLatency:         p.GetDerpLatency(),
		DERPForcedWebsocket: derpForcedWebsocket,
		Addresses:           addresses,
		AllowedIPs:          allowedIPs,
		Endpoints:           p.Endpoints,
	}, nil
}

func OnlyNodeUpdates(resp *proto.CoordinateResponse) ([]*Node, error) {
	nodes := make([]*Node, 0, len(resp.GetPeerUpdates()))
	for _, pu := range resp.GetPeerUpdates() {
		if pu.Kind != proto.CoordinateResponse_PeerUpdate_NODE {
			continue
		}
		n, err := ProtoToNode(pu.Node)
		if err != nil {
			return nil, xerrors.Errorf("failed conversion from protobuf: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func SingleNodeUpdate(id uuid.UUID, node *Node, reason string) (*proto.CoordinateResponse, error) {
	p, err := NodeToProto(node)
	if err != nil {
		return nil, xerrors.Errorf("node failed conversion to protobuf: %w", err)
	}
	return &proto.CoordinateResponse{
		PeerUpdates: []*proto.CoordinateResponse_PeerUpdate{
			{
				Kind:   proto.CoordinateResponse_PeerUpdate_NODE,
				Uuid:   UUIDToByteSlice(id),
				Node:   p,
				Reason: reason,
			},
		},
	}, nil
}
