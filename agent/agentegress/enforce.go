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
	// filterChain holds the UDP policy and hangs off filter OUTPUT.
	filterChain = "CODER_EGRESS_FILTER"
)

// EnforcerOptions configure an Enforcer.
type EnforcerOptions struct {
	// Execer runs iptables. Defaults to agentexec.DefaultExecer.
	Execer agentexec.Execer
	// ProxyPort is the local port the Proxy listens on.
	ProxyPort uint16
	// ControlPlaneHosts are host[:port] destinations that bypass the proxy.
	ControlPlaneHosts []string
	// Resolver defaults to net.DefaultResolver.
	Resolver Resolver
}

// Enforcer installs netfilter rules that redirect all outbound TCP from the
// workspace into the local proxy and drop UDP that could bypass it.
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
	proxyPort uint16
	hosts     []string

	mu        sync.Mutex
	installed bool
	// prefix is the command prefix that grants iptables privileges, either
	// empty (running as root) or ["sudo", "-n"].
	prefix []string
}

// NewEnforcer returns an Enforcer. Nothing is touched until Install.
func NewEnforcer(logger slog.Logger, opts EnforcerOptions) (*Enforcer, error) {
	if opts.ProxyPort == 0 {
		return nil, xerrors.New("proxy port is required")
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
		proxyPort: opts.ProxyPort,
		hosts:     opts.ControlPlaneHosts,
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

// buildRules produces the chain contents for one address family. Callers
// pass only exemptions of the matching family.
func buildRules(proxyPort uint16, exemptions []hostExemption) ruleSet {
	port := strconv.Itoa(int(proxyPort))
	var rs ruleSet

	// Loopback stays local, otherwise the proxy would redirect its own
	// clients back into itself.
	rs.nat = append(rs.nat, rule{"-A", natChain, "-o", "lo", "-j", "RETURN"})
	for _, ex := range exemptions {
		r := rule{"-A", natChain, "-d", ex.addr.String(), "-p", "tcp"}
		if ex.port != 0 {
			r = append(r, "--dport", strconv.Itoa(int(ex.port)))
		}
		rs.nat = append(rs.nat, append(r, "-j", "RETURN"))
	}
	rs.nat = append(rs.nat, rule{"-A", natChain, "-p", "tcp", "-j", "REDIRECT", "--to-ports", port})

	rs.filter = append(rs.filter,
		rule{"-A", filterChain, "-o", "lo", "-j", "ACCEPT"},
		// DNS is not captured in this POC; the resolver must keep working
		// so explicit proxy clients can name their destinations.
		rule{"-A", filterChain, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
	)
	for _, ex := range exemptions {
		// WireGuard, STUN and DERP-over-UDP to the control plane and exit
		// node must flow regardless of port.
		rs.filter = append(rs.filter, rule{"-A", filterChain, "-d", ex.addr.String(), "-p", "udp", "-j", "ACCEPT"})
	}
	rs.filter = append(rs.filter,
		// Dropping the rest of UDP kills QUIC so HTTP clients fall back to
		// TCP, which the nat chain redirects.
		rule{"-A", filterChain, "-p", "udp", "-j", "DROP"},
		rule{"-A", filterChain, "-p", "tcp", "-j", "ACCEPT"},
	)
	return rs
}

// resolveExemptions turns ControlPlaneHosts into concrete addresses. Hosts
// that fail to resolve are logged and skipped rather than blocking
// enforcement, since a missing exemption fails closed.
func (e *Enforcer) resolveExemptions(ctx context.Context) []hostExemption {
	var out []hostExemption
	for _, hostport := range e.hosts {
		host, port := splitHostPortDefault(hostport, 0)
		if host == "" {
			continue
		}
		if addr, err := netip.ParseAddr(host); err == nil {
			out = append(out, hostExemption{addr: addr.Unmap(), port: port})
			continue
		}
		addrs, err := e.resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			e.logger.Warn(ctx, "control plane host did not resolve, not exempting it from egress enforcement",
				slog.F("host", host), slog.Error(err))
			continue
		}
		for _, addr := range addrs {
			out = append(out, hostExemption{addr: addr.Unmap(), port: port})
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

func (r rule) String() string {
	return strings.Join(r, " ")
}
