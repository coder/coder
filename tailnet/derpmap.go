package tailnet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"
)

const DisableSTUN = "disable"

func STUNNode(regionID, index int, stunAddr string) (*tailcfg.DERPNode, error) {
	host, rawPort, err := net.SplitHostPort(stunAddr)
	if err != nil {
		return nil, xerrors.Errorf("split host port for %q: %w", stunAddr, err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return nil, xerrors.Errorf("parse port for %q: %w", stunAddr, err)
	}

	return &tailcfg.DERPNode{
		Name:     fmt.Sprintf("%dstun%d", regionID, index),
		RegionID: regionID,
		HostName: host,
		STUNOnly: true,
		STUNPort: port,
	}, nil
}

func STUNRegions(baseRegionID int, stunAddrs []string) ([]*tailcfg.DERPRegion, error) {
	regions := make([]*tailcfg.DERPRegion, 0, len(stunAddrs))
	for index, stunAddr := range stunAddrs {
		if stunAddr == DisableSTUN {
			return []*tailcfg.DERPRegion{}, nil
		}

		regionID := baseRegionID + index + 1
		node, err := STUNNode(regionID, 0, stunAddr)
		if err != nil {
			return nil, xerrors.Errorf("create stun node: %w", err)
		}

		regions = append(regions, &tailcfg.DERPRegion{
			EmbeddedRelay: false,
			RegionID:      regionID,
			RegionCode:    fmt.Sprintf("coder_stun_%d", regionID),
			RegionName:    fmt.Sprintf("Coder STUN %d", regionID),
			Nodes:         []*tailcfg.DERPNode{node},
		})
	}

	return regions, nil
}

// NewDERPMap constructs a DERPMap from a set of STUN addresses and optionally a remote
// URL to fetch a mapping from e.g. https://controlplane.tailscale.com/derpmap/default.
//
// stunAddrs is a list of STUN servers to add to the DERPMap. If the default
// region is nil, stunAddrs is ignored.
//
// individualSTUNRegions denotes whether to add a separate region for each STUN
// server or to add all STUN servers to the default region.
//
// disableSTUN will set stunAddrs to nil and remove any STUNPorts and STUNOnly
// nodes from the DERPMap.
//
// baseMapURL is a URL to fetch a base DERPMap from before applying the custom
// region and STUN servers. If the URL starts with "file:", the rest of the URL
// is treated as a file path and the file is opened locally. Otherwise, the URL
// is fetched via HTTP GET. Optional.
//
//nolint:revive
func NewDERPMap(ctx context.Context, region *tailcfg.DERPRegion, stunAddrs []string, disableSTUN bool, baseMapURL string) (*tailcfg.DERPMap, error) {
	if disableSTUN {
		stunAddrs = nil
	}

	// stunAddrs only applies when a default region is set. Each STUN node gets
	// it's own region ID because netcheck will only try a single STUN server in
	// each region before canceling the region's STUN check.
	addRegions := []*tailcfg.DERPRegion{}
	if region != nil {
		addRegions = append(addRegions, region)

		stunRegions, err := STUNRegions(region.RegionID, stunAddrs)
		if err != nil {
			return nil, xerrors.Errorf("create stun regions: %w", err)
		}
		addRegions = append(addRegions, stunRegions...)
	}

	derpMap := &tailcfg.DERPMap{
		Regions: map[int]*tailcfg.DERPRegion{},
	}

	// Fetch base DERP map if set.
	if baseMapURL != "" {
		var r io.Reader
		if strings.HasPrefix(baseMapURL, "file:") {
			f, err := os.Open(baseMapURL[5:])
			if err != nil {
				return nil, xerrors.Errorf("open file %q: %w", baseMapURL, err)
			}
			defer f.Close()
			r = f
		} else {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseMapURL, nil)
			if err != nil {
				return nil, xerrors.Errorf("create request to GET %q: %w", baseMapURL, err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, xerrors.Errorf("GET %q: %w", baseMapURL, err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				return nil, xerrors.Errorf("GET %q: invalid status code %s, expected 200", baseMapURL, res.Status)
			}
			r = res.Body
		}

		err := json.NewDecoder(r).Decode(&derpMap)
		if err != nil {
			return nil, xerrors.Errorf("fetch derpmap: %w", err)
		}
	}

	// Add our custom regions to the DERP map.
	if len(addRegions) > 0 {
		for _, region := range addRegions {
			_, conflicts := derpMap.Regions[region.RegionID]
			if conflicts {
				return nil, xerrors.Errorf("a default region ID %d (%s - %q) conflicts with a remote region from %q", region.RegionID, region.RegionCode, region.RegionName, baseMapURL)
			}
			derpMap.Regions[region.RegionID] = region
		}
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
