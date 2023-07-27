package tailnet

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

// NewDERPMap constructs a DERPMap from a set of STUN addresses and optionally a remote
// URL to fetch a mapping from e.g. https://controlplane.tailscale.com/derpmap/default.
//
//nolint:revive
func NewDERPMap(ctx context.Context, region *tailcfg.DERPRegion, stunAddrs []string, remoteURL, localPath string, disableSTUN bool) (*tailcfg.DERPMap, error) {
	if remoteURL != "" && localPath != "" {
		return nil, xerrors.New("a remote URL or local path must be specified, not both")
	}
	if disableSTUN {
		stunAddrs = nil
	}

	if region != nil {
		for index, stunAddr := range stunAddrs {
			host, rawPort, err := net.SplitHostPort(stunAddr)
			if err != nil {
				return nil, xerrors.Errorf("split host port for %q: %w", stunAddr, err)
			}
			port, err := strconv.Atoi(rawPort)
			if err != nil {
				return nil, xerrors.Errorf("parse port for %q: %w", stunAddr, err)
			}
			region.Nodes = append([]*tailcfg.DERPNode{{
				Name:     fmt.Sprintf("%dstun%d", region.RegionID, index),
				RegionID: region.RegionID,
				HostName: host,
				STUNOnly: true,
				STUNPort: port,
			}}, region.Nodes...)
		}
	}

	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{},
	}
	if remoteURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
		if err != nil {
			return nil, xerrors.Errorf("create request: %w", err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, xerrors.Errorf("get derpmap: %w", err)
		}
		defer res.Body.Close()
		err = json.NewDecoder(res.Body).Decode(&derpMap)
		if err != nil {
			return nil, xerrors.Errorf("fetch derpmap: %w", err)
		}
	}
	if localPath != "" {
		content, err := os.ReadFile(localPath)
		if err != nil {
			return nil, xerrors.Errorf("read derpmap from %q: %w", localPath, err)
		}
		err = json.Unmarshal(content, &derpMap)
		if err != nil {
			return nil, xerrors.Errorf("unmarshal derpmap: %w", err)
		}
	}
	if region != nil {
		_, conflicts := derpMap.Regions[region.RegionID]
		if conflicts {
			return nil, xerrors.Errorf("the default region ID conflicts with a remote region from %q", remoteURL)
		}
		derpMap.Regions[region.RegionID] = region
	}
	// Remove all STUNPorts from DERPy nodes, and fully remove all STUNOnly
	// nodes.
	if disableSTUN {
		for _, region := range derpMap.Regions {
			newNodes := make([]*tailcfg.DERPNode, 0, len(region.Nodes))
			for _, node := range region.Nodes {
				node.STUNPort = -1
				if !node.STUNOnly {
					newNodes = append(newNodes, node)
				}
			}
			region.Nodes = newNodes
		}
	}

	return derpMap, nil
}

// CompareDERPMaps returns true if the given DERPMaps are equivalent. Ordering
// of slices is ignored.
//
// If the first map is nil, the second map must also be nil for them to be
// considered equivalent. If the second map is nil, the first map can be any
// value and the function will return true.
func CompareDERPMaps(a *tailcfg.DERPMap, b *tailcfg.DERPMap) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return true
	}
	if len(a.Regions) != len(b.Regions) {
		return false
	}
	if a.OmitDefaultRegions != b.OmitDefaultRegions {
		return false
	}

	for id, region := range a.Regions {
		other, ok := b.Regions[id]
		if !ok {
			return false
		}
		if !compareDERPRegions(region, other) {
			return false
		}
	}
	return true
}

func compareDERPRegions(a *tailcfg.DERPRegion, b *tailcfg.DERPRegion) bool {
	if a == nil || b == nil {
		return false
	}
	if a.EmbeddedRelay != b.EmbeddedRelay {
		return false
	}
	if a.RegionID != b.RegionID {
		return false
	}
	if a.RegionCode != b.RegionCode {
		return false
	}
	if a.RegionName != b.RegionName {
		return false
	}
	if a.Avoid != b.Avoid {
		return false
	}
	if len(a.Nodes) != len(b.Nodes) {
		return false
	}

	// Convert both slices to maps so ordering can be ignored easier.
	aNodes := map[string]*tailcfg.DERPNode{}
	for _, node := range a.Nodes {
		aNodes[node.Name] = node
	}
	bNodes := map[string]*tailcfg.DERPNode{}
	for _, node := range b.Nodes {
		bNodes[node.Name] = node
	}

	for name, aNode := range aNodes {
		bNode, ok := bNodes[name]
		if !ok {
			return false
		}

		if aNode.Name != bNode.Name {
			return false
		}
		if aNode.RegionID != bNode.RegionID {
			return false
		}
		if aNode.HostName != bNode.HostName {
			return false
		}
		if aNode.CertName != bNode.CertName {
			return false
		}
		if aNode.IPv4 != bNode.IPv4 {
			return false
		}
		if aNode.IPv6 != bNode.IPv6 {
			return false
		}
		if aNode.STUNPort != bNode.STUNPort {
			return false
		}
		if aNode.STUNOnly != bNode.STUNOnly {
			return false
		}
		if aNode.DERPPort != bNode.DERPPort {
			return false
		}
		if aNode.InsecureForTests != bNode.InsecureForTests {
			return false
		}
		if aNode.ForceHTTP != bNode.ForceHTTP {
			return false
		}
		if aNode.STUNTestIP != bNode.STUNTestIP {
			return false
		}
	}

	return true
}
