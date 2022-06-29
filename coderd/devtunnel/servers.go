package devtunnel

import (
	"runtime"
	"sync"
	"time"

	"github.com/go-ping/ping"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/cryptorand"
)

type TunnelRegion struct {
	ID           int
	LocationName string
	Nodes        []TunnelNode
}

type TunnelNode struct {
	ID                int    `json:"id"`
	HostnameHTTPS     string `json:"hostname_https"`
	HostnameWireguard string `json:"hostname_wireguard"`
	WireguardPort     uint16 `json:"wireguard_port"`

	AvgLatency time.Duration `json:"avg_latency"`
}

var TunnelRegions = []TunnelRegion{
	{
		ID:           1,
		LocationName: "US East Pittsburgh",
		Nodes: []TunnelNode{
			{
				ID:                1,
				HostnameHTTPS:     "pit-1.try.coder.app",
				HostnameWireguard: "pit-1.try.coder.app",
				WireguardPort:     55551,
			},
		},
	},
}

func PickTunnelNode() (TunnelNode, error) {
	nodes := []TunnelNode{}

	for _, region := range TunnelRegions {
		// Pick a random node from each region.
		i, err := cryptorand.Intn(len(region.Nodes))
		if err != nil {
			return TunnelNode{}, err
		}
		nodes = append(nodes, region.Nodes[i])
	}

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
		return TunnelNode{}, err
	}

	slices.SortFunc(nodes, func(i, j TunnelNode) bool {
		return i.AvgLatency < j.AvgLatency
	})
	return nodes[0], nil
}
