//go:build linux
// +build linux

package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tailscale/netlink"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/cryptorand"
)

type TestNetworking struct {
	// ServerListenAddr is the IP address and port that the server listens on,
	// passed to StartServer.
	ServerListenAddr string
	// ServerAccessURLClient1 is the hostname and port that the first client
	// uses to access the server.
	ServerAccessURLClient1 string
	// ServerAccessURLClient2 is the hostname and port that the second client
	// uses to access the server.
	ServerAccessURLClient2 string

	// Networking settings for each subprocess.
	ProcessServer  TestNetworkingProcess
	ProcessClient1 TestNetworkingProcess
	ProcessClient2 TestNetworkingProcess
}

type TestNetworkingProcess struct {
	// NetNS to enter. If nil, the current network namespace is used.
	NetNS *os.File
}

// SetupNetworkingLoopback creates a network namespace with a loopback interface
// for all tests to share. This is the simplest networking setup. The network
// namespace only exists for isolation on the host and doesn't serve any routing
// purpose.
func SetupNetworkingLoopback(t *testing.T, _ slog.Logger) TestNetworking {
	netNSName := "codertest_netns_"
	randStr, err := cryptorand.String(4)
	require.NoError(t, err, "generate random string for netns name")
	netNSName += randStr

	// Create a single network namespace for all tests so we can have an
	// isolated loopback interface.
	netNSFile := createNetNS(t, netNSName)

	var (
		listenAddr = "127.0.0.1:8080"
		process    = TestNetworkingProcess{
			NetNS: netNSFile,
		}
	)
	return TestNetworking{
		ServerListenAddr:       listenAddr,
		ServerAccessURLClient1: "http://" + listenAddr,
		ServerAccessURLClient2: "http://" + listenAddr,
		ProcessServer:          process,
		ProcessClient1:         process,
		ProcessClient2:         process,
	}
}

// SetupNetworkingEasyNAT creates a network namespace with a router that NATs
// packets between two clients and a server.
// See createFakeRouter for the full topology.
// NAT is achieved through a single iptables masquerade rule.
func SetupNetworkingEasyNAT(t *testing.T, _ slog.Logger) TestNetworking {
	router := createFakeRouter(t)

	// Set up iptables masquerade rules to allow the router to NAT packets
	// between the Three Kingdoms.
	_, err := commandInNetNS(router.RouterNetNS, "sysctl", []string{"-w", "net.ipv4.ip_forward=1"}).Output()
	require.NoError(t, wrapExitErr(err), "enable IP forwarding in router NetNS")
	_, err = commandInNetNS(router.RouterNetNS, "iptables", []string{
		"-t", "nat",
		"-A", "POSTROUTING",
		// Every interface except loopback.
		"!", "-o", "lo",
		"-j", "MASQUERADE",
	}).Output()
	require.NoError(t, wrapExitErr(err), "add iptables masquerade rule")

	return router.Net
}

type fakeRouter struct {
	Net TestNetworking

	RouterNetNS *os.File
	RouterVeths struct {
		Server  string
		Client1 string
		Client2 string
	}
	ServerNetNS  *os.File
	ServerVeth   string
	Client1NetNS *os.File
	Client1Veth  string
	Client2NetNS *os.File
	Client2Veth  string
}

// fakeRouter creates multiple namespaces with veth pairs between them with
// the following topology:
//
// namespaces:
//   - router
//   - server
//   - client1
//   - client2
//
// veth pairs:
//   - router-server  (10.0.1.1) <-> server-router  (10.0.1.2)
//   - router-client1 (10.0.2.1) <-> client1-router (10.0.2.2)
//   - router-client2 (10.0.3.1) <-> client2-router (10.0.3.2)
//
// No iptables rules are created, so packets will not be forwarded out of the
// box. Routes are created between all namespaces based on the veth pairs,
// however.
func createFakeRouter(t *testing.T) fakeRouter {
	t.Helper()
	const (
		routerServerPrefix  = "10.0.1."
		routerServerIP      = routerServerPrefix + "1"
		serverIP            = routerServerPrefix + "2"
		routerClient1Prefix = "10.0.2."
		routerClient1IP     = routerClient1Prefix + "1"
		client1IP           = routerClient1Prefix + "2"
		routerClient2Prefix = "10.0.3."
		routerClient2IP     = routerClient2Prefix + "1"
		client2IP           = routerClient2Prefix + "2"
	)

	prefix := uniqNetName(t) + "_"
	router := fakeRouter{}
	router.RouterVeths.Server = prefix + "r-s"
	router.RouterVeths.Client1 = prefix + "r-c1"
	router.RouterVeths.Client2 = prefix + "r-c2"
	router.ServerVeth = prefix + "s-r"
	router.Client1Veth = prefix + "c1-r"
	router.Client2Veth = prefix + "c2-r"

	// Create namespaces.
	router.RouterNetNS = createNetNS(t, prefix+"r")
	serverNS := createNetNS(t, prefix+"s")
	client1NS := createNetNS(t, prefix+"c1")
	client2NS := createNetNS(t, prefix+"c2")

	vethPairs := []struct {
		parentName string
		peerName   string
		parentNS   *os.File
		peerNS     *os.File
		parentIP   string
		peerIP     string
	}{
		{
			parentName: router.RouterVeths.Server,
			peerName:   router.ServerVeth,
			parentNS:   router.RouterNetNS,
			peerNS:     serverNS,
			parentIP:   routerServerIP,
			peerIP:     serverIP,
		},
		{
			parentName: router.RouterVeths.Client1,
			peerName:   router.Client1Veth,
			parentNS:   router.RouterNetNS,
			peerNS:     client1NS,
			parentIP:   routerClient1IP,
			peerIP:     client1IP,
		},
		{
			parentName: router.RouterVeths.Client2,
			peerName:   router.Client2Veth,
			parentNS:   router.RouterNetNS,
			peerNS:     client2NS,
			parentIP:   routerClient2IP,
			peerIP:     client2IP,
		},
	}

	for _, vethPair := range vethPairs {
		err := createVethPair(vethPair.parentName, vethPair.peerName)
		require.NoErrorf(t, err, "create veth pair %q <-> %q", vethPair.parentName, vethPair.peerName)

		// Move the veth interfaces to the respective network namespaces.
		err = setVethNetNS(vethPair.parentName, int(vethPair.parentNS.Fd()))
		require.NoErrorf(t, err, "set veth %q to NetNS", vethPair.parentName)
		err = setVethNetNS(vethPair.peerName, int(vethPair.peerNS.Fd()))
		require.NoErrorf(t, err, "set veth %q to NetNS", vethPair.peerName)

		// Set IP addresses on the interfaces.
		err = setInterfaceIP(vethPair.parentNS, vethPair.parentName, vethPair.parentIP)
		require.NoErrorf(t, err, "set IP %q on interface %q", vethPair.parentIP, vethPair.parentName)
		err = setInterfaceIP(vethPair.peerNS, vethPair.peerName, vethPair.peerIP)
		require.NoErrorf(t, err, "set IP %q on interface %q", vethPair.peerIP, vethPair.peerName)

		// Bring up both interfaces.
		err = setInterfaceUp(vethPair.parentNS, vethPair.parentName)
		require.NoErrorf(t, err, "bring up interface %q", vethPair.parentName)
		err = setInterfaceUp(vethPair.peerNS, vethPair.peerName)
		require.NoErrorf(t, err, "bring up interface %q", vethPair.parentName)

		// We don't need to add a route from parent to peer since the kernel
		// already adds a default route for the /24. We DO need to add a default
		// route from peer to parent, however.
		err = addRouteInNetNS(vethPair.peerNS, []string{"default", "via", vethPair.parentIP, "dev", vethPair.peerName})
		require.NoErrorf(t, err, "add peer default route to %q", vethPair.peerName)
	}

	router.Net = TestNetworking{
		ServerListenAddr:       serverIP + ":8080",
		ServerAccessURLClient1: "http://" + serverIP + ":8080",
		ServerAccessURLClient2: "http://" + serverIP + ":8080",
		ProcessServer: TestNetworkingProcess{
			NetNS: serverNS,
		},
		ProcessClient1: TestNetworkingProcess{
			NetNS: client1NS,
		},
		ProcessClient2: TestNetworkingProcess{
			NetNS: client2NS,
		},
	}
	return router
}

func uniqNetName(t *testing.T) string {
	t.Helper()
	netNSName := "cdr_"
	randStr, err := cryptorand.String(3)
	require.NoError(t, err, "generate random string for netns name")
	netNSName += randStr
	return netNSName
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
	path := fmt.Sprintf("/run/netns/%s", name)
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
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

// createVethPair creates a veth pair with the given names.
func createVethPair(parentVethName, peerVethName string) error {
	vethLinkAttrs := netlink.NewLinkAttrs()
	vethLinkAttrs.Name = parentVethName
	veth := &netlink.Veth{
		LinkAttrs: vethLinkAttrs,
		PeerName:  peerVethName,
	}

	err := netlink.LinkAdd(veth)
	if err != nil {
		return xerrors.Errorf("LinkAdd(name: %q, peerName: %q): %w", parentVethName, peerVethName, err)
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
