package yamlpolicy_test

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/exitnode"
	"github.com/coder/coder/v2/enterprise/exitnode/yamlpolicy"
)

const testPolicy = `
default: deny
rules:
  - id: block-metadata
    deny:
      cidrs: ["169.254.169.254"]
  - id: no-quic
    deny:
      protocols: [udp]
      ports: [443]
  - id: github
    allow:
      hosts: ["github.com", "*.github.com"]
      ports: [443]
  - deny:
      hosts: ["*.internal.example"]
  - allow:
      cidrs: ["10.0.0.0/8"]
      ports: ["8000-8999"]
  - id: any-host-http
    allow:
      hosts: ["*"]
      ports: [80]
`

func TestPolicyEvaluate(t *testing.T) {
	t.Parallel()

	policy, err := yamlpolicy.Parse([]byte(testPolicy))
	require.NoError(t, err)

	tests := []struct {
		name   string
		flow   exitnode.FlowInfo
		allow  bool
		ruleID string
	}{
		{
			name:   "exact host allowed",
			flow:   exitnode.FlowInfo{Host: "github.com", IP: netip.MustParseAddr("140.82.112.3"), Port: 443},
			allow:  true,
			ruleID: "github",
		},
		{
			name:   "glob matches subdomain",
			flow:   exitnode.FlowInfo{Host: "API.GitHub.com.", IP: netip.MustParseAddr("140.82.112.3"), Port: 443},
			allow:  true,
			ruleID: "github",
		},
		{
			name:   "glob matches nested subdomain",
			flow:   exitnode.FlowInfo{Host: "a.b.github.com", IP: netip.MustParseAddr("140.82.112.3"), Port: 443},
			allow:  true,
			ruleID: "github",
		},
		{
			name:  "glob does not match apex",
			flow:  exitnode.FlowInfo{Host: "internal.example", IP: netip.MustParseAddr("10.1.2.3"), Port: 443},
			allow: false,
		},
		{
			name:  "host allowed but wrong port falls to default",
			flow:  exitnode.FlowInfo{Host: "github.com", IP: netip.MustParseAddr("140.82.112.3"), Port: 22},
			allow: false,
		},
		{
			name:   "cidr deny wins over later host allow",
			flow:   exitnode.FlowInfo{Host: "github.com", IP: netip.MustParseAddr("169.254.169.254"), Port: 443},
			allow:  false,
			ruleID: "block-metadata",
		},
		{
			name:   "cidr and port range allow without host",
			flow:   exitnode.FlowInfo{IP: netip.MustParseAddr("10.20.30.40"), Port: 8080},
			allow:  true,
			ruleID: "rule-5",
		},
		{
			name:  "port outside range falls to default",
			flow:  exitnode.FlowInfo{IP: netip.MustParseAddr("10.20.30.40"), Port: 9000},
			allow: false,
		},
		{
			name:   "host glob deny",
			flow:   exitnode.FlowInfo{Host: "db.internal.example", IP: netip.MustParseAddr("10.20.30.40"), Port: 8080},
			allow:  false,
			ruleID: "rule-4",
		},
		{
			name:   "wildcard host requires a host",
			flow:   exitnode.FlowInfo{Host: "anything.test", IP: netip.MustParseAddr("1.1.1.1"), Port: 80},
			allow:  true,
			ruleID: "any-host-http",
		},
		{
			name:  "wildcard host does not match empty host",
			flow:  exitnode.FlowInfo{IP: netip.MustParseAddr("1.1.1.1"), Port: 80},
			allow: false,
		},
		{
			name:   "ipv4 mapped ipv6 matches cidr",
			flow:   exitnode.FlowInfo{IP: netip.MustParseAddr("::ffff:169.254.169.254"), Port: 80},
			allow:  false,
			ruleID: "block-metadata",
		},
		{
			name:   "host unknown: cidr deny is final",
			flow:   exitnode.FlowInfo{IP: netip.MustParseAddr("169.254.169.254"), Port: 443, HostUnknown: true},
			allow:  false,
			ruleID: "block-metadata",
		},
		{
			name:   "host unknown: host allow is provisional",
			flow:   exitnode.FlowInfo{IP: netip.MustParseAddr("140.82.112.3"), Port: 443, HostUnknown: true},
			allow:  true,
			ruleID: "github",
		},
		{
			name:   "host unknown: host deny is skipped, later allow applies",
			flow:   exitnode.FlowInfo{IP: netip.MustParseAddr("10.20.30.40"), Port: 8080, HostUnknown: true},
			allow:  true,
			ruleID: "rule-5",
		},
		{
			name:  "host unknown: nothing could allow falls to default",
			flow:  exitnode.FlowInfo{IP: netip.MustParseAddr("8.8.8.8"), Port: 53, HostUnknown: true},
			allow: false,
		},
		{
			name:   "udp 443 denied by protocol rule",
			flow:   exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolUDP, Host: "github.com", IP: netip.MustParseAddr("140.82.112.3"), Port: 443},
			allow:  false,
			ruleID: "no-quic",
		},
		{
			name:   "tcp 443 to the same host is unaffected by the udp rule",
			flow:   exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolTCP, Host: "github.com", IP: netip.MustParseAddr("140.82.112.3"), Port: 443},
			allow:  true,
			ruleID: "github",
		},
		{
			name:   "udp 443 with unknown host is denied before sniffing",
			flow:   exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolUDP, IP: netip.MustParseAddr("140.82.112.3"), Port: 443, HostUnknown: true},
			allow:  false,
			ruleID: "no-quic",
		},
		{
			name:   "udp on another port follows the ordinary rules",
			flow:   exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolUDP, IP: netip.MustParseAddr("10.1.1.1"), Port: 8500},
			allow:  true,
			ruleID: "rule-5",
		},
		{
			name:   "dns: host allow matches regardless of port",
			flow:   exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "api.github.com."},
			allow:  true,
			ruleID: "github",
		},
		{
			name:   "dns: host deny applies",
			flow:   exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "db.internal.example"},
			allow:  false,
			ruleID: "rule-4",
		},
		{
			name:   "dns: wildcard host rule is eligible",
			flow:   exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "anything.test"},
			allow:  true,
			ruleID: "any-host-http",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := policy.Evaluate(tt.flow)
			require.Equal(t, tt.allow, got.Allow, "reason: %s", got.Reason)
			require.Equal(t, tt.ruleID, got.RuleID)
			require.NotEmpty(t, got.Reason)
		})
	}
}

func TestPolicyDNS(t *testing.T) {
	t.Parallel()

	// Rules with only cidrs or ports never decide dns; an explicit
	// protocols: [dns] rule may, even without hosts.
	policy, err := yamlpolicy.Parse([]byte(`
default: deny
rules:
  - id: ports-only
    deny:
      ports: [53]
  - id: dns-only-evil
    deny:
      protocols: [dns]
      hosts: ["*.evil.example"]
  - id: cidr-only
    deny:
      cidrs: ["0.0.0.0/0"]
  - id: allow-good
    allow:
      hosts: ["good.example"]
  - id: no-other-dns
    deny:
      protocols: [dns]
`))
	require.NoError(t, err)

	dns := func(host string) exitnode.Decision {
		return policy.Evaluate(exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: host})
	}
	got := dns("good.example")
	require.True(t, got.Allow, got.Reason)
	require.Equal(t, "allow-good", got.RuleID)

	got = dns("x.evil.example")
	require.False(t, got.Allow)
	require.Equal(t, "dns-only-evil", got.RuleID)

	got = dns("other.example")
	require.False(t, got.Allow)
	require.Equal(t, "no-other-dns", got.RuleID)

	// The dns-only deny has no effect on tcp to the same name.
	got = policy.Evaluate(exitnode.FlowInfo{Host: "x.evil.example", IP: netip.MustParseAddr("1.2.3.4"), Port: 443})
	require.False(t, got.Allow)
	require.Equal(t, "cidr-only", got.RuleID)

	// With no eligible rule at all, default: deny does not apply to dns.
	policy, err = yamlpolicy.Parse([]byte(`
default: deny
rules:
  - deny:
      cidrs: ["0.0.0.0/0"]
`))
	require.NoError(t, err)
	got = policy.Evaluate(exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "random.example"})
	require.True(t, got.Allow, got.Reason)
	require.Empty(t, got.RuleID)
	require.False(t, policy.Evaluate(exitnode.FlowInfo{IP: netip.MustParseAddr("1.2.3.4"), Port: 443}).Allow)
}

func TestPolicyDefaultAllow(t *testing.T) {
	t.Parallel()

	policy, err := yamlpolicy.Parse([]byte(`
default: allow
rules:
  - deny:
      hosts: ["*.blocked.example"]
`))
	require.NoError(t, err)

	got := policy.Evaluate(exitnode.FlowInfo{IP: netip.MustParseAddr("1.2.3.4"), Port: 443, HostUnknown: true})
	require.True(t, got.Allow)
	require.Empty(t, got.RuleID)

	got = policy.Evaluate(exitnode.FlowInfo{Host: "x.blocked.example", IP: netip.MustParseAddr("1.2.3.4"), Port: 443})
	require.False(t, got.Allow)
	require.Equal(t, "rule-1", got.RuleID)
}

func TestPolicyParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"bad default", "default: maybe\n", "default must be"},
		{"both actions", "rules:\n  - allow: {ports: [1]}\n    deny: {ports: [2]}\n", "not both"},
		{"no action", "rules:\n  - id: x\n", "must specify allow or deny"},
		{"empty match", "rules:\n  - allow: {}\n", "at least one of"},
		{"bad protocol", "rules:\n  - allow: {protocols: [quic]}\n", "invalid protocol"},
		{"bad glob", "rules:\n  - allow: {hosts: [\"foo.*.com\"]}\n", "invalid host glob"},
		{"bad cidr", "rules:\n  - allow: {cidrs: [\"nope\"]}\n", "invalid cidr"},
		{"bad port", "rules:\n  - allow: {ports: [70000]}\n", "invalid port"},
		{"bad range", "rules:\n  - allow: {ports: [\"90-80\"]}\n", "end before start"},
		{"duplicate id", "rules:\n  - id: a\n    allow: {ports: [1]}\n  - id: a\n    allow: {ports: [2]}\n", "duplicate rule id"},
		{"unknown field", "rules:\n  - allow: {ports: [1]}\n    priority: 3\n", "field priority not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := yamlpolicy.Parse([]byte(tt.yaml))
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestPolicyReload(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte("default: deny\n"), 0o600))

	policy, err := yamlpolicy.Load(path)
	require.NoError(t, err)
	flow := exitnode.FlowInfo{IP: netip.MustParseAddr("1.2.3.4"), Port: 443}
	require.False(t, policy.Evaluate(flow).Allow)

	require.NoError(t, os.WriteFile(path, []byte("default: allow\n"), 0o600))
	require.NoError(t, policy.Reload())
	require.True(t, policy.Evaluate(flow).Allow)

	// A broken file keeps the previous policy in force.
	require.NoError(t, os.WriteFile(path, []byte("default: nonsense\n"), 0o600))
	require.Error(t, policy.Reload())
	require.True(t, policy.Evaluate(flow).Allow)

	_, err = yamlpolicy.Parse([]byte("default: allow\n"))
	require.NoError(t, err)
	inMemory, _ := yamlpolicy.Parse([]byte("default: allow\n"))
	require.Error(t, inMemory.Reload())
}
