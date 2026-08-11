package confine

import (
	"strings"
	"sync/atomic"

	"github.com/coder/coder/v2/codersdk"
)

const (
	defaultHTTPPort  = 80
	defaultHTTPSPort = 443
)

// Decision is the policy result for one destination.
type Decision struct {
	Allowed  bool
	Revision int64
}

// PolicyEngine atomically replaces a default-deny egress policy.
type PolicyEngine struct {
	controlPlaneHost string
	controlPlanePort int
	policy           atomic.Pointer[compiledPolicy]
}

type compiledPolicy struct {
	revision int64
	rules    []compiledRule
}

type compiledRule struct {
	host     string
	wildcard bool
	ports    map[int]struct{}
}

// NewPolicyEngine creates a deny-all policy with an implicit allow for the
// coderd host and port used by the supervisor and confined child.
func NewPolicyEngine(controlPlaneHost string, controlPlanePort int) *PolicyEngine {
	engine := &PolicyEngine{
		controlPlaneHost: normalizeHost(controlPlaneHost),
		controlPlanePort: controlPlanePort,
	}
	engine.Update(codersdk.AIEgressPolicy{})
	return engine
}

// Update compiles and atomically replaces the active policy. A wildcard rule
// matches exactly one leading label, so *.example.com matches a.example.com,
// but not example.com or a.b.example.com.
func (e *PolicyEngine) Update(policy codersdk.AIEgressPolicy) {
	rules := make([]compiledRule, 0, len(policy.Rules)+1)
	if e.controlPlaneHost != "" && validPort(e.controlPlanePort) {
		rules = append(rules, compiledRule{
			host:  e.controlPlaneHost,
			ports: map[int]struct{}{e.controlPlanePort: {}},
		})
	}
	for _, rule := range policy.Rules {
		if compiled, ok := compileRule(rule); ok {
			rules = append(rules, compiled)
		}
	}
	e.policy.Store(&compiledPolicy{revision: policy.Revision, rules: rules})
}

// Decide returns whether host and port are allowed by the active policy.
func (e *PolicyEngine) Decide(host string, port int) Decision {
	policy := e.policy.Load()
	if policy == nil {
		return Decision{}
	}
	host = normalizeHost(host)
	for _, rule := range policy.rules {
		if rule.matches(host, port) {
			return Decision{Allowed: true, Revision: policy.revision}
		}
	}
	return Decision{Revision: policy.revision}
}

func compileRule(rule codersdk.AIEgressRule) (compiledRule, bool) {
	host := normalizeHost(rule.Host)
	if host == "" {
		return compiledRule{}, false
	}

	wildcard := strings.HasPrefix(host, "*.")
	if wildcard {
		host = strings.TrimPrefix(host, "*.")
		if host == "" || strings.Contains(host, "*") {
			return compiledRule{}, false
		}
	} else if strings.Contains(host, "*") {
		return compiledRule{}, false
	}

	ports := make(map[int]struct{}, len(rule.Ports))
	if len(rule.Ports) == 0 {
		ports[defaultHTTPPort] = struct{}{}
		ports[defaultHTTPSPort] = struct{}{}
	} else {
		for _, port := range rule.Ports {
			if validPort(port) {
				ports[port] = struct{}{}
			}
		}
	}
	if len(ports) == 0 {
		return compiledRule{}, false
	}
	return compiledRule{host: host, wildcard: wildcard, ports: ports}, true
}

func (r compiledRule) matches(host string, port int) bool {
	if _, ok := r.ports[port]; !ok {
		return false
	}
	if !r.wildcard {
		return host == r.host
	}
	suffix := "." + r.host
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	label := strings.TrimSuffix(host, suffix)
	return label != "" && !strings.Contains(label, ".")
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(host), "."))
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}
