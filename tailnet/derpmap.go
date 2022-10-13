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
func NewDERPMap(ctx context.Context, region *tailcfg.DERPRegion, stunAddrs []string, remoteURL, localPath string) (*tailcfg.DERPMap, error) {
	if remoteURL != "" && localPath != "" {
		return nil, xerrors.New("a remote URL or local path must be specified, not both")
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
	return derpMap, nil
}
