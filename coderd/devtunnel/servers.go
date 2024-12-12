package devtunnel

import (
	"runtime"
	"sync"
	"time"

	ping "github.com/prometheus-community/pro-bing"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/cryptorand"
)

type Region struct {
	ID           int
	LocationName string
	Nodes        []Node
}

type Node struct {
	ID            int    `json:"id"`
	RegionID      int    `json:"region_id"`
	HostnameHTTPS string `json:"hostname_https"`

	AvgLatency time.Duration `json:"-"`
}

var Regions = []Region{
	{
		ID:           0,
		LocationName: "US East Pittsburgh",
		Nodes: []Node{
			{
				ID:            1,
				RegionID:      0,
				HostnameHTTPS: "pit-1.try.coder.app",
			},
		},
	},
}

// Nodes returns a list of nodes to use for the tunnel. It will pick a random
// node from each region.
//
// If a customNode is provided, it will be returned as the only node with ID
// 9999.
func Nodes(customTunnelHost string) ([]Node, error) {
	nodes := []Node{}

	if customTunnelHost != "" {
		return []Node{
			{
				ID:            9999,
				RegionID:      9999,
				HostnameHTTPS: customTunnelHost,
			},
		}, nil
	}

	for _, region := range Regions {
		// Pick a random node from each region.
		i, err := cryptorand.Intn(len(region.Nodes))
		if err != nil {
			return []Node{}, err
		}
		nodes = append(nodes, region.Nodes[i])
	}

	return nodes, nil
}

// FindClosestNode pings each node and returns the one with the lowest latency.
func FindClosestNode(nodes []Node) (Node, error) {
	if len(nodes) == 0 {
		return Node{}, xerrors.New("no wgtunnel nodes")
	}

	// Copy the nodes so we don't mutate the original.
	nodes = append([]Node{}, nodes...)

	var (
		nodesMu sync.Mutex
		eg      = errgroup.Group{}
	)
	for i, node := range nodes {
		i, node := i, node
		eg.Go(func() error {
			pinger, err := ping.NewPinger(node.HostnameHTTPS)
			if err != nil {
				return err
			}

			if runtime.GOOS == "windows" {
				pinger.SetPrivileged(true)
			}

			pinger.Count = 5
			pinger.Timeout = 5 * time.Second
			err = pinger.Run()
			if err != nil {
				return err
			}

			nodesMu.Lock()
			nodes[i].AvgLatency = pinger.Statistics().AvgRtt
			nodesMu.Unlock()
			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return Node{}, err
	}

	slices.SortFunc(nodes, func(a, b Node) int {
		return slice.Ascending(a.AvgLatency, b.AvgLatency)
	})
	return nodes[0], nil
}
