//go:build linux

package confine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/xerrors"
)

var hostSubnetAllocator = newSubnetAllocator(listHostNetworkPrefixes)

// NetworkNamespace is a named network namespace connected to the host by a
// dedicated veth pair.
type NetworkNamespace struct {
	mu sync.Mutex

	name          string
	hostVeth      string
	peerVeth      string
	hostIP        string
	peerIP        string
	prefix        netip.Prefix
	ipPath        string
	iptablesPath  string
	ip6tablesPath string
	hostRules     []installedFirewallRule
	closed        bool
	configured    bool
}

type networkNamespace = NetworkNamespace

type networkNamespaceTools struct {
	ipPath        string
	iptablesPath  string
	ip6tablesPath string
}

type installedFirewallRule struct {
	commandPath string
	deleteArgs  []string
}

// PreflightNetworkNamespace verifies that this host can enforce network
// namespace confinement.
func PreflightNetworkNamespace(ctx context.Context) error {
	_, err := checkNetworkNamespaceSupport(ctx)
	return err
}

func checkNetworkNamespaceSupport(ctx context.Context) (networkNamespaceTools, error) {
	var tools networkNamespaceTools
	var err error
	tools.ipPath, err = exec.LookPath("ip")
	if err != nil {
		return tools, unsupportedNetworkNamespace("ip binary not found", err)
	}
	tools.iptablesPath, err = exec.LookPath("iptables")
	if err != nil {
		return tools, unsupportedNetworkNamespace("iptables binary not found", err)
	}
	tools.ip6tablesPath, err = exec.LookPath("ip6tables")
	if err != nil {
		return tools, unsupportedNetworkNamespace("ip6tables binary not found", err)
	}

	if err := probeFirewall(ctx, tools.iptablesPath, []string{"-p", "tcp", "-m", "multiport", "--dports", "80,443", "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"}); err != nil {
		return tools, unsupportedNetworkNamespace("cannot modify host IPv4 firewall", err)
	}
	if err := probeFirewall(ctx, tools.ip6tablesPath, []string{"-j", "DROP"}); err != nil {
		return tools, unsupportedNetworkNamespace("cannot modify host IPv6 firewall", err)
	}

	suffix, err := randomHex(4)
	if err != nil {
		return tools, unsupportedNetworkNamespace("cannot generate preflight namespace name", err)
	}
	name := "coder-confine-check-" + suffix
	if err := runIP(ctx, tools.ipPath, "netns", "add", name); err != nil {
		return tools, unsupportedNetworkNamespace("cannot create a network namespace", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), reportTimeout)
			defer cancel()
			_ = runIP(cleanupCtx, tools.ipPath, "netns", "del", name)
		}
	}()
	if err := runIP(ctx, tools.ipPath, "netns", "del", name); err != nil {
		return tools, unsupportedNetworkNamespace("cannot delete a network namespace", err)
	}
	cleanup = false
	return tools, nil
}

func probeFirewall(ctx context.Context, commandPath string, ruleSpec []string) (retErr error) {
	suffix, err := randomHex(4)
	if err != nil {
		return err
	}
	chain := "CODERCF" + suffix
	if _, err := runCommandOutput(ctx, commandPath, "-w", "-N", chain); err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), reportTimeout)
			defer cancel()
			_, _ = runCommandOutput(cleanupCtx, commandPath, "-w", "-F", chain)
			_, _ = runCommandOutput(cleanupCtx, commandPath, "-w", "-X", chain)
		}
	}()

	rule := append([]string{"-w", "-A", chain}, ruleSpec...)
	if _, err := runCommandOutput(ctx, commandPath, rule...); err != nil {
		return err
	}
	if _, err := runCommandOutput(ctx, commandPath, "-w", "-F", chain); err != nil {
		return err
	}
	if _, err := runCommandOutput(ctx, commandPath, "-w", "-X", chain); err != nil {
		return err
	}
	return nil
}

// OpenNetworkNamespace creates an isolated network namespace. Call
// ConfigureEgress after binding the host-side listeners and before starting a
// process in the namespace.
func OpenNetworkNamespace(ctx context.Context, options NetworkNamespaceOptions) (_ *NetworkNamespace, retErr error) {
	poolValue := options.Pool
	if poolValue == "" {
		poolValue = DefaultNetNSPool
	}
	pool, err := netip.ParsePrefix(poolValue)
	if err != nil {
		return nil, xerrors.Errorf("parse netns subnet pool %q: %w", poolValue, err)
	}
	tools, err := checkNetworkNamespaceSupport(ctx)
	if err != nil {
		return nil, err
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
		name:          "coder-confine-" + suffix,
		hostVeth:      "cc-h-" + suffix,
		peerVeth:      "cc-n-" + suffix,
		hostIP:        lease.Host.String(),
		peerIP:        lease.Peer.String(),
		prefix:        lease.Prefix,
		ipPath:        tools.ipPath,
		iptablesPath:  tools.iptablesPath,
		ip6tablesPath: tools.ip6tablesPath,
	}
	defer func() {
		if retErr != nil {
			if cleanupErr := netns.Close(); cleanupErr != nil {
				retErr = errors.Join(retErr, xerrors.Errorf("roll back network namespace: %w", cleanupErr))
			}
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
		if err := runIP(ctx, tools.ipPath, args...); err != nil {
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

// ConfigureEgress installs child-side redirection rules and host-side
// enforcement rules. Failure closes the namespace and removes partial state.
func (n *NetworkNamespace) ConfigureEgress(ctx context.Context, ports NetworkNamespacePorts) (retErr error) {
	if n == nil {
		return xerrors.New("configure nil network namespace")
	}
	if err := ports.validate(); err != nil {
		_ = n.Close()
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
			if cleanupErr := n.closeLocked(); cleanupErr != nil {
				retErr = errors.Join(retErr, xerrors.Errorf("roll back network namespace egress: %w", cleanupErr))
			}
		}
	}()

	for _, args := range hostIPv4Rules(n.hostVeth, n.hostIP, n.peerIP, ports) {
		if err := n.insertHostRule(ctx, n.iptablesPath, args); err != nil {
			return err
		}
	}
	for _, args := range hostIPv6Rules(n.hostVeth) {
		if err := n.insertHostRule(ctx, n.ip6tablesPath, args); err != nil {
			return err
		}
	}
	for _, args := range childIPv4Rules(n.hostIP, ports) {
		if err := runInNetworkNamespace(ctx, n.ipPath, n.name, n.iptablesPath, args...); err != nil {
			return err
		}
	}
	tryDisableNetworkNamespaceIPv6(ctx, n.ipPath, n.name)
	for _, args := range childIPv6Rules() {
		if err := runInNetworkNamespace(ctx, n.ipPath, n.name, n.ip6tablesPath, args...); err != nil {
			return err
		}
	}
	n.configured = true
	return nil
}

func tryDisableNetworkNamespaceIPv6(ctx context.Context, ipPath, name string) {
	sysctlPath, err := exec.LookPath("sysctl")
	if err != nil {
		return
	}
	_ = runInNetworkNamespace(ctx, ipPath, name, sysctlPath,
		"-w",
		"net.ipv6.conf.all.disable_ipv6=1",
		"net.ipv6.conf.default.disable_ipv6=1",
		"net.ipv6.conf.lo.disable_ipv6=1",
	)
}

func (n *NetworkNamespace) insertHostRule(ctx context.Context, commandPath string, args []string) error {
	deleteArgs, err := deleteArgsForInsertedRule(args)
	if err != nil {
		return err
	}
	if _, err := runCommandOutput(ctx, commandPath, args...); err != nil {
		return err
	}
	n.hostRules = append(n.hostRules, installedFirewallRule{
		commandPath: commandPath,
		deleteArgs:  deleteArgs,
	})
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

func childIPv6Rules() [][]string {
	return [][]string{
		{"-w", "-P", "INPUT", "DROP"},
		{"-w", "-P", "OUTPUT", "DROP"},
		{"-w", "-P", "FORWARD", "DROP"},
	}
}

func hostIPv4Rules(hostVeth, hostIP, peerIP string, ports NetworkNamespacePorts) [][]string {
	httpPort := strconv.FormatUint(uint64(ports.HTTP), 10)
	sniPort := strconv.FormatUint(uint64(ports.SNI), 10)
	dnsPort := strconv.FormatUint(uint64(ports.DNS), 10)
	tcpListenerPorts := strings.Join([]string{httpPort, sniPort, dnsPort}, ",")

	return [][]string{
		{"-w", "-I", "INPUT", "1", "-i", hostVeth, "-j", "DROP"},
		{"-w", "-I", "INPUT", "1", "-i", hostVeth, "-s", peerIP, "-d", hostIP, "-p", "udp", "--dport", dnsPort, "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-I", "INPUT", "1", "-i", hostVeth, "-s", peerIP, "-d", hostIP, "-p", "tcp", "-m", "multiport", "--dports", tcpListenerPorts, "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"},
		{"-w", "-I", "FORWARD", "1", "-i", hostVeth, "-j", "DROP"},
		{"-w", "-I", "FORWARD", "1", "-o", hostVeth, "-j", "DROP"},
	}
}

func hostIPv6Rules(hostVeth string) [][]string {
	return [][]string{
		{"-w", "-I", "INPUT", "1", "-i", hostVeth, "-j", "DROP"},
		{"-w", "-I", "OUTPUT", "1", "-o", hostVeth, "-j", "DROP"},
		{"-w", "-I", "FORWARD", "1", "-i", hostVeth, "-j", "DROP"},
		{"-w", "-I", "FORWARD", "2", "-o", hostVeth, "-j", "DROP"},
	}
}

func deleteArgsForInsertedRule(insertArgs []string) ([]string, error) {
	if len(insertArgs) < 5 || insertArgs[0] != "-w" || insertArgs[1] != "-I" {
		return nil, xerrors.Errorf("invalid inserted firewall rule: %v", insertArgs)
	}
	deleteArgs := []string{"-w", "-D", insertArgs[2]}
	return append(deleteArgs, insertArgs[4:]...), nil
}

// CommandArgs returns argv that executes a command inside a fully configured
// namespace.
func (n *NetworkNamespace) CommandArgs(args []string) ([]string, error) {
	if n == nil {
		return nil, xerrors.New("execute in nil network namespace")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil, xerrors.New("execute in closed network namespace")
	}
	if !n.configured {
		return nil, xerrors.New("execute in network namespace before egress is configured")
	}
	result := []string{n.ipPath, "netns", "exec", n.name}
	return append(result, args...), nil
}

func (n *NetworkNamespace) execArgs(args []string) []string {
	result, err := n.CommandArgs(args)
	if err != nil {
		return []string{n.ipPath, "netns", "exec", "__coder_confine_unconfigured__"}
	}
	return result
}

// Close removes host firewall rules, the namespace, veth pair, resolver
// configuration, and subnet lease. It is safe to call more than once.
func (n *NetworkNamespace) Close() error {
	if n == nil {
		return nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closeLocked()
}

func (n *NetworkNamespace) closeLocked() error {
	n.closed = true

	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()
	var cleanupErrors []error
	remainingRules := make([]installedFirewallRule, 0, len(n.hostRules))
	for index := len(n.hostRules) - 1; index >= 0; index-- {
		rule := n.hostRules[index]
		if _, err := runCommandOutput(ctx, rule.commandPath, rule.deleteArgs...); err != nil && !isMissingFirewallRuleError(err) {
			remainingRules = append(remainingRules, rule)
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	slices.Reverse(remainingRules)
	n.hostRules = remainingRules
	if n.ipPath != "" {
		_ = runIP(ctx, n.ipPath, "netns", "del", n.name)
		_ = runIP(ctx, n.ipPath, "link", "del", n.hostVeth)
	}
	if err := os.RemoveAll(filepath.Join("/etc/netns", n.name)); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	}
	if n.prefix.IsValid() {
		hostSubnetAllocator.Release(n.prefix)
		n.prefix = netip.Prefix{}
	}
	return errors.Join(cleanupErrors...)
}

func isMissingFirewallRuleError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "bad rule") ||
		strings.Contains(message, "does a matching rule exist")
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
