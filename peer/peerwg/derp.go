package peerwg

import "tailscale.com/tailcfg"

// This is currently set to use Tailscale's DERP server in DFW while we build in
// our own support for DERP servers.
var DerpMap = &tailcfg.DERPMap{
	Regions: map[int]*tailcfg.DERPRegion{
		9: {
			RegionID:   9,
			RegionCode: "dfw",
			RegionName: "Dallas",
			Avoid:      false,
			Nodes: []*tailcfg.DERPNode{
				{
					Name:             "9a",
					RegionID:         9,
					HostName:         "derp9.tailscale.com",
					CertName:         "",
					IPv4:             "207.148.3.137",
					IPv6:             "2001:19f0:6401:1d9c:5400:2ff:feef:bb82",
					STUNPort:         0,
					STUNOnly:         false,
					DERPPort:         0,
					InsecureForTests: false,
					STUNTestIP:       "",
				},
				{
					Name:             "9c",
					RegionID:         9,
					HostName:         "derp9c.tailscale.com",
					CertName:         "",
					IPv4:             "155.138.243.219",
					IPv6:             "2001:19f0:6401:fe7:5400:3ff:fe8d:6d9c",
					STUNPort:         0,
					STUNOnly:         false,
					DERPPort:         0,
					InsecureForTests: false,
					STUNTestIP:       "",
				},
				{
					Name:             "9b",
					RegionID:         9,
					HostName:         "derp9b.tailscale.com",
					CertName:         "",
					IPv4:             "144.202.67.195",
					IPv6:             "2001:19f0:6401:eb5:5400:3ff:fe8d:6d9b",
					STUNPort:         0,
					STUNOnly:         false,
					DERPPort:         0,
					InsecureForTests: false,
					STUNTestIP:       "",
				},
			},
		},
	},
	OmitDefaultRegions: true,
}

const DefaultDerpHome = "127.3.3.40:9"
