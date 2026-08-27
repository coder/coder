package confine

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coder-sandbox/policy"
	"github.com/coder/coder/v2/codersdk"
)

func TestPolicyEvaluatorMatchesPolicyEngine(t *testing.T) {
	t.Parallel()

	engine := NewPolicyEngine("coder.example.com", 8443)
	engine.Update(codersdk.AIEgressPolicy{
		Revision: 7,
		Rules: []codersdk.AIEgressRule{
			{Host: "example.com"},
			{Host: "*.services.example.com", Ports: []int{1234}},
			{Host: "192.0.2.10", Ports: []int{8080}},
		},
	})
	evaluator := newPolicyEvaluator(engine, DestinationOptions{
		LookupNetIP: evaluatorLookup(map[string][]netip.Addr{
			"example.com":              {netip.MustParseAddr("203.0.113.10")},
			"api.services.example.com": {netip.MustParseAddr("203.0.113.11")},
			"services.example.com":     {netip.MustParseAddr("203.0.113.12")},
			"a.b.services.example.com": {netip.MustParseAddr("203.0.113.13")},
			"coder.example.com":        {netip.MustParseAddr("203.0.113.14")},
		}),
	})

	tests := []struct {
		name string
		host string
		port uint16
	}{
		{name: "exact default http", host: "example.com", port: 80},
		{name: "exact default https", host: "EXAMPLE.COM.", port: 443},
		{name: "exact other port", host: "example.com", port: 22},
		{name: "wildcard one label", host: "api.services.example.com", port: 1234},
		{name: "wildcard apex", host: "services.example.com", port: 1234},
		{name: "wildcard multiple labels", host: "a.b.services.example.com", port: 1234},
		{name: "wildcard wrong port", host: "api.services.example.com", port: 443},
		{name: "literal IP", host: "192.0.2.10", port: 8080},
		{name: "implicit control plane", host: "coder.example.com", port: 8443},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			want := engine.Decide(tt.host, int(tt.port))
			got, err := evaluator.EvaluateName(t.Context(), tt.host, tt.port)
			require.NoError(t, err)
			require.NoError(t, policy.ValidateNetworkDecision(got))
			require.Equal(t, want.Allowed, got.Action == policy.ActionAllow)
			require.Equal(t, want.Revision, got.Generation)
			require.Equal(t, policy.TLSPassthrough, got.TLS)
		})
	}
}

func TestPolicyEvaluatorAlwaysAllowsControlChannel(t *testing.T) {
	t.Parallel()

	newEvaluator := func() (*PolicyEngine, *policyEvaluator) {
		engine := NewPolicyEngine("", 0)
		engine.Update(codersdk.AIEgressPolicy{
			Revision: 1,
			Rules: []codersdk.AIEgressRule{{
				Host:  "policy.example.com",
				Ports: []int{443},
			}},
		})
		return engine, newPolicyEvaluator(engine, DestinationOptions{
			LookupNetIP: evaluatorLookup(map[string][]netip.Addr{
				"coder.example.com":  {netip.MustParseAddr("127.0.0.1")},
				"policy.example.com": {netip.MustParseAddr("203.0.113.10")},
				"denied.example.com": {netip.MustParseAddr("203.0.113.11")},
			}),
			AllowPrivateHost: "different.example.com",
			AlwaysAllowHost:  "CODER.Example.COM.",
			AlwaysAllowPort:  8443,
		})
	}

	tests := []struct {
		name    string
		host    string
		port    uint16
		allowed bool
	}{
		{name: "normalized control channel", host: "Coder.Example.Com.", port: 8443, allowed: true},
		{name: "control channel wrong port", host: "coder.example.com", port: 443, allowed: false},
		{name: "unaffected policy allow", host: "policy.example.com", port: 443, allowed: true},
		{name: "unaffected policy deny", host: "denied.example.com", port: 443, allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, evaluator := newEvaluator()
			decision, err := evaluator.EvaluateName(t.Context(), tt.host, tt.port)
			require.NoError(t, err)
			require.Equal(t, tt.allowed, decision.Action == policy.ActionAllow)
			require.EqualValues(t, 1, decision.Generation)
		})
	}

	engine, evaluator := newEvaluator()
	engine.Update(codersdk.AIEgressPolicy{Revision: 2})
	decision, err := evaluator.EvaluateName(t.Context(), "CODER.EXAMPLE.COM.", 8443)
	require.NoError(t, err)
	require.Equal(t, policy.ActionAllow, decision.Action)
	require.EqualValues(t, 2, decision.Generation)
	require.Contains(t, decision.Reason, "platform control channel")

	decision, err = evaluator.EvaluateName(t.Context(), "policy.example.com", 443)
	require.NoError(t, err)
	require.Equal(t, policy.ActionDeny, decision.Action)
	require.EqualValues(t, 2, decision.Generation)
}

func TestPolicyEvaluatorDestinationValidation(t *testing.T) {
	t.Parallel()

	engine := NewPolicyEngine("", 0)
	engine.Update(codersdk.AIEgressPolicy{
		Revision: 8,
		Rules: []codersdk.AIEgressRule{
			{Host: "0.0.0.1", Ports: []int{443}},
			{Host: "224.0.0.1", Ports: []int{443}},
			{Host: "::", Ports: []int{443}},
			{Host: "ff02::1", Ports: []int{443}},
			{Host: "blocked.example", Ports: []int{443}},
			{Host: "mixed.example", Ports: []int{443}},
			{Host: "private.example", Ports: []int{443}},
		},
	})
	lookup := evaluatorLookup(map[string][]netip.Addr{
		"blocked.example": {netip.MustParseAddr("127.0.0.1")},
		"mixed.example": {
			netip.MustParseAddr("203.0.113.20"),
			netip.MustParseAddr("224.0.0.1"),
		},
		"private.example": {netip.MustParseAddr("127.0.0.1")},
	})

	tests := []struct {
		name    string
		host    string
		options DestinationOptions
		allowed bool
	}{
		{name: "literal zero network", host: "0.0.0.1"},
		{name: "literal IPv4 multicast", host: "224.0.0.1"},
		{name: "literal IPv6 unspecified", host: "::"},
		{name: "literal IPv6 multicast", host: "ff02::1"},
		{name: "resolved loopback", host: "blocked.example", options: DestinationOptions{LookupNetIP: lookup}},
		{name: "any denied resolved address", host: "mixed.example", options: DestinationOptions{LookupNetIP: lookup}},
		{
			name: "exact private host exemption", host: "private.example", allowed: true,
			options: DestinationOptions{LookupNetIP: lookup, AllowPrivateHost: "PRIVATE.EXAMPLE."},
		},
		{name: "private exemption does not cover another host", host: "blocked.example", options: DestinationOptions{LookupNetIP: lookup, AllowPrivateHost: "private.example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evaluator := newPolicyEvaluator(engine, tt.options)
			decision, err := evaluator.EvaluateName(t.Context(), tt.host, 443)
			require.NoError(t, err)
			require.NoError(t, policy.ValidateNetworkDecision(decision))
			require.Equal(t, tt.allowed, decision.Action == policy.ActionAllow)
			require.EqualValues(t, 8, decision.Generation)
			require.Equal(t, policy.TLSPassthrough, decision.TLS)
		})
	}
}

func TestPolicyEvaluatorResolvedIP(t *testing.T) {
	t.Parallel()

	engine := NewPolicyEngine("", 0)
	engine.Update(codersdk.AIEgressPolicy{
		Revision: 9,
		Rules:    []codersdk.AIEgressRule{{Host: "allowed.example", Ports: []int{443}}},
	})

	tests := []struct {
		name    string
		options DestinationOptions
		address netip.Addr
		allowed bool
	}{
		{name: "public", address: netip.MustParseAddr("203.0.113.30"), allowed: true},
		{name: "private", address: netip.MustParseAddr("10.0.0.1")},
		{name: "zero network", address: netip.MustParseAddr("0.0.0.1")},
		{name: "multicast", address: netip.MustParseAddr("224.0.0.1")},
		{
			name: "private exemption", address: netip.MustParseAddr("10.0.0.1"), allowed: true,
			options: DestinationOptions{AllowPrivateHost: "allowed.example"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evaluator := newPolicyEvaluator(engine, tt.options)
			decision, err := evaluator.EvaluateResolvedIP(t.Context(), "allowed.example", 443, tt.address)
			require.NoError(t, err)
			require.NoError(t, policy.ValidateNetworkDecision(decision))
			require.Equal(t, tt.allowed, decision.Action == policy.ActionAllow)
			require.EqualValues(t, 9, decision.Generation)
			require.Equal(t, policy.TLSPassthrough, decision.TLS)
		})
	}
}

func TestPolicyEvaluatorGenerationAndMCP(t *testing.T) {
	t.Parallel()

	engine := NewPolicyEngine("", 0)
	evaluator := newPolicyEvaluator(engine, DestinationOptions{})
	require.EqualValues(t, 0, evaluator.Generation())

	engine.Update(codersdk.AIEgressPolicy{Revision: 1})
	require.EqualValues(t, 1, evaluator.Generation())

	hasEndpoint, err := evaluator.HasMCPEndpoint(t.Context(), "example.com", 443)
	require.NoError(t, err)
	require.False(t, hasEndpoint)
	isEndpoint, err := evaluator.IsMCPEndpoint(t.Context(), "example.com", 443, "/mcp")
	require.NoError(t, err)
	require.False(t, isEndpoint)
	decision, err := evaluator.EvaluateMCP(t.Context(), policy.MCPCall{})
	require.NoError(t, err)
	require.NoError(t, policy.ValidateMCPDecision(decision))
	require.Equal(t, policy.ActionDeny, decision.Action)
	require.EqualValues(t, 1, decision.Generation)
	require.Equal(t, policy.TLSPassthrough, decision.TLS)
	require.Contains(t, decision.Reason, "not supported")
}

// TestPolicyEvaluatorMCPAllowsControlChannel pins the one destination whose
// MCP tool calls survive inspection. Coder's MCP gateway terminates control
// channel traffic and enforces the server's tool policy itself, so denying
// here would break every sandboxed MCP client while adding no containment.
// Everything else still fails closed: the egress policy has no tool rules.
func TestPolicyEvaluatorMCPAllowsControlChannel(t *testing.T) {
	t.Parallel()

	engine := NewPolicyEngine("coder.example.com", 8443)
	engine.Update(codersdk.AIEgressPolicy{
		Revision: 4,
		// A network-allowed destination that is not the control channel:
		// reachable, but still not MCP-inspectable.
		Rules: []codersdk.AIEgressRule{{Host: "api.githubcopilot.com", Ports: []int{443}}},
	})
	evaluator := newPolicyEvaluator(engine, DestinationOptions{
		AlwaysAllowHost: "CODER.Example.COM.",
		AlwaysAllowPort: 8443,
	})

	tests := []struct {
		name    string
		host    string
		port    uint16
		allowed bool
	}{
		{name: "control channel", host: "coder.example.com", port: 8443, allowed: true},
		{name: "control channel normalized", host: "Coder.Example.Com.", port: 8443, allowed: true},
		{name: "control channel wrong port", host: "coder.example.com", port: 443, allowed: false},
		{name: "network allowed but not control channel", host: "api.githubcopilot.com", port: 443, allowed: false},
		{name: "unrelated host", host: "evil.example.com", port: 443, allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			decision, err := evaluator.EvaluateMCP(t.Context(), policy.MCPCall{
				Host: tt.host,
				Port: tt.port,
				Path: "/api/v2/ai-gateway/mcp/github",
				Tool: "get_me",
			})
			require.NoError(t, err)
			require.NoError(t, policy.ValidateMCPDecision(decision))
			require.EqualValues(t, 4, decision.Generation)
			require.Equal(t, decision.Action, decision.Verdict)

			if tt.allowed {
				require.Equal(t, policy.ActionAllow, decision.Action)
				require.Contains(t, decision.Reason, "control channel")
				return
			}
			require.Equal(t, policy.ActionDeny, decision.Action)
			require.Contains(t, decision.Reason, "not supported")
		})
	}
}

func TestPolicyEvaluatorResolutionFailureDenies(t *testing.T) {
	t.Parallel()

	engine := NewPolicyEngine("", 0)
	engine.Update(codersdk.AIEgressPolicy{
		Revision: 10,
		Rules:    []codersdk.AIEgressRule{{Host: "unresolved.example", Ports: []int{443}}},
	})
	evaluator := newPolicyEvaluator(engine, DestinationOptions{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			return nil, xerrors.New("resolver unavailable")
		},
	})
	decision, err := evaluator.EvaluateName(t.Context(), "unresolved.example", 443)
	require.NoError(t, err)
	require.Equal(t, policy.ActionDeny, decision.Action)
	require.EqualValues(t, 10, decision.Generation)
}

func evaluatorLookup(addresses map[string][]netip.Addr) func(context.Context, string, string) ([]netip.Addr, error) {
	return func(_ context.Context, network, host string) ([]netip.Addr, error) {
		if network != "ip" {
			return nil, xerrors.New("unexpected lookup network")
		}
		resolved, ok := addresses[normalizeHost(host)]
		if !ok {
			return nil, xerrors.New("host not found")
		}
		return append([]netip.Addr(nil), resolved...), nil
	}
}
