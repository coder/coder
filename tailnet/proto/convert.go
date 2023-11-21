package proto

import (
	"tailscale.com/tailcfg"
)

func DERPMapToProto(derpMap *tailcfg.DERPMap) *DERPMap {
	if derpMap == nil {
		return nil
	}

	regionScore := make(map[int64]float64)
	if derpMap.HomeParams != nil {
		for k, v := range derpMap.HomeParams.RegionScore {
			regionScore[int64(k)] = v
		}
	}

	regions := make(map[int64]*DERPMap_Region, len(derpMap.Regions))
	for regionID, region := range derpMap.Regions {
		regions[int64(regionID)] = DERPRegionToProto(region)
	}

	return &DERPMap{
		HomeParams: &DERPMap_HomeParams{
			RegionScore: regionScore,
		},
		Regions: regions,
	}
}

func DERPRegionToProto(region *tailcfg.DERPRegion) *DERPMap_Region {
	if region == nil {
		return nil
	}

	regionNodes := make([]*DERPMap_Region_Node, len(region.Nodes))
	for i, node := range region.Nodes {
		regionNodes[i] = DERPNodeToProto(node)
	}

	return &DERPMap_Region{
		RegionId:      int64(region.RegionID),
		EmbeddedRelay: region.EmbeddedRelay,
		RegionCode:    region.RegionCode,
		RegionName:    region.RegionName,
		Avoid:         region.Avoid,

		Nodes: regionNodes,
	}
}

func DERPNodeToProto(node *tailcfg.DERPNode) *DERPMap_Region_Node {
	if node == nil {
		return nil
	}

	return &DERPMap_Region_Node{
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

func DERPMapFromProto(derpMap *DERPMap) *tailcfg.DERPMap {
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

func DERPRegionFromProto(region *DERPMap_Region) *tailcfg.DERPRegion {
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

func DERPNodeFromProto(node *DERPMap_Region_Node) *tailcfg.DERPNode {
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
