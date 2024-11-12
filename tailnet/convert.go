package tailnet

import (
	"net/netip"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/codersdk"
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
				Id:     UUIDToByteSlice(id),
				Node:   p,
				Reason: reason,
			},
		},
	}, nil
}

func DERPMapToProto(derpMap *tailcfg.DERPMap) *proto.DERPMap {
	if derpMap == nil {
		return nil
	}

	regionScore := make(map[int64]float64)
	if derpMap.HomeParams != nil {
		for k, v := range derpMap.HomeParams.RegionScore {
			regionScore[int64(k)] = v
		}
	}

	regions := make(map[int64]*proto.DERPMap_Region, len(derpMap.Regions))
	for regionID, region := range derpMap.Regions {
		regions[int64(regionID)] = DERPRegionToProto(region)
	}

	return &proto.DERPMap{
		HomeParams: &proto.DERPMap_HomeParams{
			RegionScore: regionScore,
		},
		Regions: regions,
	}
}

func DERPRegionToProto(region *tailcfg.DERPRegion) *proto.DERPMap_Region {
	if region == nil {
		return nil
	}

	regionNodes := make([]*proto.DERPMap_Region_Node, len(region.Nodes))
	for i, node := range region.Nodes {
		regionNodes[i] = DERPNodeToProto(node)
	}

	return &proto.DERPMap_Region{
		RegionId:      int64(region.RegionID),
		EmbeddedRelay: region.EmbeddedRelay,
		RegionCode:    region.RegionCode,
		RegionName:    region.RegionName,
		Avoid:         region.Avoid,

		Nodes: regionNodes,
	}
}

func DERPNodeToProto(node *tailcfg.DERPNode) *proto.DERPMap_Region_Node {
	if node == nil {
		return nil
	}

	return &proto.DERPMap_Region_Node{
		Name:             node.Name,
		RegionId:         int64(node.RegionID),
		HostName:         node.HostName,
		CertName:         node.CertName,
		Ipv4:             node.IPv4,
		Ipv6:             node.IPv6,
		StunPort:         int32(node.STUNPort),
		StunOnly:         node.STUNOnly,
		DerpPort:         int32(node.DERPPort),
		InsecureForTests: node.InsecureForTests,
		ForceHttp:        node.ForceHTTP,
		StunTestIp:       node.STUNTestIP,
		CanPort_80:       node.CanPort80,
	}
}

func DERPMapFromProto(derpMap *proto.DERPMap) *tailcfg.DERPMap {
	if derpMap == nil {
		return nil
	}

	regionScore := make(map[int]float64, len(derpMap.HomeParams.RegionScore))
	for k, v := range derpMap.HomeParams.RegionScore {
		regionScore[int(k)] = v
	}

	regions := make(map[int]*tailcfg.DERPRegion, len(derpMap.Regions))
	for regionID, region := range derpMap.Regions {
		regions[int(regionID)] = DERPRegionFromProto(region)
	}

	return &tailcfg.DERPMap{
		HomeParams: &tailcfg.DERPHomeParams{
			RegionScore: regionScore,
		},
		Regions: regions,
	}
}

func DERPRegionFromProto(region *proto.DERPMap_Region) *tailcfg.DERPRegion {
	if region == nil {
		return nil
	}

	regionNodes := make([]*tailcfg.DERPNode, len(region.Nodes))
	for i, node := range region.Nodes {
		regionNodes[i] = DERPNodeFromProto(node)
	}

	return &tailcfg.DERPRegion{
		RegionID:      int(region.RegionId),
		EmbeddedRelay: region.EmbeddedRelay,
		RegionCode:    region.RegionCode,
		RegionName:    region.RegionName,
		Avoid:         region.Avoid,

		Nodes: regionNodes,
	}
}

func DERPNodeFromProto(node *proto.DERPMap_Region_Node) *tailcfg.DERPNode {
	if node == nil {
		return nil
	}

	return &tailcfg.DERPNode{
		Name:             node.Name,
		RegionID:         int(node.RegionId),
		HostName:         node.HostName,
		CertName:         node.CertName,
		IPv4:             node.Ipv4,
		IPv6:             node.Ipv6,
		STUNPort:         int(node.StunPort),
		STUNOnly:         node.StunOnly,
		DERPPort:         int(node.DerpPort),
		InsecureForTests: node.InsecureForTests,
		ForceHTTP:        node.ForceHttp,
		STUNTestIP:       node.StunTestIp,
		CanPort80:        node.CanPort_80,
	}
}

func WorkspaceStatusToProto(status codersdk.WorkspaceStatus) proto.Workspace_Status {
	switch status {
	case codersdk.WorkspaceStatusCanceled:
		return proto.Workspace_CANCELED
	case codersdk.WorkspaceStatusCanceling:
		return proto.Workspace_CANCELING
	case codersdk.WorkspaceStatusDeleted:
		return proto.Workspace_DELETED
	case codersdk.WorkspaceStatusDeleting:
		return proto.Workspace_DELETING
	case codersdk.WorkspaceStatusFailed:
		return proto.Workspace_FAILED
	case codersdk.WorkspaceStatusPending:
		return proto.Workspace_PENDING
	case codersdk.WorkspaceStatusRunning:
		return proto.Workspace_RUNNING
	case codersdk.WorkspaceStatusStarting:
		return proto.Workspace_STARTING
	case codersdk.WorkspaceStatusStopped:
		return proto.Workspace_STOPPED
	case codersdk.WorkspaceStatusStopping:
		return proto.Workspace_STOPPING
	default:
		return proto.Workspace_UNKNOWN
	}
}

type DERPFromDRPCWrapper struct {
	Client proto.DRPCTailnet_StreamDERPMapsClient
}

func (w *DERPFromDRPCWrapper) Close() error {
	return w.Client.Close()
}

func (w *DERPFromDRPCWrapper) Recv() (*tailcfg.DERPMap, error) {
	p, err := w.Client.Recv()
	if err != nil {
		return nil, err
	}
	return DERPMapFromProto(p), nil
}

var _ DERPClient = &DERPFromDRPCWrapper{}
