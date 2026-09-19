package agentegress

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/agent/agentexec"
)

// ErrEnforcementUnavailable is returned by Enforcer.Install when netfilter
// rules cannot be managed on this host, either because the platform has no
// iptables or because the agent has neither root nor passwordless sudo. The
// caller should fall back to advisory proxy mode.
var ErrEnforcementUnavailable = xerrors.New("egress enforcement unavailable")

const (
	// natChain holds the REDIRECT rules and hangs off nat OUTPUT.
	natChain = "CODER_EGRESS"
	// filterChain holds the ICMP policy and hangs off filter OUTPUT.
	filterChain = "CODER_EGRESS_FILTER"
)

// ndpICMPv6Types are the Neighbor Discovery message types (RFC 4861) a host
// must keep sending to stay reachable on an IPv6 link: router solicitation
// and advertisement, neighbor solicitation and advertisement, and redirect.
var ndpICMPv6Types = []string{"133", "134", "135", "136", "137"}

// EnforcerOptions configure an Enforcer.
type EnforcerOptions struct {
	// Execer runs iptables. Defaults to agentexec.DefaultExecer.
	Execer agentexec.Execer
	// ProxyPort is the local port the Proxy's TCP listener is on.
	ProxyPort uint16
	// DNSPort is the local port the Proxy's DNS listener is on.
	DNSPort uint16
	// UDPPort is the local port the Proxy's UDP listener is on.
	UDPPort uint16
	// ControlPlaneHosts are host[:port] destinations that bypass the proxy.
	ControlPlaneHosts []string
	// Resolvers are the upstream DNS servers the proxy forwards exempt
	// queries to over TCP. Defaults to the system resolvers.
	Resolvers []netip.Addr
	// Resolver defaults to net.DefaultResolver.
	Resolver Resolver
}

// Enforcer installs netfilter rules that redirect all outbound TCP, UDP and
// DNS from the workspace into the local proxy and drop ICMP, which the
// proxy cannot carry.
//
// Rules are attached to the OUTPUT chain without an owner match. The agent
// runs as the same uid as the workspace user, so a uid exemption would also
// exempt the user's processes. The agent's own control traffic (coderd,
// DERP, STUN, exit node WireGuard) is instead exempted by destination via
// ControlPlaneHosts, and its CONNECT traffic to the exit node travels inside
// the userspace tailnet stack, never touching kernel TCP.
type Enforcer struct {
	logger    slog.Logger
	execer    agentexec.Execer
	resolver  Resolver
	ports     proxyPorts
	hosts     []string
	resolvers []netip.Addr

	mu        sync.Mutex
	installed bool
	// prefix is the command prefix that grants iptables privileges, either
	// empty (running as root) or ["sudo", "-n"].
	prefix []string
}

// proxyPorts are the loopback ports the Proxy listens on.
type proxyPorts struct {
	tcp uint16
	dns uint16
	udp uint16
}

// NewEnforcer returns an Enforcer. Nothing is touched until Install.
func NewEnforcer(logger slog.Logger, opts EnforcerOptions) (*Enforcer, error) {
	if opts.ProxyPort == 0 {
		return nil, xerrors.New("proxy port is required")
	}
	if opts.DNSPort == 0 {
		return nil, xerrors.New("dns port is required")
	}
	if opts.UDPPort == 0 {
		return nil, xerrors.New("udp port is required")
	}
	e := &Enforcer{
		logger:    logger,
		execer:    opts.Execer,
		resolver:  opts.Resolver,
		ports:     proxyPorts{tcp: opts.ProxyPort, dns: opts.DNSPort, udp: opts.UDPPort},
		hosts:     opts.ControlPlaneHosts,
		resolvers: opts.Resolvers,
	}
	if e.execer == nil {
		e.execer = agentexec.DefaultExecer
	}
	if e.resolver == nil {
		e.resolver = net.DefaultResolver
	}
	return e, nil
}

// rule is the argument list passed to iptables after "-t <table>".
type rule []string

// ruleSet maps an iptables table to the rules for the chain owned there.
type ruleSet map[string][]rule

// tables pairs each managed iptables table with the chain owned in it.
var tables = []struct{ table, chain string }{{"nat", natChain}, {"filter", filterChain}}

// ipFamily selects the address family a rule set is built for.
type ipFamily int

const (
	ipv4 ipFamily = iota
	ipv6
)

// matches reports whether addr belongs to the family.
func (f ipFamily) matches(addr netip.Addr) bool {
	return addr.Is4() == (f == ipv4)
}

// buildRules produces the chain contents for one address family. Exemptions
// and resolvers of the other family are skipped; ipv6 selects the ip6tables
// spelling of the ICMP rules. A zero exemption port matches every port.
func buildRules(ports proxyPorts, exemptions []netip.AddrPort, resolvers []netip.Addr, family ipFamily) ruleSet {
	rs := ruleSet{}
	nat := func(args ...string) { rs["nat"] = append(rs["nat"], append(rule{"-A", natChain}, args...)) }
	filter := func(args ...string) { rs["filter"] = append(rs["filter"], append(rule{"-A", filterChain}, args...)) }
	port := func(p uint16) string { return strconv.Itoa(int(p)) }

	// Loopback stays local, otherwise the proxy would redirect its own
	// clients back into itself.
	nat("-o", "lo", "-j", "RETURN")
	for _, ex := range exemptions {
		if !family.matches(ex.Addr()) {
			continue
		}
		// WireGuard, STUN and DERP to the control plane and exit node must
		// flow over both transports.
		for _, proto := range []string{"tcp", "udp"} {
			args := []string{"-d", ex.Addr().String(), "-p", proto}
			if ex.Port() != 0 {
				args = append(args, "--dport", port(ex.Port()))
			}
			nat(append(args, "-j", "RETURN")...)
		}
	}
	// The proxy reaches the system resolvers over TCP for exempt names.
	// Clients that do the same bypass the fake IP scheme but still hit the
	// TCP redirect for whatever they connect to next.
	for _, addr := range resolvers {
		if family.matches(addr) {
			nat("-d", addr.String(), "-p", "tcp", "--dport", "53", "-j", "RETURN")
		}
	}
	nat("-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", port(ports.dns))
	nat("-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-ports", port(ports.dns))
	nat("-p", "udp", "-j", "REDIRECT", "--to-ports", port(ports.udp))
	nat("-p", "tcp", "-j", "REDIRECT", "--to-ports", port(ports.tcp))

	// UDP is not dropped here: it is redirected and the exit node decides,
	// so a QUIC denial is a logged policy decision rather than a silent
	// kernel drop. ICMP cannot be carried by the proxy at all.
	filter("-o", "lo", "-j", "ACCEPT")
	if family == ipv4 {
		filter("-p", "icmp", "-j", "DROP")
		return rs
	}
	for _, typ := range ndpICMPv6Types {
		filter("-p", "icmpv6", "--icmpv6-type", typ, "-j", "ACCEPT")
	}
	filter("-p", "icmpv6", "-j", "DROP")
	return rs
}

// resolveExemptions turns ControlPlaneHosts into concrete addresses. Hosts
// that fail to resolve are logged and skipped rather than blocking
// enforcement, since a missing exemption fails closed.
func (e *Enforcer) resolveExemptions(ctx context.Context) []netip.AddrPort {
	var out []netip.AddrPort
	for _, hostport := range e.hosts {
		host, port := splitHostPortDefault(hostport, 0)
		if host == "" {
			continue
		}
		var addrs []netip.Addr
		if addr, err := netip.ParseAddr(host); err == nil {
			addrs = []netip.Addr{addr}
		} else if addrs, err = e.resolver.LookupNetIP(ctx, "ip", host); err != nil {
			e.logger.Warn(ctx, "control plane host did not resolve, not exempting it from egress enforcement",
				slog.F("host", host), slog.Error(err))
			continue
		}
		for _, addr := range addrs {
			if ex := netip.AddrPortFrom(addr.Unmap(), port); !slices.Contains(out, ex) {
				out = append(out, ex)
			}
		}
	}
	return out
}
