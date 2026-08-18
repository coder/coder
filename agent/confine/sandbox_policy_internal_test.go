package confine

import (
	"context"
	"net/netip"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/coder-sandbox/policy"
	"github.com/coder/coder/v2/codersdk"
)

type sandboxPolicyResolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f sandboxPolicyResolverFunc) LookupNetIP(
	ctx context.Context,
	network string,
	host string,
) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestBuildSandboxRuntimeNetwork(t *testing.T) {
	t.Parallel()

	publicResolver := sandboxPolicyResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	})
	accessURL := mustParseURL(t, "https://coder.example")
	tests := []struct {
		name       string
		policy     codersdk.AIEgressPolicy
		accessURL  *url.URL
		resolver   sandboxPolicyResolver
		assertions func(*testing.T, sandboxRuntimeNetwork, error)
	}{
		{
			name: "EmptyPorts",
			policy: codersdk.AIEgressPolicy{Rules: []codersdk.AIEgressRule{{
				Host: "packages.example",
			}}},
			accessURL: accessURL,
			resolver:  publicResolver,
			assertions: func(t *testing.T, network sandboxRuntimeNetwork, err error) {
				require.NoError(t, err)
				rule := requireSandboxHostRule(t, network.Rules, "packages.example")
				require.Equal(t, []uint16{80, 443}, rule.Ports)
			},
		},
		{
			name: "WildcardPassesThrough",
			policy: codersdk.AIEgressPolicy{Rules: []codersdk.AIEgressRule{{
				Host:  "*.example.com",
				Ports: []int{443},
			}}},
			accessURL: accessURL,
			resolver:  publicResolver,
			assertions: func(t *testing.T, network sandboxRuntimeNetwork, err error) {
				require.NoError(t, err)
				rule := requireSandboxHostRule(t, network.Rules, "*.example.com")
				require.Equal(t, []uint16{443}, rule.Ports)
			},
		},
		{
			name: "LiteralIPAddressUsesCIDR",
			policy: codersdk.AIEgressPolicy{Rules: []codersdk.AIEgressRule{{
				Host:  "192.0.2.8",
				Ports: []int{8443},
			}}},
			accessURL: accessURL,
			resolver:  publicResolver,
			assertions: func(t *testing.T, network sandboxRuntimeNetwork, err error) {
				require.NoError(t, err)
				rule := requireSandboxCIDRRule(t, network.Rules, "192.0.2.8/32")
				require.Equal(t, []uint16{8443}, rule.Ports)
			},
		},
		{
			name:      "PrivateAccessURLAddressesUseExactCIDRs",
			accessURL: mustParseURL(t, "https://coder.internal:8443"),
			resolver: sandboxPolicyResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
				return []netip.Addr{
					netip.MustParseAddr("203.0.113.20"),
					netip.MustParseAddr("::1"),
					netip.MustParseAddr("10.0.0.4"),
				}, nil
			}),
			assertions: func(t *testing.T, network sandboxRuntimeNetwork, err error) {
				require.NoError(t, err)
				require.Equal(t, []uint16{8443}, requireSandboxHostRule(t, network.Rules, "coder.internal").Ports)
				require.Equal(t, []uint16{8443}, requireSandboxCIDRRule(t, network.Rules, "10.0.0.4/32").Ports)
				require.Equal(t, []uint16{8443}, requireSandboxCIDRRule(t, network.Rules, "::1/128").Ports)
				require.NotContains(t, network.Rules, policy.Rule{CIDR: "203.0.113.20/32"})
			},
		},
		{
			name:      "ResolutionFailureKeepsHostnameRule",
			accessURL: accessURL,
			resolver: sandboxPolicyResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
				return nil, xerrors.New("resolver unavailable")
			}),
			assertions: func(t *testing.T, network sandboxRuntimeNetwork, err error) {
				require.ErrorContains(t, err, "resolver unavailable")
				require.Equal(t, []uint16{443}, requireSandboxHostRule(t, network.Rules, "coder.example").Ports)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			network, err := buildSandboxRuntimeNetwork(t.Context(), test.policy, test.accessURL, test.resolver)
			require.Equal(t, "watch", network.Reload)
			require.Equal(t, policy.ActionDeny, network.Default)
			require.Equal(t, policy.ModeEnforce, network.Mode)
			for _, rule := range network.Rules {
				require.Equal(t, policy.ActionAllow, rule.Action)
				require.Equal(t, policy.TLSPassthrough, rule.TLS)
			}
			test.assertions(t, network, err)
		})
	}
}

func TestSandboxRuntimeNetworkDeterministic(t *testing.T) {
	t.Parallel()

	accessURL := mustParseURL(t, "https://coder.example")
	resolver := sandboxPolicyResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
	})
	first, err := buildSandboxRuntimeNetwork(t.Context(), codersdk.AIEgressPolicy{
		Revision: 1,
		Rules: []codersdk.AIEgressRule{
			{Host: "b.example", Ports: []int{443, 80, 443}},
			{Host: "a.example", Ports: []int{8443}},
		},
	}, accessURL, resolver)
	require.NoError(t, err)
	second, err := buildSandboxRuntimeNetwork(t.Context(), codersdk.AIEgressPolicy{
		Revision: 2,
		Rules: []codersdk.AIEgressRule{
			{Host: "a.example", Ports: []int{8443}},
			{Host: "b.example", Ports: []int{80, 443}},
		},
	}, accessURL, resolver)
	require.NoError(t, err)
	firstYAML, err := yaml.Marshal(first)
	require.NoError(t, err)
	secondYAML, err := yaml.Marshal(second)
	require.NoError(t, err)
	require.Equal(t, string(firstYAML), string(secondYAML))
}

func requireSandboxHostRule(t *testing.T, rules []policy.Rule, host string) policy.Rule {
	t.Helper()
	for _, rule := range rules {
		if rule.Host == host {
			return rule
		}
	}
	require.FailNow(t, "host rule not found", host)
	return policy.Rule{}
}

func requireSandboxCIDRRule(t *testing.T, rules []policy.Rule, cidr string) policy.Rule {
	t.Helper()
	for _, rule := range rules {
		if rule.CIDR == cidr {
			return rule
		}
	}
	require.FailNow(t, "CIDR rule not found", cidr)
	return policy.Rule{}
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return parsed
}
