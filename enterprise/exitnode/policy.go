package exitnode

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// FlowInfo describes a single outbound TCP flow for policy evaluation.
type FlowInfo struct {
	// Host is the destination name learned from TLS SNI or the HTTP Host
	// header. It is empty when no name could be learned.
	Host string
	// IP is the destination address requested by the agent.
	IP netip.Addr
	// Port is the destination port requested by the agent.
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

// PolicyEvaluator decides whether a flow may proceed. Implementations must be
// safe for concurrent use. The interface exists so that an external engine
// such as Envoy or OPA can replace the built-in YAML policy.
type PolicyEvaluator interface {
	Evaluate(FlowInfo) Decision
}

// Reloader is implemented by policy evaluators whose configuration can be
// re-read at runtime, for example on SIGHUP.
type Reloader interface {
	Reload() error
}

// Action is the outcome a rule or default produces.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// PolicyFile is the YAML representation of a policy. Rules are evaluated in
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
//	  - allow:
//	      ports: ["8000-8999"]
type PolicyFile struct {
	Default Action     `yaml:"default"`
	Rules   []RuleFile `yaml:"rules"`
}

// RuleFile is one YAML rule. Exactly one of Allow or Deny must be set.
type RuleFile struct {
	ID    string     `yaml:"id"`
	Allow *MatchFile `yaml:"allow"`
	Deny  *MatchFile `yaml:"deny"`
}

// MatchFile lists the criteria a rule matches on.
type MatchFile struct {
	// Hosts are exact names or "*.suffix" globs. A glob matches subdomains
	// only, so "*.example.com" does not match "example.com".
	Hosts []string `yaml:"hosts"`
	// CIDRs are prefixes or single addresses.
	CIDRs []string `yaml:"cidrs"`
	// Ports are integers or "start-end" ranges.
	Ports []PortRange `yaml:"ports"`
}

// PortRange is an inclusive port range. A single port is a range with equal
// ends. It unmarshals from either a YAML integer or a "start-end" string.
type PortRange struct {
	Start int
	End   int
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (p *PortRange) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return xerrors.Errorf("port must be an integer or \"start-end\": %w", err)
	}
	pr, err := ParsePortRange(raw)
	if err != nil {
		return err
	}
	*p = pr
	return nil
}

// ParsePortRange parses "443" or "8000-8999".
func ParsePortRange(raw string) (PortRange, error) {
	startStr, endStr, isRange := strings.Cut(strings.TrimSpace(raw), "-")
	start, err := parsePort(startStr)
	if err != nil {
		return PortRange{}, err
	}
	end := start
	if isRange {
		end, err = parsePort(endStr)
		if err != nil {
			return PortRange{}, err
		}
		if end < start {
			return PortRange{}, xerrors.Errorf("invalid port range %q: end before start", raw)
		}
	}
	return PortRange{Start: start, End: end}, nil
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

// Contains reports whether port falls inside the range.
func (p PortRange) Contains(port int) bool {
	return port >= p.Start && port <= p.End
}

type hostMatcher struct {
	// exact is the lowercased name for exact rules, or the lowercased
	// suffix including the leading dot for glob rules.
	exact  string
	suffix string
}

func (m hostMatcher) matches(host string) bool {
	if m.suffix != "" {
		return len(host) > len(m.suffix) && strings.HasSuffix(host, m.suffix)
	}
	return host == m.exact
}

type compiledRule struct {
	id     string
	action Action
	hosts  []hostMatcher
	// anyHost is set for the bare "*" glob, which matches any known host.
	anyHost bool
	cidrs   []netip.Prefix
	ports   []PortRange
}

func (r *compiledRule) hasHostCriteria() bool {
	return r.anyHost || len(r.hosts) > 0
}

// matchesIPPort checks the criteria that do not depend on the host.
func (r *compiledRule) matchesIPPort(flow FlowInfo) bool {
	if len(r.cidrs) > 0 {
		ip := flow.IP.Unmap()
		matched := false
		for _, cidr := range r.cidrs {
			if cidr.Contains(ip) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(r.ports) > 0 {
		matched := false
		for _, pr := range r.ports {
			if pr.Contains(flow.Port) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (r *compiledRule) matchesHost(host string) bool {
	if !r.hasHostCriteria() {
		return true
	}
	if host == "" {
		return false
	}
	if r.anyHost {
		return true
	}
	for _, m := range r.hosts {
		if m.matches(host) {
			return true
		}
	}
	return false
}

type compiledPolicy struct {
	def   Action
	rules []*compiledRule
}

// Policy is a PolicyEvaluator backed by a YAML file. It is safe for
// concurrent use and can be reloaded in place.
type Policy struct {
	path string

	mu       sync.RWMutex
	compiled *compiledPolicy
}

var _ PolicyEvaluator = (*Policy)(nil)

var _ Reloader = (*Policy)(nil)

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
	return &Policy{compiled: compiled}, nil
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
	p.mu.Lock()
	p.compiled = compiled
	p.mu.Unlock()
	return nil
}

// Evaluate walks the rules in order and returns the first match, falling back
// to the default action. See FlowInfo.HostUnknown for provisional semantics.
func (p *Policy) Evaluate(flow FlowInfo) Decision {
	p.mu.RLock()
	compiled := p.compiled
	p.mu.RUnlock()

	host := NormalizeHost(flow.Host)
	for _, rule := range compiled.rules {
		if !rule.matchesIPPort(flow) {
			continue
		}
		if flow.HostUnknown && rule.hasHostCriteria() {
			if rule.action == ActionAllow {
				return Decision{
					Allow:  true,
					RuleID: rule.id,
					Reason: fmt.Sprintf("provisionally allowed by rule %s pending host", rule.id),
				}
			}
			// A host-based deny cannot be applied until the host is known;
			// a later rule or the default decides for now.
			continue
		}
		if !rule.matchesHost(host) {
			continue
		}
		return Decision{
			Allow:  rule.action == ActionAllow,
			RuleID: rule.id,
			Reason: fmt.Sprintf("matched rule %s", rule.id),
		}
	}
	return Decision{
		Allow:  compiled.def == ActionAllow,
		Reason: fmt.Sprintf("default %s", compiled.def),
	}
}

// NormalizeHost lowercases a host name and strips a trailing dot and any
// port suffix so it can be compared against policy rules.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	// Strip a port if present. IPv6 literals are bracketed so SplitHostPort
	// handles them; bare names without a port fail and are used as is.
	if h, _, err := splitHostPortLenient(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

func splitHostPortLenient(hostport string) (host, port string, err error) {
	if strings.HasPrefix(hostport, "[") {
		end := strings.IndexByte(hostport, ']')
		if end < 0 {
			return "", "", xerrors.New("missing ]")
		}
		host = hostport[1:end]
		rest := hostport[end+1:]
		if rest == "" {
			return host, "", nil
		}
		if rest[0] != ':' {
			return "", "", xerrors.New("unexpected characters after ]")
		}
		return host, rest[1:], nil
	}
	// A single colon separates host and port; more than one means an
	// unbracketed IPv6 address with no port.
	if strings.Count(hostport, ":") != 1 {
		return "", "", xerrors.New("no port")
	}
	host, port, _ = strings.Cut(hostport, ":")
	return host, port, nil
}

func compilePolicy(data []byte) (*compiledPolicy, error) {
	var file PolicyFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return nil, xerrors.Errorf("decode yaml: %w", err)
	}

	def := file.Default
	if def == "" {
		def = ActionDeny
	}
	if def != ActionAllow && def != ActionDeny {
		return nil, xerrors.Errorf("default must be %q or %q, got %q", ActionAllow, ActionDeny, def)
	}

	compiled := &compiledPolicy{def: def}
	seen := make(map[string]struct{}, len(file.Rules))
	for i, rf := range file.Rules {
		rule, err := compileRule(i, rf)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[rule.id]; dup {
			return nil, xerrors.Errorf("rule %d: duplicate rule id %q", i, rule.id)
		}
		seen[rule.id] = struct{}{}
		compiled.rules = append(compiled.rules, rule)
	}
	return compiled, nil
}

func compileRule(index int, rf RuleFile) (*compiledRule, error) {
	id := rf.ID
	if id == "" {
		id = fmt.Sprintf("rule-%d", index+1)
	}
	rule := &compiledRule{id: id}

	var match *MatchFile
	switch {
	case rf.Allow != nil && rf.Deny != nil:
		return nil, xerrors.Errorf("rule %s: specify either allow or deny, not both", id)
	case rf.Allow != nil:
		rule.action = ActionAllow
		match = rf.Allow
	case rf.Deny != nil:
		rule.action = ActionDeny
		match = rf.Deny
	default:
		return nil, xerrors.Errorf("rule %s: must specify allow or deny", id)
	}

	for _, raw := range match.Hosts {
		pattern := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(raw, ".")))
		switch {
		case pattern == "":
			return nil, xerrors.Errorf("rule %s: empty host pattern", id)
		case pattern == "*":
			rule.anyHost = true
		case strings.HasPrefix(pattern, "*."):
			suffix := pattern[1:]
			if suffix == "." || strings.Contains(suffix, "*") {
				return nil, xerrors.Errorf("rule %s: invalid host glob %q", id, raw)
			}
			rule.hosts = append(rule.hosts, hostMatcher{suffix: suffix})
		case strings.Contains(pattern, "*"):
			return nil, xerrors.Errorf("rule %s: invalid host glob %q: only a leading \"*.\" is supported", id, raw)
		default:
			rule.hosts = append(rule.hosts, hostMatcher{exact: pattern})
		}
	}

	for _, raw := range match.CIDRs {
		raw = strings.TrimSpace(raw)
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			addr, addrErr := netip.ParseAddr(raw)
			if addrErr != nil {
				return nil, xerrors.Errorf("rule %s: invalid cidr %q: %w", id, raw, err)
			}
			addr = addr.Unmap()
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		rule.cidrs = append(rule.cidrs, prefix.Masked())
	}

	rule.ports = append(rule.ports, match.Ports...)

	if !rule.hasHostCriteria() && len(rule.cidrs) == 0 && len(rule.ports) == 0 {
		return nil, xerrors.Errorf("rule %s: must specify at least one of hosts, cidrs, or ports", id)
	}
	return rule, nil
}
