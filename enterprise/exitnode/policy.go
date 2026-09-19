package exitnode

import (
	"net"
	"net/netip"
	"strings"

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

// Policy decides whether a flow may proceed. It is the extension point for
// embedding the exit node with custom rules: implementations must be safe for
// concurrent use and should return quickly, because Evaluate runs on the path
// of every CONNECT request and every dns query. Implementations that also
// provide a Reload() error method are reloadable through Server.ReloadPolicy.
//
// The yamlpolicy package provides the rule-based implementation the CLI uses.
type Policy interface {
	Evaluate(FlowInfo) Decision
}

// PolicyFunc adapts a function to the Policy interface.
type PolicyFunc func(FlowInfo) Decision

// Evaluate implements Policy.
func (f PolicyFunc) Evaluate(flow FlowInfo) Decision { return f(flow) }

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
