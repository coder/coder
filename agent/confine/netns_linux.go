//go:build linux

package confine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/xerrors"
)

var hostSubnetAllocator = newSubnetAllocator(listHostNetworkPrefixes)

// NetworkNamespaceOptions controls creation of a confined network namespace.
type NetworkNamespaceOptions struct {
	// Pool is an IPv4 prefix divided into /30 networks. An empty value uses
	// DefaultNetNSPool.
	Pool string
}

// NetworkNamespacePorts identifies the host-side egress listeners.
type NetworkNamespacePorts struct {
	HTTP uint16
	SNI  uint16
	DNS  uint16
}

// NetworkNamespace is a named network namespace connected to the host by a
// dedicated veth pair.
type NetworkNamespace struct {
	mu sync.Mutex

	name         string
	hostVeth     string
	peerVeth     string
	hostIP       string
	peerIP       string
	prefix       netip.Prefix
	ipPath       string
	iptablesPath string
	closed       bool
	configured   bool
}

type networkNamespace = NetworkNamespace

// OpenNetworkNamespace creates an isolated network namespace. Call
// ConfigureEgress after binding the host-side listeners and before starting a
// process in the namespace.
func OpenNetworkNamespace(ctx context.Context, options NetworkNamespaceOptions) (_ *NetworkNamespace, retErr error) {
	ipPath, err := exec.LookPath("ip")
	if err != nil {
		return nil, xerrors.Errorf("find ip binary: %w", err)
	}
	iptablesPath, err := exec.LookPath("iptables")
	if err != nil {
		return nil, xerrors.Errorf("find iptables binary: %w", err)
	}

	poolValue := options.Pool
	if poolValue == "" {
		poolValue = DefaultNetNSPool
	}
	pool, err := netip.ParsePrefix(poolValue)
	if err != nil {
		return nil, xerrors.Errorf("parse netns subnet pool %q: %w", poolValue, err)
	}
	lease, err := hostSubnetAllocator.Allocate(ctx, pool)
	if err != nil {
		return nil, err
	}

	suffix, err := randomHex(4)
	if err != nil {
		hostSubnetAllocator.Release(lease.Prefix)
		return nil, err
	}
	netns := &NetworkNamespace{
		name:         "coder-confine-" + suffix,
		hostVeth:     "cc-h-" + suffix,
		peerVeth:     "cc-n-" + suffix,
		hostIP:       lease.Host.String(),
		peerIP:       lease.Peer.String(),
		prefix:       lease.Prefix,
		ipPath:       ipPath,
		iptablesPath: iptablesPath,
	}
	defer func() {
		if retErr != nil {
			_ = netns.Close()
		}
	}()

	prefixLength := strconv.Itoa(lease.Prefix.Bits())
	commands := [][]string{
		{"netns", "add", netns.name},
		{"link", "add", netns.hostVeth, "type", "veth", "peer", "name", netns.peerVeth},
		{"addr", "add", netns.hostIP + "/" + prefixLength, "dev", netns.hostVeth},
		{"link", "set", netns.hostVeth, "up"},
		{"link", "set", netns.peerVeth, "netns", netns.name},
		{"-n", netns.name, "addr", "add", netns.peerIP + "/" + prefixLength, "dev", netns.peerVeth},
		{"-n", netns.name, "link", "set", netns.peerVeth, "up"},
		{"-n", netns.name, "link", "set", "lo", "up"},
		{"-n", netns.name, "route", "add", "default", "via", netns.hostIP, "dev", netns.peerVeth},
	}
	for _, args := range commands {
		if err := runIP(ctx, ipPath, args...); err != nil {
			return nil, err
		}
	}

	resolverDir := filepath.Join("/etc/netns", netns.name)
	if err := os.MkdirAll(resolverDir, 0o755); err != nil {
		return nil, xerrors.Errorf("create netns resolver directory: %w", err)
	}
	resolver := []byte("nameserver " + netns.hostIP + "\n")
	//nolint:gosec // Resolver configuration is conventionally world-readable and contains no secrets.
	if err := os.WriteFile(filepath.Join(resolverDir, "resolv.conf"), resolver, 0o644); err != nil {
		return nil, xerrors.Errorf("write netns resolver: %w", err)
	}
	return netns, nil
}

func newNetworkNamespace(ctx context.Context) (*networkNamespace, error) {
	return OpenNetworkNamespace(ctx, NetworkNamespaceOptions{})
}

// HostIP returns the address host-side listeners must bind to.
func (n *NetworkNamespace) HostIP() string {
	if n == nil {
		return ""
	}
	return n.hostIP
}

// ConfigureEgress installs the child-side redirection and filter rules.
func (n *NetworkNamespace) ConfigureEgress(ctx context.Context, ports NetworkNamespacePorts) (retErr error) {
	if n == nil {
		return xerrors.New("configure nil network namespace")
	}
	if err := ports.validate(); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return xerrors.New("configure closed network namespace")
	}
	if n.configured {
		return xerrors.New("network namespace egress is already configured")
	}
	defer func() {
		if retErr != nil {
			n.mu.Unlock()
			_ = n.Close()
			n.mu.Lock()
		}
	}()

	for _, args := range childIPv4Rules(n.hostIP, ports) {
		if err := runInNetworkNamespace(ctx, n.ipPath, n.name, n.iptablesPath, args...); err != nil {
			return err
		}
	}
	n.configured = true
	return nil
}

func (p NetworkNamespacePorts) validate() error {
	if p.HTTP == 0 {
		return xerrors.New("HTTP listener port must be non-zero")
	}
	if p.SNI == 0 {
		return xerrors.New("SNI listener port must be non-zero")
	}
	if p.DNS == 0 {
		return xerrors.New("DNS listener port must be non-zero")
	}
	return nil
}

func childIPv4Rules(hostIP string, ports NetworkNamespacePorts) [][]string {
	httpPort := strconv.FormatUint(uint64(ports.HTTP), 10)
	sniPort := strconv.FormatUint(uint64(ports.SNI), 10)
	dnsPort := strconv.FormatUint(uint64(ports.DNS), 10)
	tcpListenerPorts := strings.Join([]string{httpPort, sniPort, dnsPort}, ",")

	return [][]string{
		{"-w", "-t", "nat", "-A", "OUTPUT", "-d", "127.0.0.0/8", "-j", "RETURN"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-d", hostIP, "-p", "tcp", "-m", "multiport", "--dports", tcpListenerPorts, "-j", "RETURN"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-d", hostIP, "-p", "udp", "--dport", dnsPort, "-j", "RETURN"},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "80", "-j", "DNAT", "--to-destination", net.JoinHostPort(hostIP, httpPort)},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "443", "-j", "DNAT", "--to-destination", net.JoinHostPort(hostIP, sniPort)},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "DNAT", "--to-destination", net.JoinHostPort(hostIP, dnsPort)},
		{"-w", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "DNAT", "--to-destination", net.JoinHostPort(hostIP, dnsPort)},
		{"-w", "-P", "INPUT", "DROP"},
		{"-w", "-P", "OUTPUT", "DROP"},
		{"-w", "-P", "FORWARD", "DROP"},
		{"-w", "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"},
		{"-w", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT"},
		{"-w", "-A", "OUTPUT", "-d", hostIP, "-p", "tcp", "-m", "multiport", "--dports", tcpListenerPorts, "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-A", "OUTPUT", "-d", hostIP, "-p", "udp", "--dport", dnsPort, "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-A", "INPUT", "-s", hostIP, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
	}
}

// CommandArgs returns argv that executes a command inside the namespace.
func (n *NetworkNamespace) CommandArgs(args []string) []string {
	result := []string{n.ipPath, "netns", "exec", n.name}
	return append(result, args...)
}

func (n *NetworkNamespace) execArgs(args []string) []string {
	return n.CommandArgs(args)
}

// Close removes the namespace, veth pair, resolver configuration, and subnet
// lease. It is safe to call more than once.
func (n *NetworkNamespace) Close() error {
	if n == nil {
		return nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()
	if n.ipPath != "" {
		_ = runIP(ctx, n.ipPath, "netns", "del", n.name)
		_ = runIP(ctx, n.ipPath, "link", "del", n.hostVeth)
	}
	resolverErr := os.RemoveAll(filepath.Join("/etc/netns", n.name))
	if n.prefix.IsValid() {
		hostSubnetAllocator.Release(n.prefix)
		n.prefix = netip.Prefix{}
	}
	return resolverErr
}

func listHostNetworkPrefixes(ctx context.Context) ([]netip.Prefix, error) {
	ipPath, err := exec.LookPath("ip")
	if err != nil {
		return nil, xerrors.Errorf("find ip binary: %w", err)
	}

	routeOutput, err := runCommandOutput(ctx, ipPath, "-j", "route", "show")
	if err != nil {
		return nil, err
	}
	var routes []struct {
		Destination string `json:"dst"`
	}
	if err := json.Unmarshal(routeOutput, &routes); err != nil {
		return nil, xerrors.Errorf("decode ip route output: %w", err)
	}

	addrOutput, err := runCommandOutput(ctx, ipPath, "-j", "addr", "show")
	if err != nil {
		return nil, err
	}
	var links []struct {
		Addresses []struct {
			Family       string `json:"family"`
			Local        string `json:"local"`
			PrefixLength int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(addrOutput, &links); err != nil {
		return nil, xerrors.Errorf("decode ip addr output: %w", err)
	}

	prefixes := make([]netip.Prefix, 0, len(routes)+len(links))
	for _, route := range routes {
		if route.Destination == "" || route.Destination == "default" {
			continue
		}
		prefix, err := netip.ParsePrefix(route.Destination)
		if err != nil || !prefix.Addr().Is4() {
			continue
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	for _, link := range links {
		for _, address := range link.Addresses {
			if address.Family != "inet" {
				continue
			}
			addr, err := netip.ParseAddr(address.Local)
			if err != nil || !addr.Is4() || address.PrefixLength < 0 || address.PrefixLength > addr.BitLen() {
				continue
			}
			prefixes = append(prefixes, netip.PrefixFrom(addr, address.PrefixLength))
		}
	}
	return prefixes, nil
}

func runIP(ctx context.Context, ipPath string, args ...string) error {
	_, err := runCommandOutput(ctx, ipPath, args...)
	return err
}

func runInNetworkNamespace(ctx context.Context, ipPath, name, commandPath string, args ...string) error {
	commandArgs := []string{"netns", "exec", name, commandPath}
	commandArgs = append(commandArgs, args...)
	_, err := runCommandOutput(ctx, ipPath, commandArgs...)
	return err
}

func runCommandOutput(ctx context.Context, commandPath string, args ...string) ([]byte, error) {
	//nolint:gosec,gocritic // The namespace manager invokes fixed system binaries with validated arguments.
	output, err := exec.CommandContext(ctx, commandPath, args...).CombinedOutput()
	if err != nil {
		return nil, xerrors.Errorf("%s %v: %w: %s", commandPath, args, err, output)
	}
	return output, nil
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", xerrors.Errorf("generate netns suffix: %w", err)
	}
	return hex.EncodeToString(value), nil
}
