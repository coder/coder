//go:build linux
// +build linux

package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tailscale/netlink"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/cryptorand"
)

const (
	client1Port           = 48001
	client1RouterPort     = 48011 // used in easy and hard NAT
	client1RouterPortSTUN = 48201 // used in hard NAT
	client2Port           = 48002
	client2RouterPort     = 48012 // used in easy and hard NAT
	client2RouterPortSTUN = 48101 // used in hard NAT
)

type TestNetworking struct {
	Server  TestNetworkingServer
	STUNs   []TestNetworkingSTUN
	Client1 TestNetworkingClient
	Client2 TestNetworkingClient
}

type TestNetworkingServer struct {
	Process    TestNetworkingProcess
	ListenAddr string
}

type TestNetworkingSTUN struct {
	Process    TestNetworkingProcess
	IP         string
	ListenAddr string
}

type TestNetworkingClient struct {
	Process TestNetworkingProcess
	// ServerAccessURL is the hostname and port that the client uses to access
	// the server over HTTP for coordination.
	ServerAccessURL string
	// DERPMap is the DERP map that the client uses. If nil, a basic DERP map
	// containing only a single DERP with `ServerAccessURL` is used with no
	// STUN servers.
	DERPMap *tailcfg.DERPMap
}

func (c TestNetworkingClient) ResolveDERPMap() (*tailcfg.DERPMap, error) {
	if c.DERPMap != nil {
		return c.DERPMap, nil
	}

	return basicDERPMap(c.ServerAccessURL)
}

type TestNetworkingProcess struct {
	// NetNS to enter. If nil, the current network namespace is used.
	NetNS *os.File
}

func (p TestNetworkingProcess) CapturePackets(t *testing.T, name, dir string) {
	dumpfile := path.Join(dir, name+".pcap")
	_, _ = ExecBackground(t, name+".pcap", p.NetNS, "tcpdump", []string{
		"-i", "any",
		"-w", dumpfile,
	})
}

// NetworkingLoopback creates a network namespace with a loopback interface
// for all tests to share. This is the simplest networking setup. The network
// namespace only exists for isolation on the host and doesn't serve any routing
// purpose.
type NetworkingLoopback struct{}

func (NetworkingLoopback) SetupNetworking(t *testing.T, _ slog.Logger) TestNetworking {
	// Create a single network namespace for all tests so we can have an
	// isolated loopback interface.
	netNSFile := createNetNS(t, uniqNetName(t))

	var (
		listenAddr = "127.0.0.1:8080"
		process    = TestNetworkingProcess{
			NetNS: netNSFile,
		}
	)
	return TestNetworking{
		Server: TestNetworkingServer{
			Process:    process,
			ListenAddr: listenAddr,
		},
		Client1: TestNetworkingClient{
			Process:         process,
			ServerAccessURL: "http://" + listenAddr,
		},
		Client2: TestNetworkingClient{
			Process:         process,
			ServerAccessURL: "http://" + listenAddr,
		},
	}
}

// NetworkingNAT creates a fake internet and sets up "NAT"
// forwarding rules, either easy or hard.
// See createFakeInternet.
// NAT is achieved through a single iptables masquerade rule.
type NetworkingNAT struct {
	StunCount   int
	Client1Hard bool
	Client2Hard bool
}

// SetupNetworking creates a fake internet with multiple STUN servers and sets up
// NAT forwarding rules. Client NATs are controlled by the switches ClientXHard, which if true, sets up hard
// nat.
func (n NetworkingNAT) SetupNetworking(t *testing.T, l slog.Logger) TestNetworking {
	logger := l.Named("setup-networking").Leveled(slog.LevelDebug)
	internet := createFakeInternet(t)
	logger.Debug(context.Background(), "preparing STUN", slog.F("stun_count", n.StunCount))
	internet.Net.STUNs = make([]TestNetworkingSTUN, n.StunCount)
	for i := 0; i < n.StunCount; i++ {
		internet.Net.STUNs[i] = prepareSTUNServer(t, &internet, i)
	}

	_, err := commandInNetNS(internet.BridgeNetNS, "sysctl", []string{"-w", "net.ipv4.ip_forward=1"}).Output()
	require.NoError(t, wrapExitErr(err), "enable IP forwarding in bridge NetNS")

	// Set up iptables masquerade rules to allow each router to NAT packets.
	leaves := []struct {
		fakeRouterLeaf
		peerIP           string
		clientPort       int
		natPortPeer      int
		natStartPortSTUN int
	}{
		{
			fakeRouterLeaf: internet.Client1,
			// If peerIP is empty, we do easy NAT (even for STUN)
			peerIP: func() string {
				if n.Client1Hard {
					return internet.Client2.RouterIP
				}
				return ""
			}(),
			clientPort:       client1Port,
			natPortPeer:      client1RouterPort,
			natStartPortSTUN: client1RouterPortSTUN,
		},
		{
			fakeRouterLeaf: internet.Client2,
			// If peerIP is empty, we do easy NAT (even for STUN)
			peerIP: func() string {
				if n.Client2Hard {
					return internet.Client1.RouterIP
				}
				return ""
			}(),
			clientPort:       client2Port,
			natPortPeer:      client2RouterPort,
			natStartPortSTUN: client2RouterPortSTUN,
		},
	}
	for _, leaf := range leaves {
		_, err := commandInNetNS(leaf.RouterNetNS, "sysctl", []string{"-w", "net.ipv4.ip_forward=1"}).Output()
		require.NoError(t, wrapExitErr(err), "enable IP forwarding in router NetNS")

		// All non-UDP traffic should use regular masquerade e.g. for HTTP.
		iptablesMasqueradeNonUDP(t, leaf.RouterNetNS)

		// NAT from this client to its peer.
		iptablesNAT(t, leaf.RouterNetNS, leaf.ClientIP, leaf.clientPort, leaf.RouterIP, leaf.natPortPeer, leaf.peerIP)

		// NAT from this client to each STUN server. Only do this if we're doing
		// hard NAT, as the rule above will also touch STUN traffic in easy NAT.
		if leaf.peerIP != "" {
			logger.Debug(context.Background(), "creating NAT to STUN",
				slog.F("client_ip", leaf.ClientIP), slog.F("peer_ip", leaf.peerIP),
			)
			for i, stun := range internet.Net.STUNs {
				natPort := leaf.natStartPortSTUN + i
				iptablesNAT(t, leaf.RouterNetNS, leaf.ClientIP, leaf.clientPort, leaf.RouterIP, natPort, stun.IP)
			}
		}
	}

	return internet.Net
}

type vethPair struct {
	Outer string
	Inner string
}

type fakeRouterLeaf struct {
	// RouterIP is the IP address of the router on the bridge.
	RouterIP string
	// ClientIP is the IP address of the client on the router.
	ClientIP string
	// RouterNetNS is the router for this specific leaf.
	RouterNetNS *os.File
	// ClientNetNS is where the "user" is.
	ClientNetNS *os.File
	// Veth pair between the router and the bridge.
	OuterVethPair vethPair
	// Veth pair between the user and the router.
	InnerVethPair vethPair
}

type fakeInternet struct {
	Net TestNetworking

	NamePrefix     string
	BridgeNetNS    *os.File
	BridgeName     string
	ServerNetNS    *os.File
	ServerVethPair vethPair // between bridge and server NS
	Client1        fakeRouterLeaf
	Client2        fakeRouterLeaf
}

// createFakeInternet creates multiple namespaces with veth pairs between them
// with the following topology:
//
// .                    veth   ┌────────┐   veth
// .         ┌─────────────────┤ Bridge ├───────────────────┐
// .         │                 └───┬────┘                   │
// .         │                     │                        │
// .         │10.0.0.1         veth│10.0.0.2                │10.0.0.3
// . ┌───────┴───────┐     ┌───────┴─────────┐     ┌────────┴────────┐
// . │    Server     │     │ Client 1 router │     │ Client 2 router │
// . └───────────────┘     └───────┬─────────┘     └────────┬────────┘
// .                               │10.0.2.1                │10.0.3.1
// .                           veth│                    veth│
// .                               │10.0.2.2                │10.0.3.2
// .                       ┌───────┴─────────┐     ┌────────┴────────┐
// .                       │    Client 1     │     │    Client 2     │
// .                       └─────────────────┘     └─────────────────┘
//
// No iptables rules are created, so packets will not be forwarded out of the
// box. Default routes are created from the edge namespaces (client1, client2)
// to their respective routers, but no NAT rules are created.
func createFakeInternet(t *testing.T) fakeInternet {
	t.Helper()
	const (
		bridgePrefix  = "10.0.0."
		serverIP      = bridgePrefix + "1"
		client1Prefix = "10.0.2."
		client2Prefix = "10.0.3."
	)
	var (
		namePrefix = uniqNetName(t) + "_"
		router     = fakeInternet{
			NamePrefix: namePrefix,
			BridgeName: namePrefix + "b",
		}
	)

	// Create bridge namespace and bridge interface.
	router.BridgeNetNS = createNetNS(t, router.BridgeName)
	err := createBridge(router.BridgeNetNS, router.BridgeName)
	require.NoError(t, err, "create bridge in netns")

	// Create server namespace and veth pair between bridge and server.
	router.ServerNetNS = createNetNS(t, namePrefix+"s")
	router.ServerVethPair = vethPair{
		Outer: namePrefix + "b-s",
		Inner: namePrefix + "s-b",
	}
	err = joinBridge(joinBridgeOpts{
		bridgeNetNS: router.BridgeNetNS,
		netNS:       router.ServerNetNS,
		bridgeName:  router.BridgeName,
		vethPair:    router.ServerVethPair,
		ip:          serverIP,
	})
	require.NoError(t, err, "join bridge with server")

	leaves := []struct {
		leaf           *fakeRouterLeaf
		routerName     string
		clientName     string
		routerBridgeIP string
		routerClientIP string
		clientIP       string
	}{
		{
			leaf:           &router.Client1,
			routerName:     "c1r",
			clientName:     "c1",
			routerBridgeIP: bridgePrefix + "2",
			routerClientIP: client1Prefix + "1",
			clientIP:       client1Prefix + "2",
		},
		{
			leaf:           &router.Client2,
			routerName:     "c2r",
			clientName:     "c2",
			routerBridgeIP: bridgePrefix + "3",
			routerClientIP: client2Prefix + "1",
			clientIP:       client2Prefix + "2",
		},
	}

	for _, leaf := range leaves {
		leaf.leaf.RouterIP = leaf.routerBridgeIP
		leaf.leaf.ClientIP = leaf.clientIP

		// Create two network namespaces for each leaf: one for the router and
		// one for the "client".
		leaf.leaf.RouterNetNS = createNetNS(t, namePrefix+leaf.routerName)
		leaf.leaf.ClientNetNS = createNetNS(t, namePrefix+leaf.clientName)

		// Join the bridge.
		leaf.leaf.OuterVethPair = vethPair{
			Outer: namePrefix + "b-" + leaf.routerName,
			Inner: namePrefix + leaf.routerName + "-b",
		}
		err = joinBridge(joinBridgeOpts{
			bridgeNetNS: router.BridgeNetNS,
			netNS:       leaf.leaf.RouterNetNS,
			bridgeName:  router.BridgeName,
			vethPair:    leaf.leaf.OuterVethPair,
			ip:          leaf.routerBridgeIP,
		})
		require.NoError(t, err, "join bridge with router")

		// Create inner veth pair between the router and the client.
		leaf.leaf.InnerVethPair = vethPair{
			Outer: namePrefix + leaf.routerName + "-" + leaf.clientName,
			Inner: namePrefix + leaf.clientName + "-" + leaf.routerName,
		}
		err = createVethPair(leaf.leaf.InnerVethPair.Outer, leaf.leaf.InnerVethPair.Inner)
		require.NoErrorf(t, err, "create veth pair %q <-> %q", leaf.leaf.InnerVethPair.Outer, leaf.leaf.InnerVethPair.Inner)

		// Move the network interfaces to the respective network namespaces.
		err = setVethNetNS(leaf.leaf.InnerVethPair.Outer, int(leaf.leaf.RouterNetNS.Fd()))
		require.NoErrorf(t, err, "set veth %q to NetNS", leaf.leaf.InnerVethPair.Outer)
		err = setVethNetNS(leaf.leaf.InnerVethPair.Inner, int(leaf.leaf.ClientNetNS.Fd()))
		require.NoErrorf(t, err, "set veth %q to NetNS", leaf.leaf.InnerVethPair.Inner)

		// Set router's "local" IP on the veth.
		err = setInterfaceIP(leaf.leaf.RouterNetNS, leaf.leaf.InnerVethPair.Outer, leaf.routerClientIP)
		require.NoErrorf(t, err, "set IP %q on interface %q", leaf.routerClientIP, leaf.leaf.InnerVethPair.Outer)
		// Set client's IP on the veth.
		err = setInterfaceIP(leaf.leaf.ClientNetNS, leaf.leaf.InnerVethPair.Inner, leaf.clientIP)
		require.NoErrorf(t, err, "set IP %q on interface %q", leaf.clientIP, leaf.leaf.InnerVethPair.Inner)

		// Bring up the interfaces.
		err = setInterfaceUp(leaf.leaf.RouterNetNS, leaf.leaf.InnerVethPair.Outer)
		require.NoErrorf(t, err, "bring up interface %q", leaf.leaf.OuterVethPair.Outer)
		err = setInterfaceUp(leaf.leaf.ClientNetNS, leaf.leaf.InnerVethPair.Inner)
		require.NoErrorf(t, err, "bring up interface %q", leaf.leaf.InnerVethPair.Inner)

		// We don't need to add a route from parent to peer since the kernel
		// already adds a default route for the /24. We DO need to add a default
		// route from peer to parent, however.
		err = addRouteInNetNS(leaf.leaf.ClientNetNS, []string{"default", "via", leaf.routerClientIP, "dev", leaf.leaf.InnerVethPair.Inner})
		require.NoErrorf(t, err, "add peer default route to %q", leaf.leaf.InnerVethPair.Inner)
	}

	router.Net = TestNetworking{
		Server: TestNetworkingServer{
			Process:    TestNetworkingProcess{NetNS: router.ServerNetNS},
			ListenAddr: serverIP + ":8080",
		},
		Client1: TestNetworkingClient{
			Process:         TestNetworkingProcess{NetNS: router.Client1.ClientNetNS},
			ServerAccessURL: "http://" + serverIP + ":8080",
		},
		Client2: TestNetworkingClient{
			Process:         TestNetworkingProcess{NetNS: router.Client2.ClientNetNS},
			ServerAccessURL: "http://" + serverIP + ":8080",
		},
	}
	return router
}

type TriangleNetwork struct {
	Client1MTU int
}

type fakeTriangleNetwork struct {
	NamePrefix      string
	ServerNetNS     *os.File
	Client1NetNS    *os.File
	Client2NetNS    *os.File
	RouterNetNS     *os.File
	ServerVethPair  vethPair
	Client1VethPair vethPair
	Client2VethPair vethPair
}

// SetupNetworking creates multiple namespaces with a central router in the following topology
// .
// .   ┌──────────────┐
// .   │              │
// .   │  Server      ├─────────────────────────────────────┐
// .   │              │fdac:38fa:ffff:3::2                  │
// .   └──────────────┘                                     │ fdac:38fa:ffff:3::1
// .   ┌──────────────┐                               ┌─────┴───────┐
// .   │              │            fdac:38fa:ffff:1::1│             │
// .   │  Client 1    ├───────────────────────────────┤  Router     │
// .   │              │fdac:38fa:ffff:1::2            │             │
// .   └──────────────┘                               └─────┬───────┘
// .   ┌──────────────┐                                     │ fdac:38fa:ffff:2::1
// .   │              │                                     │
// .   │  Client 2    ├─────────────────────────────────────┘
// .   │              │fdac:38fa:ffff:2::2
// .   └──────────────┘
// The veth link between Client 1 and the router has a configurable MTU via Client1MTU.
func (n TriangleNetwork) SetupNetworking(t *testing.T, l slog.Logger) TestNetworking {
	logger := l.Named("setup-networking").Leveled(slog.LevelDebug)
	t.Helper()
	var (
		namePrefix = uniqNetName(t) + "_"
		network    = fakeTriangleNetwork{
			NamePrefix: namePrefix,
		}
		// Unique Local Address prefix
		ula = "fdac:38fa:ffff:"
	)

	// Create three network namespaces for server, client1, and client2
	network.ServerNetNS = createNetNS(t, namePrefix+"server")
	network.Client1NetNS = createNetNS(t, namePrefix+"client1")
	network.Client2NetNS = createNetNS(t, namePrefix+"client2")
	network.RouterNetNS = createNetNS(t, namePrefix+"router")

	// Create veth pair between server and router
	network.ServerVethPair = vethPair{
		Outer: namePrefix + "s-r",
		Inner: namePrefix + "r-s",
	}
	err := createVethPair(network.ServerVethPair.Outer, network.ServerVethPair.Inner)
	require.NoErrorf(t, err, "create veth pair %q <-> %q",
		network.ServerVethPair.Outer, network.ServerVethPair.Inner)

	// Move server-router veth ends to their respective namespaces
	err = setVethNetNS(network.ServerVethPair.Outer, int(network.ServerNetNS.Fd()))
	require.NoErrorf(t, err, "set veth %q to server NetNS", network.ServerVethPair.Outer)
	err = setVethNetNS(network.ServerVethPair.Inner, int(network.RouterNetNS.Fd()))
	require.NoErrorf(t, err, "set veth %q to router NetNS", network.ServerVethPair.Inner)

	// Create veth pair between client1 and router
	network.Client1VethPair = vethPair{
		Outer: namePrefix + "1-r",
		Inner: namePrefix + "r-1",
	}
	logger.Debug(context.Background(), "creating client1 link", slog.F("mtu", n.Client1MTU))
	err = createVethPair(network.Client1VethPair.Outer, network.Client1VethPair.Inner, withMTU(n.Client1MTU))
	require.NoErrorf(t, err, "create veth pair %q <-> %q",
		network.Client1VethPair.Outer, network.Client1VethPair.Inner)

	// Move client1-router veth ends to their respective namespaces
	err = setVethNetNS(network.Client1VethPair.Outer, int(network.Client1NetNS.Fd()))
	require.NoErrorf(t, err, "set veth %q to server NetNS", network.Client1VethPair.Outer)
	err = setVethNetNS(network.Client1VethPair.Inner, int(network.RouterNetNS.Fd()))
	require.NoErrorf(t, err, "set veth %q to client2 NetNS", network.Client1VethPair.Inner)

	// Create veth pair between client1 and client2
	network.Client2VethPair = vethPair{
		Outer: namePrefix + "2-r",
		Inner: namePrefix + "r-2",
	}

	err = createVethPair(network.Client2VethPair.Outer, network.Client2VethPair.Inner)
	require.NoErrorf(t, err, "create veth pair %q <-> %q",
		network.Client2VethPair.Outer, network.Client2VethPair.Inner)

	// Move client1-client2 veth ends to their respective namespaces
	err = setVethNetNS(network.Client2VethPair.Outer, int(network.Client2NetNS.Fd()))
	require.NoErrorf(t, err, "set veth %q to client1 NetNS", network.Client2VethPair.Outer)
	err = setVethNetNS(network.Client2VethPair.Inner, int(network.RouterNetNS.Fd()))
	require.NoErrorf(t, err, "set veth %q to client2 NetNS", network.Client2VethPair.Inner)

	// Set IP addresses according to the diagram:
	err = setInterfaceIP6(network.ServerNetNS, network.ServerVethPair.Outer, ula+"3::2")
	require.NoErrorf(t, err, "set IP on server interface")
	err = setInterfaceIP6(network.Client1NetNS, network.Client1VethPair.Outer, ula+"1::2")
	require.NoErrorf(t, err, "set IP on client1 interface")
	err = setInterfaceIP6(network.Client2NetNS, network.Client2VethPair.Outer, ula+"2::2")
	require.NoErrorf(t, err, "set IP on client2 interface")

	err = setInterfaceIP6(network.RouterNetNS, network.ServerVethPair.Inner, ula+"3::1")
	require.NoErrorf(t, err, "set IP on router-server interface")
	err = setInterfaceIP6(network.RouterNetNS, network.Client1VethPair.Inner, ula+"1::1")
	require.NoErrorf(t, err, "set IP on router-client1 interface")
	err = setInterfaceIP6(network.RouterNetNS, network.Client2VethPair.Inner, ula+"2::1")
	require.NoErrorf(t, err, "set IP on router-client2 interface")

	// Bring up all interfaces
	interfaces := []struct {
		netNS        *os.File
		ifaceName    string
		defaultRoute string
	}{
		{network.ServerNetNS, network.ServerVethPair.Outer, ula + "3::1"},
		{network.Client1NetNS, network.Client1VethPair.Outer, ula + "1::1"},
		{network.Client2NetNS, network.Client2VethPair.Outer, ula + "2::1"},
		{network.RouterNetNS, network.ServerVethPair.Inner, ""},
		{network.RouterNetNS, network.Client1VethPair.Inner, ""},
		{network.RouterNetNS, network.Client2VethPair.Inner, ""},
	}
	for _, iface := range interfaces {
		err = setInterfaceUp(iface.netNS, iface.ifaceName)
		require.NoErrorf(t, err, "bring up interface %q", iface.ifaceName)

		if iface.defaultRoute != "" {
			err = addRouteInNetNS(iface.netNS, []string{"default", "via", iface.defaultRoute, "dev", iface.ifaceName})
			require.NoErrorf(t, err, "add peer default route to %s", iface.defaultRoute)
		}
	}

	// enable IP forwarding in the router
	_, err = commandInNetNS(network.RouterNetNS, "sysctl", []string{"-w", "net.ipv6.conf.all.forwarding=1"}).Output()
	require.NoError(t, wrapExitErr(err), "enable IPv6 forwarding in router NetNS")

	return TestNetworking{
		Server: TestNetworkingServer{
			Process:    TestNetworkingProcess{NetNS: network.ServerNetNS},
			ListenAddr: "[::]:8080", // Server listens on all IPs
		},
		Client1: TestNetworkingClient{
			Process:         TestNetworkingProcess{NetNS: network.Client1NetNS},
			ServerAccessURL: "http://[" + ula + "3::2]:8080",
		},
		Client2: TestNetworkingClient{
			Process:         TestNetworkingProcess{NetNS: network.Client2NetNS},
			ServerAccessURL: "http://[" + ula + "3::2]:8080",
		},
	}
}

func uniqNetName(t *testing.T) string {
	t.Helper()
	netNSName := "cdr_"
	randStr, err := cryptorand.String(3)
	require.NoError(t, err, "generate random string for netns name")
	netNSName += randStr
	return netNSName
}

type joinBridgeOpts struct {
	bridgeNetNS *os.File
	netNS       *os.File
	bridgeName  string
	// This vethPair will be created and should not already exist.
	vethPair vethPair
	ip       string
}

// joinBridge joins the given network namespace to the bridge. It creates a veth
// pair between the specified NetNS and the bridge NetNS, sets the IP address on
// the "child" veth, and brings up the interfaces.
func joinBridge(opts joinBridgeOpts) error {
	// Create outer veth pair between the router and the bridge.
	err := createVethPair(opts.vethPair.Outer, opts.vethPair.Inner)
	if err != nil {
		return xerrors.Errorf("create veth pair %q <-> %q: %w", opts.vethPair.Outer, opts.vethPair.Inner, err)
	}

	// Move the network interfaces to the respective network namespaces.
	err = setVethNetNS(opts.vethPair.Outer, int(opts.bridgeNetNS.Fd()))
	if err != nil {
		return xerrors.Errorf("set veth %q to NetNS: %w", opts.vethPair.Outer, err)
	}
	err = setVethNetNS(opts.vethPair.Inner, int(opts.netNS.Fd()))
	if err != nil {
		return xerrors.Errorf("set veth %q to NetNS: %w", opts.vethPair.Inner, err)
	}

	// Connect the outer veth to the bridge.
	err = setInterfaceBridge(opts.bridgeNetNS, opts.vethPair.Outer, opts.bridgeName)
	if err != nil {
		return xerrors.Errorf("set interface %q master to %q: %w", opts.vethPair.Outer, opts.bridgeName, err)
	}

	// Set the bridge IP on the inner veth.
	err = setInterfaceIP(opts.netNS, opts.vethPair.Inner, opts.ip)
	if err != nil {
		return xerrors.Errorf("set IP %q on interface %q: %w", opts.ip, opts.vethPair.Inner, err)
	}

	// Bring up the interfaces.
	err = setInterfaceUp(opts.bridgeNetNS, opts.vethPair.Outer)
	if err != nil {
		return xerrors.Errorf("bring up interface %q: %w", opts.vethPair.Outer, err)
	}
	err = setInterfaceUp(opts.netNS, opts.vethPair.Inner)
	if err != nil {
		return xerrors.Errorf("bring up interface %q: %w", opts.vethPair.Inner, err)
	}

	return nil
}

// createNetNS creates a new network namespace with the given name. The returned
// file is a file descriptor to the network namespace.
// Note: all cleanup is handled for you, you do not need to call Close on the
// returned file.
func createNetNS(t *testing.T, name string) *os.File {
	// We use ip-netns here because it handles the process of creating a
	// disowned netns for us.
	// The only way to create a network namespace is by calling unshare(2) or
	// clone(2) with the CLONE_NEWNET flag, and as soon as the last process in a
	// network namespace exits, the namespace is destroyed.
	// However, if you create a bind mount of /proc/$PID/ns/net to a file, it
	// will keep the namespace alive until the mount is removed.
	// ip-netns does this for us. Without it, we would have to fork anyways.
	// Later, we will use nsenter to enter this network namespace.
	_, err := exec.Command("ip", "netns", "add", name).Output()
	require.NoError(t, wrapExitErr(err), "create network namespace via ip-netns")
	t.Cleanup(func() {
		_, _ = exec.Command("ip", "netns", "delete", name).Output()
	})

	// Open /run/netns/$name to get a file descriptor to the network namespace.
	netnsPath := fmt.Sprintf("/run/netns/%s", name)
	file, err := os.OpenFile(netnsPath, os.O_RDONLY, 0)
	require.NoError(t, err, "open network namespace file")
	t.Cleanup(func() {
		_ = file.Close()
	})

	// Exec "ip link set lo up" in the namespace to bring up loopback
	// networking.
	//nolint:gosec
	_, err = exec.Command("ip", "-netns", name, "link", "set", "lo", "up").Output()
	require.NoError(t, wrapExitErr(err), "bring up loopback interface in network namespace")

	return file
}

// createBridge creates a bridge in the given network namespace. The bridge is
// automatically brought up.
func createBridge(netNS *os.File, name string) error {
	// While it might be possible to create a bridge directly in a NetNS or move
	// an existing bridge to a NetNS, I couldn't figure out a way to do it.
	// Creating it directly within the NetNS is the simplest way.
	_, err := commandInNetNS(netNS, "ip", []string{"link", "add", name, "type", "bridge"}).Output()
	if err != nil {
		return xerrors.Errorf("create bridge %q in netns: %w", name, wrapExitErr(err))
	}

	_, err = commandInNetNS(netNS, "ip", []string{"link", "set", name, "up"}).Output()
	if err != nil {
		return xerrors.Errorf("set bridge %q up in netns: %w", name, wrapExitErr(err))
	}

	return nil
}

// setInterfaceBridge sets the master of the given interface to the specified
// bridge.
func setInterfaceBridge(netNS *os.File, ifaceName, bridgeName string) error {
	_, err := commandInNetNS(netNS, "ip", []string{"link", "set", ifaceName, "master", bridgeName}).Output()
	if err != nil {
		return xerrors.Errorf("set interface %q master to %q in netns: %w", ifaceName, bridgeName, wrapExitErr(err))
	}

	return nil
}

type linkOption func(attrs netlink.LinkAttrs) netlink.LinkAttrs

func withMTU(mtu int) linkOption {
	return func(attrs netlink.LinkAttrs) netlink.LinkAttrs {
		attrs.MTU = mtu
		return attrs
	}
}

// createVethPair creates a veth pair with the given names.
func createVethPair(parentVethName, peerVethName string, options ...linkOption) error {
	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = parentVethName
	for _, option := range options {
		linkAttrs = option(linkAttrs)
	}
	veth := &netlink.Veth{
		LinkAttrs: linkAttrs,
		PeerName:  peerVethName,
	}

	err := netlink.LinkAdd(veth)
	if err != nil {
		return xerrors.Errorf("LinkAdd(type: veth, name: %q, peerName: %q): %w", parentVethName, peerVethName, err)
	}

	return nil
}

// setVethNetNS moves the veth interface to the specified network namespace.
func setVethNetNS(vethName string, netNSFd int) error {
	veth, err := netlink.LinkByName(vethName)
	if err != nil {
		return xerrors.Errorf("LinkByName(%q): %w", vethName, err)
	}

	err = netlink.LinkSetNsFd(veth, netNSFd)
	if err != nil {
		return xerrors.Errorf("LinkSetNsFd(%q, %v): %w", vethName, netNSFd, err)
	}

	return nil
}

// setInterfaceIP sets the IP address on the given interface. It automatically
// adds a /24 subnet mask.
func setInterfaceIP(netNS *os.File, ifaceName, ip string) error {
	_, err := commandInNetNS(netNS, "ip", []string{"addr", "add", ip + "/24", "dev", ifaceName}).Output()
	if err != nil {
		return xerrors.Errorf("set IP %q on interface %q in netns: %w", ip, ifaceName, wrapExitErr(err))
	}

	return nil
}

// setInterfaceIP6 sets the IPv6 address on the given interface. It automatically
// adds a /64 subnet mask.
func setInterfaceIP6(netNS *os.File, ifaceName, ip string) error {
	_, err := commandInNetNS(netNS, "ip", []string{"addr", "add", ip + "/64", "dev", ifaceName}).Output()
	if err != nil {
		return xerrors.Errorf("set IP %q on interface %q in netns: %w", ip, ifaceName, wrapExitErr(err))
	}

	return nil
}

// setInterfaceUp brings the given interface up.
func setInterfaceUp(netNS *os.File, ifaceName string) error {
	_, err := commandInNetNS(netNS, "ip", []string{"link", "set", ifaceName, "up"}).Output()
	if err != nil {
		return xerrors.Errorf("bring up interface %q in netns: %w", ifaceName, wrapExitErr(err))
	}

	return nil
}

// addRouteInNetNS adds a route to the given network namespace.
func addRouteInNetNS(netNS *os.File, route []string) error {
	_, err := commandInNetNS(netNS, "ip", append([]string{"route", "add"}, route...)).Output()
	if err != nil {
		return xerrors.Errorf("add route %q in netns: %w", route, wrapExitErr(err))
	}

	return nil
}

// prepareSTUNServer creates a STUN server networking spec in a network
// namespace and joins it to the bridge. It also sets up the DERP map for the
// clients to use the STUN.
func prepareSTUNServer(t *testing.T, internet *fakeInternet, number int) TestNetworkingSTUN {
	name := fmt.Sprintf("stn%d", number)

	stunNetNS := createNetNS(t, internet.NamePrefix+name)
	stun := TestNetworkingSTUN{
		Process: TestNetworkingProcess{
			NetNS: stunNetNS,
		},
	}

	stun.IP = "10.0.0." + fmt.Sprint(64+number)
	err := joinBridge(joinBridgeOpts{
		bridgeNetNS: internet.BridgeNetNS,
		netNS:       stunNetNS,
		bridgeName:  internet.BridgeName,
		vethPair: vethPair{
			Outer: internet.NamePrefix + "b-" + name,
			Inner: internet.NamePrefix + name + "-b",
		},
		ip: stun.IP,
	})
	require.NoError(t, err, "join bridge with STUN server")
	stun.ListenAddr = stun.IP + ":3478"

	// Define custom DERP map.
	stunRegion := &tailcfg.DERPRegion{
		RegionID:   10000 + number,
		RegionCode: name,
		RegionName: name,
		Nodes: []*tailcfg.DERPNode{
			{
				Name:     name + "a",
				RegionID: 1,
				IPv4:     stun.IP,
				IPv6:     "none",
				STUNPort: 3478,
				STUNOnly: true,
			},
		},
	}
	client1DERP, err := internet.Net.Client1.ResolveDERPMap()
	require.NoError(t, err, "resolve DERP map for client 1")
	client1DERP.Regions[stunRegion.RegionID] = stunRegion
	internet.Net.Client1.DERPMap = client1DERP
	client2DERP, err := internet.Net.Client2.ResolveDERPMap()
	require.NoError(t, err, "resolve DERP map for client 2")
	client2DERP.Regions[stunRegion.RegionID] = stunRegion
	internet.Net.Client2.DERPMap = client2DERP

	return stun
}

func iptablesMasqueradeNonUDP(t *testing.T, netNS *os.File) {
	t.Helper()
	_, err := commandInNetNS(netNS, "iptables", []string{
		"-t", "nat",
		"-A", "POSTROUTING",
		// Every interface except loopback.
		"!", "-o", "lo",
		// Every protocol except UDP.
		"!", "-p", "udp",
		"-j", "MASQUERADE",
	}).Output()
	require.NoError(t, wrapExitErr(err), "add iptables non-UDP masquerade rule")
}

// iptablesNAT sets up iptables rules for NAT forwarding. If destIP is
// specified, the forwarding rule will only apply to traffic to/from that IP
// (mapvarydest).
func iptablesNAT(
	t *testing.T, netNS *os.File, clientIP string, clientPort int, routerIP string, routerPort int, destIP string,
) {
	t.Helper()

	snatArgs := []string{
		"-t", "nat",
		"-A", "POSTROUTING",
		"-p", "udp",
		"--sport", fmt.Sprint(clientPort),
		"-j", "SNAT",
		"--to-source", fmt.Sprintf("%s:%d", routerIP, routerPort),
	}
	if destIP != "" {
		// Insert `-d $destIP` after the --sport flag+value.
		newSnatArgs := append([]string{}, snatArgs[:8]...)
		newSnatArgs = append(newSnatArgs, "-d", destIP)
		newSnatArgs = append(newSnatArgs, snatArgs[8:]...)
		snatArgs = newSnatArgs
	}
	_, err := commandInNetNS(netNS, "iptables", snatArgs).Output()
	require.NoError(t, wrapExitErr(err), "add iptables SNAT rule")

	// Incoming traffic should be forwarded to the client's IP.
	dnatArgs := []string{
		"-t", "nat",
		"-A", "PREROUTING",
		"-p", "udp",
		"--dport", fmt.Sprint(routerPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", clientIP, clientPort),
	}
	if destIP != "" {
		// Insert `-s $destIP` before the --dport flag+value.
		newDnatArgs := append([]string{}, dnatArgs[:6]...)
		newDnatArgs = append(newDnatArgs, "-s", destIP)
		newDnatArgs = append(newDnatArgs, dnatArgs[6:]...)
		dnatArgs = newDnatArgs
	}
	_, err = commandInNetNS(netNS, "iptables", dnatArgs).Output()
	require.NoError(t, wrapExitErr(err), "add iptables DNAT rule")
}

func commandInNetNS(netNS *os.File, bin string, args []string) *exec.Cmd {
	//nolint:gosec
	cmd := exec.Command("nsenter", append([]string{"--net=/proc/self/fd/3", bin}, args...)...)
	cmd.ExtraFiles = []*os.File{netNS}
	return cmd
}

func wrapExitErr(err error) error {
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if xerrors.As(err, &exitErr) {
		return xerrors.Errorf("output: %s\n\n%w", bytes.TrimSpace(exitErr.Stderr), exitErr)
	}
	return err
}
