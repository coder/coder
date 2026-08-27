package confine

import (
	"context"
	"net"
	"net/netip"
	"strings"

	"github.com/coder/coder/coder-sandbox/policy"
)

type policyEvaluator struct {
	engine           *PolicyEngine
	lookupNetIP      func(context.Context, string, string) ([]netip.Addr, error)
	allowPrivateHost string
	alwaysAllowHost  string
	alwaysAllowPort  int
}

func newPolicyEvaluator(engine *PolicyEngine, options DestinationOptions) *policyEvaluator {
	lookupNetIP := options.LookupNetIP
	if lookupNetIP == nil {
		lookupNetIP = net.DefaultResolver.LookupNetIP
	}
	alwaysAllowHost := normalizeHost(options.AlwaysAllowHost)
	allowPrivateHost := normalizeHost(options.AllowPrivateHost)
	if alwaysAllowHost != "" {
		allowPrivateHost = alwaysAllowHost
	}
	alwaysAllowPort := options.AlwaysAllowPort
	if !validPort(alwaysAllowPort) {
		alwaysAllowPort = 0
	}
	return &policyEvaluator{
		engine:           engine,
		lookupNetIP:      lookupNetIP,
		allowPrivateHost: allowPrivateHost,
		alwaysAllowHost:  alwaysAllowHost,
		alwaysAllowPort:  alwaysAllowPort,
	}
}

// Generation implements policy.Evaluator.
func (e *policyEvaluator) Generation() int64 {
	if e == nil || e.engine == nil {
		return 0
	}
	current := e.engine.policy.Load()
	if current == nil {
		return 0
	}
	return current.revision
}

// EvaluateName implements policy.Evaluator.
func (e *policyEvaluator) EvaluateName(ctx context.Context, host string, port uint16) (policy.Decision, error) {
	if err := ctx.Err(); err != nil {
		return policy.Decision{}, err
	}
	if e == nil {
		return networkPolicyDecision(policy.ActionDeny, 0, "egress policy is unavailable"), nil
	}

	host = normalizeHost(host)
	decision, controlChannel := e.decide(host, port)
	if !decision.Allowed {
		return networkPolicyDecision(policy.ActionDeny, decision.Revision, "destination is not allowed by the AI egress policy"), nil
	}
	allowedReason := "destination is allowed by the AI egress policy"
	if controlChannel {
		allowedReason = "destination is the platform control channel"
	}

	literalHost := strings.Trim(strings.TrimSpace(host), "[]")
	if address, err := netip.ParseAddr(literalHost); err == nil {
		if !e.privateHostAllowed(host) && deniedDestination(address) {
			return networkPolicyDecision(policy.ActionDeny, decision.Revision, "destination address is in a denied range"), nil
		}
		return networkPolicyDecision(policy.ActionAllow, decision.Revision, allowedReason), nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, destinationTimeout)
	defer cancel()
	addresses, err := e.lookupNetIP(lookupCtx, "ip", host)
	if err != nil {
		return networkPolicyDecision(policy.ActionDeny, decision.Revision, "destination name could not be resolved"), nil
	}
	if len(addresses) == 0 {
		return networkPolicyDecision(policy.ActionDeny, decision.Revision, "destination name resolved to no addresses"), nil
	}
	if !e.privateHostAllowed(host) {
		// The sandbox proxy performs a second resolved-IP evaluation for IPv4
		// loopback, RFC1918, CGNAT and link-local ranges, plus IPv6 loopback,
		// private and link-local ranges. Its sensitive-prefix list omits
		// 0.0.0.0/8, 224.0.0.0/4, ::/128 and ff00::/8. Checking every answer
		// here blocks stable resolutions into those ranges, but a resolver change
		// between this check and the library's dial remains possible for them.
		for _, address := range addresses {
			if !address.IsValid() || deniedDestination(address) {
				return networkPolicyDecision(policy.ActionDeny, decision.Revision, "destination name resolved to a denied address"), nil
			}
		}
	}
	return networkPolicyDecision(policy.ActionAllow, decision.Revision, allowedReason), nil
}

// EvaluateResolvedIP implements policy.Evaluator.
func (e *policyEvaluator) EvaluateResolvedIP(ctx context.Context, host string, port uint16, address netip.Addr) (policy.Decision, error) {
	if err := ctx.Err(); err != nil {
		return policy.Decision{}, err
	}
	if e == nil {
		return networkPolicyDecision(policy.ActionDeny, 0, "egress policy is unavailable"), nil
	}

	host = normalizeHost(host)
	decision, controlChannel := e.decide(host, port)
	if !decision.Allowed {
		return networkPolicyDecision(policy.ActionDeny, decision.Revision, "destination is not allowed by the AI egress policy"), nil
	}
	if !address.IsValid() || !e.privateHostAllowed(host) && deniedDestination(address) {
		return networkPolicyDecision(policy.ActionDeny, decision.Revision, "resolved destination address is in a denied range"), nil
	}
	allowedReason := "resolved destination is allowed by the AI egress policy"
	if controlChannel {
		allowedReason = "resolved destination is the platform control channel"
	}
	return networkPolicyDecision(policy.ActionAllow, decision.Revision, allowedReason), nil
}

// HasMCPEndpoint implements policy.Evaluator.
func (*policyEvaluator) HasMCPEndpoint(ctx context.Context, _ string, _ uint16) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return false, nil
}

// IsMCPEndpoint implements policy.Evaluator.
func (*policyEvaluator) IsMCPEndpoint(ctx context.Context, _ string, _ uint16, _ string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return false, nil
}

// EvaluateMCP implements policy.Evaluator.
//
// The AI egress proxy cannot evaluate MCP tool calls on their merits: the
// template egress policy carries network rules only (see
// codersdk.AIEgressRule), so there are no tool-level rules to consult and
// any inspected call must fail closed.
//
// Calls to the platform control channel are the exception. That traffic
// terminates at Coder's own MCP gateway, which resolves the upstream
// credential and applies the server's tool rules, allow and deny lists and
// escalation flow before forwarding anything upstream. Denying it here
// would break the only sanctioned MCP path out of the sandbox without
// adding containment, because the gateway has already made the decision.
// Reaching the control channel at all still requires passing the network
// evaluation above.
func (e *policyEvaluator) EvaluateMCP(ctx context.Context, call policy.MCPCall) (policy.Decision, error) {
	if err := ctx.Err(); err != nil {
		return policy.Decision{}, err
	}
	action := policy.ActionDeny
	reason := "MCP inspection is not supported by the AI egress proxy"
	if e != nil && e.controlChannelAllowed(call.Host, call.Port) {
		action = policy.ActionAllow
		reason = "MCP call targets the platform control channel, which enforces its own tool policy"
	}
	return policy.Decision{
		Action:     action,
		Verdict:    action,
		Mode:       policy.ModeEnforce,
		Generation: e.Generation(),
		TLS:        policy.TLSPassthrough,
		Reason:     reason,
	}, nil
}

func (e *policyEvaluator) decide(host string, port uint16) (Decision, bool) {
	if e.controlChannelAllowed(host, port) {
		return Decision{Allowed: true, Revision: e.Generation()}, true
	}
	if e.engine == nil {
		return Decision{}, false
	}
	return e.engine.Decide(host, int(port)), false
}

func (e *policyEvaluator) controlChannelAllowed(host string, port uint16) bool {
	return e.alwaysAllowHost != "" && normalizeHost(host) == e.alwaysAllowHost && int(port) == e.alwaysAllowPort
}

func (e *policyEvaluator) privateHostAllowed(host string) bool {
	return e.allowPrivateHost != "" && normalizeHost(host) == e.allowPrivateHost
}

func networkPolicyDecision(action policy.Action, generation int64, reason string) policy.Decision {
	return policy.Decision{
		Action:     action,
		Verdict:    action,
		Mode:       policy.ModeEnforce,
		Generation: generation,
		TLS:        policy.TLSPassthrough,
		Reason:     reason,
	}
}

var _ policy.Evaluator = (*policyEvaluator)(nil)
