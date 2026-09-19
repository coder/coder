package exitnode

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/codersdk"
)

// FlowInfo describes a single outbound flow for policy evaluation.
type FlowInfo struct {
	// Protocol is tcp, udp, or dns. Empty means tcp.
	Protocol codersdk.ExitNodeProtocol
	// Host is the destination name. For tcp it comes from the CONNECT
	// target, TLS SNI, or the HTTP Host header; for udp from the CONNECT
	// target; for dns it is the query name. It is empty when no name could
	// be learned.
	Host string
	// IP is the destination address requested by the agent or resolved by
	// the exit node. It is unset for dns flows.
	IP netip.Addr
	// Port is the destination port requested by the agent. It is ignored
	// for dns flows.
	Port int
	// HostUnknown reports that the host has not been sniffed yet. While set,
	// host-based allow rules match provisionally and host-based deny rules
	// are skipped, because either could still match once the host is known.
	// An Allow decision under HostUnknown is therefore never final and must
	// be re-evaluated with the sniffed host; a deny decision is final.
	HostUnknown bool
}

// Decision is the outcome of evaluating a policy against a flow.
type Decision struct {
	Allow bool
	// RuleID identifies the matching rule, or is empty when the default
	// applied.
	RuleID string
	// Reason is a short human readable explanation suitable for logs and for
	// the X-Coder-Deny-Reason response header.
	Reason string
}

// policyFile is the YAML representation of a policy. Rules are evaluated in
// order and the first match wins. A rule matches when every criterion it
// specifies matches; criteria that are omitted match anything.
//
//	default: deny
//	rules:
//	  - id: github
//	    allow:
//	      hosts: ["github.com", "*.github.com"]
//	      ports: [443]
//	  - deny:
//	      cidrs: ["10.0.0.0/8", "192.168.0.0/16"]
//	  - id: no-quic
//	    deny:
//	      protocols: [udp]
//	      ports: [443]
//	  - allow:
//	      ports: ["8000-8999"]
//
// hosts are exact names or "*.suffix" globs; a glob matches subdomains only,
// and the bare "*" matches any known host. cidrs are prefixes or single
// addresses. ports are integers or "start-end" ranges. protocols restricts
// the rule to some of tcp, udp, and dns.
//
// tcp and udp flows are evaluated identically: hosts, cidrs, ports, and
// protocols must all match, and default applies when no rule matches.
//
// dns flows are different because the exit node resolves names on behalf of
// the workspace and a name must resolve before any tcp or udp rule can allow
// a connection to it. For a dns flow only the query name and the protocol
// are considered: cidrs and ports are ignored, and a rule is eligible only
// when it lists dns in protocols or when it has host criteria and no
// protocol restriction. The first eligible rule whose hosts match decides.
// When no rule matches, the query is allowed regardless of default. To block
// resolution of a name, add a deny rule with hosts and no protocols (which
// also blocks tcp and udp to that name) or with protocols: [dns].
type policyFile struct {
	Default string `yaml:"default"`
	Rules   []struct {
		ID    string     `yaml:"id"`
		Allow *matchFile `yaml:"allow"`
		Deny  *matchFile `yaml:"deny"`
	} `yaml:"rules"`
}

type matchFile struct {
	Hosts     []string                    `yaml:"hosts"`
	CIDRs     []string                    `yaml:"cidrs"`
	Ports     []string                    `yaml:"ports"`
	Protocols []codersdk.ExitNodeProtocol `yaml:"protocols"`
}

type rule struct {
	id    string
	allow bool
	// exact holds lowercased names; suffixes holds glob suffixes including
	// the leading dot, or "" for the bare "*" which matches any known host.
	exact, suffixes []string
	cidrs           []netip.Prefix
	// ports are inclusive [start, end] ranges.
	ports [][2]int
	// protocols is empty when the rule applies to every protocol.
	protocols []codersdk.ExitNodeProtocol
}

func (r *rule) hasHostCriteria() bool {
	return len(r.exact)+len(r.suffixes) > 0
}

func (r *rule) matchesProtocol(proto codersdk.ExitNodeProtocol) bool {
	return len(r.protocols) == 0 || slices.Contains(r.protocols, proto)
}

// matchesIPPort checks the criteria that do not depend on the host.
func (r *rule) matchesIPPort(flow FlowInfo) bool {
	ip := flow.IP.Unmap()
	return (len(r.cidrs) == 0 || slices.ContainsFunc(r.cidrs, func(p netip.Prefix) bool { return p.Contains(ip) })) &&
		(len(r.ports) == 0 || slices.ContainsFunc(r.ports, func(p [2]int) bool { return flow.Port >= p[0] && flow.Port <= p[1] }))
}

func (r *rule) matchesHost(host string) bool {
	if !r.hasHostCriteria() {
		return true
	}
	return host != "" && (slices.Contains(r.exact, host) ||
		slices.ContainsFunc(r.suffixes, func(s string) bool { return len(host) > len(s) && strings.HasSuffix(host, s) }))
}

func (r *rule) decision(reason string) Decision {
	return Decision{Allow: r.allow, RuleID: r.id, Reason: fmt.Sprintf(reason, r.id)}
}

type compiledPolicy struct {
	defaultAllow bool
	rules        []*rule
}

// Policy decides whether a flow may proceed, based on a YAML policy. It is
// safe for concurrent use and can be reloaded in place.
type Policy struct {
	path     string
	compiled atomic.Pointer[compiledPolicy]
}

// LoadPolicyFile reads and compiles the policy at path. The returned Policy
// remembers the path so Reload can re-read it.
func LoadPolicyFile(path string) (*Policy, error) {
	p := &Policy{path: path}
	if err := p.Reload(); err != nil {
		return nil, err
	}
	return p, nil
}

// ParsePolicy compiles a policy from YAML bytes. The returned Policy has no
// backing file, so Reload returns an error.
func ParsePolicy(data []byte) (*Policy, error) {
	compiled, err := compilePolicy(data)
	if err != nil {
		return nil, err
	}
	p := &Policy{}
	p.compiled.Store(compiled)
	return p, nil
}

// Reload re-reads the backing file and atomically swaps in the new rules. On
// error the previous rules stay in effect.
func (p *Policy) Reload() error {
	if p.path == "" {
		return xerrors.New("policy has no backing file to reload")
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return xerrors.Errorf("read policy file %q: %w", p.path, err)
	}
	compiled, err := compilePolicy(data)
	if err != nil {
		return xerrors.Errorf("parse policy file %q: %w", p.path, err)
	}
	p.compiled.Store(compiled)
	return nil
}

// Evaluate walks the rules in order and returns the first match, falling back
// to the default action. See FlowInfo.HostUnknown for provisional semantics
// and policyFile for how dns flows differ.
func (p *Policy) Evaluate(flow FlowInfo) Decision {
	compiled := p.compiled.Load()
	host := NormalizeHost(flow.Host)
	if flow.Protocol == codersdk.ExitNodeProtocolDNS {
		// Only the query name is matched, and no match means allow. Rules
		// that only carry cidrs or ports have nothing to say about a name
		// and are skipped unless they opt in with protocols: [dns].
		for _, r := range compiled.rules {
			if r.matchesProtocol(codersdk.ExitNodeProtocolDNS) && (len(r.protocols) > 0 || r.hasHostCriteria()) && r.matchesHost(host) {
				return r.decision("matched rule %s")
			}
		}
		return Decision{Allow: true, Reason: "no dns rule matched"}
	}
	proto := flow.Protocol
	if proto == "" {
		proto = codersdk.ExitNodeProtocolTCP
	}
	for _, r := range compiled.rules {
		if !r.matchesProtocol(proto) || !r.matchesIPPort(flow) {
			continue
		}
		if flow.HostUnknown && r.hasHostCriteria() {
			if r.allow {
				return r.decision("provisionally allowed by rule %s pending host")
			}
			// A host-based deny cannot be applied until the host is known;
			// a later rule or the default decides for now.
			continue
		}
		if r.matchesHost(host) {
			return r.decision("matched rule %s")
		}
	}
	if compiled.defaultAllow {
		return Decision{Allow: true, Reason: "default allow"}
	}
	return Decision{Reason: "default deny"}
}

// NormalizeHost lowercases a host name and strips a trailing dot and any
// port suffix so it can be compared against policy rules.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func compilePolicy(data []byte) (*compiledPolicy, error) {
	var file policyFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return nil, xerrors.Errorf("decode yaml: %w", err)
	}
	if file.Default != "" && file.Default != "allow" && file.Default != "deny" {
		return nil, xerrors.Errorf("default must be %q or %q, got %q", "allow", "deny", file.Default)
	}

	compiled := &compiledPolicy{defaultAllow: file.Default == "allow"}
	seen := make(map[string]struct{}, len(file.Rules))
	for i, rf := range file.Rules {
		id := rf.ID
		if id == "" {
			id = fmt.Sprintf("rule-%d", i+1)
		}
		if _, dup := seen[id]; dup {
			return nil, xerrors.Errorf("rule %d: duplicate rule id %q", i, id)
		}
		seen[id] = struct{}{}

		r := &rule{id: id, allow: rf.Allow != nil}
		match := rf.Allow
		switch {
		case rf.Allow != nil && rf.Deny != nil:
			return nil, xerrors.Errorf("rule %s: specify either allow or deny, not both", id)
		case rf.Deny != nil:
			match = rf.Deny
		case rf.Allow == nil:
			return nil, xerrors.Errorf("rule %s: must specify allow or deny", id)
		}
		if err := r.compile(match); err != nil {
			return nil, xerrors.Errorf("rule %s: %w", id, err)
		}
		compiled.rules = append(compiled.rules, r)
	}
	return compiled, nil
}

func (r *rule) compile(match *matchFile) error {
	for _, raw := range match.Hosts {
		pattern := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(raw, ".")))
		suffix, glob := strings.CutPrefix(pattern, "*")
		switch {
		case pattern == "":
			return xerrors.New("empty host pattern")
		case glob && (suffix == "" || (strings.HasPrefix(suffix, ".") && suffix != "." && !strings.Contains(suffix, "*"))):
			r.suffixes = append(r.suffixes, suffix)
		case strings.Contains(pattern, "*"):
			return xerrors.Errorf("invalid host glob %q: only a leading \"*.\" is supported", raw)
		default:
			r.exact = append(r.exact, pattern)
		}
	}
	for _, raw := range match.CIDRs {
		raw = strings.TrimSpace(raw)
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			addr, addrErr := netip.ParseAddr(raw)
			if addrErr != nil {
				return xerrors.Errorf("invalid cidr %q: %w", raw, err)
			}
			prefix = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
		}
		r.cidrs = append(r.cidrs, prefix.Masked())
	}
	for _, raw := range match.Ports {
		startStr, endStr, isRange := strings.Cut(strings.TrimSpace(raw), "-")
		start, err := parsePort(startStr)
		if err != nil {
			return err
		}
		end := start
		if isRange {
			if end, err = parsePort(endStr); err != nil {
				return err
			}
			if end < start {
				return xerrors.Errorf("invalid port range %q: end before start", raw)
			}
		}
		r.ports = append(r.ports, [2]int{start, end})
	}
	for _, raw := range match.Protocols {
		proto := codersdk.ExitNodeProtocol(strings.ToLower(strings.TrimSpace(string(raw))))
		switch proto {
		case codersdk.ExitNodeProtocolTCP, codersdk.ExitNodeProtocolUDP, codersdk.ExitNodeProtocolDNS:
			r.protocols = append(r.protocols, proto)
		default:
			return xerrors.Errorf("invalid protocol %q: must be tcp, udp, or dns", raw)
		}
	}
	if !r.hasHostCriteria() && len(r.cidrs) == 0 && len(r.ports) == 0 && len(r.protocols) == 0 {
		return xerrors.New("must specify at least one of hosts, cidrs, ports, or protocols")
	}
	return nil
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, xerrors.Errorf("invalid port %q: %w", s, err)
	}
	if port < 1 || port > 65535 {
		return 0, xerrors.Errorf("invalid port %d: must be between 1 and 65535", port)
	}
	return port, nil
}
