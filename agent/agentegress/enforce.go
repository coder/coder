package agentegress

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
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
	execer := opts.Execer
	if execer == nil {
		execer = agentexec.DefaultExecer
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Enforcer{
		logger:    logger,
		execer:    execer,
		resolver:  resolver,
		ports:     proxyPorts{tcp: opts.ProxyPort, dns: opts.DNSPort, udp: opts.UDPPort},
		hosts:     opts.ControlPlaneHosts,
		resolvers: opts.Resolvers,
	}, nil
}

// hostExemption is a control plane destination that must bypass the proxy.
// A zero port matches every port.
type hostExemption struct {
	addr netip.Addr
	port uint16
}

// rule is the argument list passed to iptables after "-t <table>".
type rule []string

// ruleSet is the complete set of rules for one address family.
type ruleSet struct {
	nat    []rule
	filter []rule
}

// ipFamily selects the address family a rule set is built for.
type ipFamily int

const (
	ipv4 ipFamily = iota
	ipv6
)

// buildRules produces the chain contents for one address family. Callers
// pass only exemptions and resolvers of the matching family; ipv6 selects
// the ip6tables spelling of the ICMP rules.
func buildRules(ports proxyPorts, exemptions []hostExemption, resolvers []netip.Addr, family ipFamily) ruleSet {
	var rs ruleSet

	// Loopback stays local, otherwise the proxy would redirect its own
	// clients back into itself.
	rs.nat = append(rs.nat, rule{"-A", natChain, "-o", "lo", "-j", "RETURN"})
	for _, ex := range exemptions {
		// WireGuard, STUN and DERP to the control plane and exit node must
		// flow over both transports.
		for _, proto := range []string{"tcp", "udp"} {
			r := rule{"-A", natChain, "-d", ex.addr.String(), "-p", proto}
			if ex.port != 0 {
				r = append(r, "--dport", strconv.Itoa(int(ex.port)))
			}
			rs.nat = append(rs.nat, append(r, "-j", "RETURN"))
		}
	}
	// The proxy reaches the system resolvers over TCP for exempt names.
	// Clients that do the same bypass the fake IP scheme but still hit the
	// TCP redirect for whatever they connect to next.
	for _, addr := range resolvers {
		rs.nat = append(rs.nat, rule{"-A", natChain, "-d", addr.String(), "-p", "tcp", "--dport", "53", "-j", "RETURN"})
	}
	rs.nat = append(rs.nat,
		rule{"-A", natChain, "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", strconv.Itoa(int(ports.dns))},
		rule{"-A", natChain, "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-ports", strconv.Itoa(int(ports.dns))},
		rule{"-A", natChain, "-p", "udp", "-j", "REDIRECT", "--to-ports", strconv.Itoa(int(ports.udp))},
		rule{"-A", natChain, "-p", "tcp", "-j", "REDIRECT", "--to-ports", strconv.Itoa(int(ports.tcp))},
	)

	// UDP is no longer dropped here: it is redirected and the exit node
	// decides, so a QUIC denial is a logged policy decision rather than a
	// silent kernel drop. ICMP cannot be carried by the proxy at all.
	rs.filter = append(rs.filter, rule{"-A", filterChain, "-o", "lo", "-j", "ACCEPT"})
	if family == ipv6 {
		for _, typ := range ndpICMPv6Types {
			rs.filter = append(rs.filter, rule{"-A", filterChain, "-p", "icmpv6", "--icmpv6-type", typ, "-j", "ACCEPT"})
		}
		rs.filter = append(rs.filter, rule{"-A", filterChain, "-p", "icmpv6", "-j", "DROP"})
	} else {
		rs.filter = append(rs.filter, rule{"-A", filterChain, "-p", "icmp", "-j", "DROP"})
	}
	return rs
}

// resolveExemptions turns ControlPlaneHosts into concrete addresses. Hosts
// that fail to resolve are logged and skipped rather than blocking
// enforcement, since a missing exemption fails closed.
func (e *Enforcer) resolveExemptions(ctx context.Context) []hostExemption {
	var out []hostExemption
	seen := make(map[hostExemption]struct{})
	add := func(ex hostExemption) {
		if _, ok := seen[ex]; ok {
			return
		}
		seen[ex] = struct{}{}
		out = append(out, ex)
	}
	for _, hostport := range e.hosts {
		host, port := splitHostPortDefault(hostport, 0)
		if host == "" {
			continue
		}
		if addr, err := netip.ParseAddr(host); err == nil {
			add(hostExemption{addr: addr.Unmap(), port: port})
			continue
		}
		addrs, err := e.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			e.logger.Warn(ctx, "control plane host did not resolve, not exempting it from egress enforcement",
				slog.F("host", host), slog.Error(err))
			continue
		}
		for _, addr := range addrs {
			add(hostExemption{addr: addr.Unmap(), port: port})
		}
	}
	return out
}

func splitByFamily(exemptions []hostExemption) (v4, v6 []hostExemption) {
	for _, ex := range exemptions {
		if ex.addr.Is4() {
			v4 = append(v4, ex)
		} else {
			v6 = append(v6, ex)
		}
	}
	return v4, v6
}

func splitAddrsByFamily(addrs []netip.Addr) (v4, v6 []netip.Addr) {
	for _, addr := range addrs {
		if addr.Is4() {
			v4 = append(v4, addr)
		} else {
			v6 = append(v6, addr)
		}
	}
	return v4, v6
}

// upstreamResolvers returns the configured resolvers, or the system ones.
func (e *Enforcer) upstreamResolvers() []netip.Addr {
	if e.resolvers != nil {
		return e.resolvers
	}
	return systemResolvers()
}

func (r rule) String() string {
	return strings.Join(r, " ")
}
