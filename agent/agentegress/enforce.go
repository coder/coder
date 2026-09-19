package agentegress

import (
	"context"
	"net"
	"net/netip"
	"os"
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
	// tproxyOutputChain marks locally generated UDP for policy routing.
	tproxyOutputChain = "CODER_EGRESS_UDP"
	// tproxyPreroutingChain delivers policy-routed UDP to the proxy.
	tproxyPreroutingChain = "CODER_EGRESS_TPROXY"
	// tproxyMark selects policy routing into the transparent UDP listener.
	tproxyMark = "0x434f"
	// tproxyBypassMark prevents transparent reply sockets from being captured.
	tproxyBypassMark = "0x4350"
	// tproxyTable is dedicated to the local route used by transparent UDP.
	tproxyTable = "54321"
)

// ndpICMPv6Types are the Neighbor Discovery message types (RFC 4861) a host
// must keep sending to stay reachable on an IPv6 link: router solicitation
// and advertisement, neighbor solicitation and advertisement, and redirect.
var ndpICMPv6Types = []string{"133", "134", "135", "136", "137"}

// EnforcerOptions configure an Enforcer.
type EnforcerOptions struct {
	// Execer runs iptables and ip. Defaults to agentexec.DefaultExecer.
	Execer agentexec.Execer
	// ProxyPort is the local port the Proxy's TCP listener is on.
	ProxyPort uint16
	// DNSPort is the local port the Proxy's DNS listener is on.
	DNSPort uint16
	// UDPPort is the local port the Proxy's UDP listener is on.
	UDPPort uint16
	// ControlPlaneHosts are proto/host:port destinations that bypass the
	// proxy. Protocol and port are mandatory.
	ControlPlaneHosts []string
	// Resolvers are the upstream DNS servers the proxy forwards exempt
	// queries to over TCP. Defaults to the system resolvers.
	Resolvers []netip.Addr
	// Resolver defaults to net.DefaultResolver.
	Resolver Resolver
	// LockdownCapabilities drops CAP_NET_ADMIN from the process bounding set
	// after installation. It defaults to true. Set LockdownCapabilitiesSet
	// when explicitly disabling it for debugging or tests.
	LockdownCapabilities bool
	// LockdownCapabilitiesSet distinguishes an explicit false value from the
	// secure default.
	LockdownCapabilitiesSet bool
	// capabilityLockdown is injected by tests.
	capabilityLockdown func() error
	// readFile and writeFile are injected by tests.
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
}

// Enforcer installs netfilter rules that redirect outbound TCP and DNS into
// the local proxy, transparently intercept non-DNS IPv4 UDP, and drop ICMP.
//
// Rules are attached to the OUTPUT chain without an owner match. The agent
// runs as the same uid as the workspace user, so a uid exemption would also
// exempt the user's processes. The agent's own control traffic is exempted by
// destination via ControlPlaneHosts.
type Enforcer struct {
	logger               slog.Logger
	execer               agentexec.Execer
	resolver             Resolver
	ports                proxyPorts
	hosts                []string
	resolvers            []netip.Addr
	lockdown             bool
	lockdownCapabilities func() error
	readFile             func(string) ([]byte, error)
	writeFile            func(string, []byte, os.FileMode) error

	mu          sync.Mutex
	installed   bool
	lockedDown  bool
	transparent bool
	// prefix grants networking privileges, either empty or ["sudo", "-n"].
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
	lockdown := true
	if opts.LockdownCapabilitiesSet {
		lockdown = opts.LockdownCapabilities
	}
	e := &Enforcer{
		logger:               logger,
		execer:               opts.Execer,
		resolver:             opts.Resolver,
		ports:                proxyPorts{tcp: opts.ProxyPort, dns: opts.DNSPort, udp: opts.UDPPort},
		hosts:                opts.ControlPlaneHosts,
		resolvers:            opts.Resolvers,
		lockdown:             lockdown,
		lockdownCapabilities: opts.capabilityLockdown,
		readFile:             opts.readFile,
		writeFile:            opts.writeFile,
	}
	if e.execer == nil {
		e.execer = agentexec.DefaultExecer
	}
	if e.resolver == nil {
		e.resolver = net.DefaultResolver
	}
	if e.lockdownCapabilities == nil {
		e.lockdownCapabilities = lockdownNetAdmin
	}
	if e.readFile == nil {
		e.readFile = os.ReadFile
	}
	if e.writeFile == nil {
		e.writeFile = os.WriteFile
	}
	for _, value := range e.hosts {
		if _, err := ParseExemption(value); err != nil {
			return nil, xerrors.Errorf("parse control plane exemption: %w", err)
		}
	}
	return e, nil
}

// TransparentUDP reports whether Install enabled transparent UDP relay.
func (e *Enforcer) TransparentUDP() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.transparent
}

// rule is the argument list passed to iptables after "-t <table>".
type rule []string

// ruleSet maps an iptables table to the rules for the chain owned there.
type ruleSet map[string][]rule

// managedChains are the base enforcement chains.
var managedChains = []struct{ table, chain, parent string }{
	{"nat", natChain, "OUTPUT"},
	{"filter", filterChain, "OUTPUT"},
}

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

// udpMode selects how non-DNS UDP is intercepted.
type udpMode int

const (
	udpModeTProxy udpMode = iota
	udpModeRedirect
)

// buildRules produces the base chain contents for one address family. IPv4
// TPROXY mode omits the non-DNS UDP REDIRECT rule.
func buildRules(ports proxyPorts, exemptions []resolvedExemption, resolvers []netip.Addr, family ipFamily, mode udpMode) ruleSet {
	rs := ruleSet{}
	nat := func(args ...string) { rs["nat"] = append(rs["nat"], append(rule{"-A", natChain}, args...)) }
	filter := func(args ...string) { rs["filter"] = append(rs["filter"], append(rule{"-A", filterChain}, args...)) }
	port := func(p uint16) string { return strconv.Itoa(int(p)) }

	nat("-o", "lo", "-j", "RETURN")
	for _, ex := range exemptions {
		if !family.matches(ex.Addr) {
			continue
		}
		nat("-d", ex.Addr.String(), "-p", ex.Proto, "--dport", port(ex.Port), "-j", "RETURN")
	}
	for _, addr := range resolvers {
		if family.matches(addr) {
			nat("-d", addr.String(), "-p", "tcp", "--dport", "53", "-j", "RETURN")
		}
	}
	nat("-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-ports", port(ports.dns))
	nat("-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-ports", port(ports.dns))
	if mode == udpModeRedirect || family == ipv6 {
		nat("-p", "udp", "-j", "REDIRECT", "--to-ports", port(ports.udp))
	}
	nat("-p", "tcp", "-j", "REDIRECT", "--to-ports", port(ports.tcp))

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

// buildTPROXYRules builds IPv4-only transparent UDP rules. IPv6 remains on
// the REDIRECT fallback because the proxy currently listens on IPv4 loopback.
func buildTPROXYRules(ports proxyPorts, exemptions []resolvedExemption) ruleSet {
	rs := ruleSet{}
	output := func(args ...string) {
		rs["output"] = append(rs["output"], append(rule{"-A", tproxyOutputChain}, args...))
	}
	prerouting := func(args ...string) {
		rs["prerouting"] = append(rs["prerouting"], append(rule{"-A", tproxyPreroutingChain}, args...))
	}
	port := func(p uint16) string { return strconv.Itoa(int(p)) }

	output("-m", "mark", "--mark", tproxyBypassMark, "-j", "RETURN")
	output("-o", "lo", "-j", "RETURN")
	output("-d", "127.0.0.0/8", "-j", "RETURN")
	for _, ex := range exemptions {
		if !ex.Addr.Is4() || ex.Proto != "udp" {
			continue
		}
		output("-d", ex.Addr.String(), "-p", "udp", "--dport", port(ex.Port), "-j", "RETURN")
	}
	output("-p", "udp", "--dport", "53", "-j", "RETURN")
	output("-p", "udp", "-j", "MARK", "--set-mark", tproxyMark)
	prerouting("-p", "udp", "-m", "mark", "--mark", tproxyMark,
		"-j", "TPROXY", "--on-ip", "127.0.0.1", "--on-port", port(ports.udp),
		"--tproxy-mark", tproxyMark)
	return rs
}

type resolvedExemption struct {
	Proto string
	Addr  netip.Addr
	Port  uint16
}

// resolveExemptions turns ControlPlaneHosts into concrete addresses. Hosts
// that fail to resolve are logged and skipped rather than blocking
// enforcement, since a missing exemption fails closed.
func (e *Enforcer) resolveExemptions(ctx context.Context) []resolvedExemption {
	var out []resolvedExemption
	for _, value := range e.hosts {
		exemption, err := ParseExemption(value)
		if err != nil {
			e.logger.Error(ctx, "invalid control plane exemption, not exempting it from egress enforcement",
				slog.F("exemption", value), slog.Error(err))
			continue
		}
		var addrs []netip.Addr
		if addr, err := netip.ParseAddr(exemption.Host); err == nil {
			addrs = []netip.Addr{addr}
		} else if addrs, err = e.resolver.LookupNetIP(ctx, "ip", exemption.Host); err != nil {
			e.logger.Warn(ctx, "control plane host did not resolve, not exempting it from egress enforcement",
				slog.F("host", exemption.Host), slog.Error(err))
			continue
		}
		for _, addr := range addrs {
			ex := resolvedExemption{Proto: exemption.Proto, Addr: addr.Unmap(), Port: exemption.Port}
			if !slices.Contains(out, ex) {
				out = append(out, ex)
			}
		}
	}
	return out
}
