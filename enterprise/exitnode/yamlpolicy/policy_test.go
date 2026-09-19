package yamlpolicy_test

import (
	"crypto/sha256"
	"encoding/hex"
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
    deny: {cidrs: ["169.254.169.254"]}
  - id: no-quic
    deny: {protocols: [udp], ports: [443]}
  - id: github
    allow: {hosts: ["github.com", "*.github.com"], ports: [443]}
  - deny: {hosts: ["*.internal.example"]}
  - allow: {cidrs: ["10.0.0.0/8"], ports: ["8000-8999"]}
  - id: any-host-http
    allow: {hosts: ["*"], ports: [80]}
`

func ip(s string) netip.Addr { return netip.MustParseAddr(s) }
func TestPolicyEvaluate(t *testing.T) {
	t.Parallel()
	policy, err := yamlpolicy.Parse([]byte(testPolicy))
	require.NoError(t, err)
	for _, tt := range []struct {
		name, rule string
		flow       exitnode.FlowInfo
		allow      bool
	}{
		{"exact host", "github", exitnode.FlowInfo{Host: "github.com", IP: ip("140.82.112.3"), Port: 443}, true},
		{"normalized glob", "github", exitnode.FlowInfo{Host: "API.GitHub.com.", IP: ip("140.82.112.3"), Port: 443}, true},
		{"nested glob", "github", exitnode.FlowInfo{Host: "a.b.github.com", IP: ip("140.82.112.3"), Port: 443}, true},
		{"glob excludes apex", "", exitnode.FlowInfo{Host: "internal.example", IP: ip("10.1.2.3"), Port: 443}, false},
		{"wrong port", "", exitnode.FlowInfo{Host: "github.com", IP: ip("140.82.112.3"), Port: 22}, false},
		{"first rule wins", "block-metadata", exitnode.FlowInfo{Host: "github.com", IP: ip("169.254.169.254"), Port: 443}, false},
		{"cidr and range", "rule-5", exitnode.FlowInfo{IP: ip("10.20.30.40"), Port: 8080}, true},
		{"outside range", "", exitnode.FlowInfo{IP: ip("10.20.30.40"), Port: 9000}, false},
		{"host deny", "rule-4", exitnode.FlowInfo{Host: "db.internal.example", IP: ip("10.20.30.40"), Port: 8080}, false},
		{"wildcard known host", "any-host-http", exitnode.FlowInfo{Host: "anything.test", IP: ip("1.1.1.1"), Port: 80}, true},
		{"wildcard empty host", "", exitnode.FlowInfo{IP: ip("1.1.1.1"), Port: 80}, false},
		{"mapped ipv4", "block-metadata", exitnode.FlowInfo{IP: ip("::ffff:169.254.169.254"), Port: 80}, false},
		{"unknown cidr deny", "block-metadata", exitnode.FlowInfo{IP: ip("169.254.169.254"), Port: 443, HostUnknown: true}, false},
		{"unknown provisional allow", "github", exitnode.FlowInfo{IP: ip("140.82.112.3"), Port: 443, HostUnknown: true}, true},
		{"unknown skips host deny", "rule-5", exitnode.FlowInfo{IP: ip("10.20.30.40"), Port: 8080, HostUnknown: true}, true},
		{"unknown default deny", "", exitnode.FlowInfo{IP: ip("8.8.8.8"), Port: 53, HostUnknown: true}, false},
		{"udp protocol deny", "no-quic", exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolUDP, Host: "github.com", IP: ip("140.82.112.3"), Port: 443}, false},
		{"tcp excludes udp rule", "github", exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolTCP, Host: "github.com", IP: ip("140.82.112.3"), Port: 443}, true},
		{"udp unknown host deny", "no-quic", exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolUDP, IP: ip("140.82.112.3"), Port: 443, HostUnknown: true}, false},
		{"udp ordinary rule", "rule-5", exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolUDP, IP: ip("10.1.1.1"), Port: 8500}, true},
		{"dns host allow ignores port", "github", exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "api.github.com."}, true},
		{"dns host deny", "rule-4", exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "db.internal.example"}, false},
		{"dns wildcard", "any-host-http", exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "anything.test"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := policy.Evaluate(tt.flow)
			require.Equal(t, tt.allow, got.Allow, got.Reason)
			require.Equal(t, tt.rule, got.RuleID)
			require.NotEmpty(t, got.Reason)
		})
	}
}
func TestPolicyDNS(t *testing.T) {
	t.Parallel()
	policy, err := yamlpolicy.Parse([]byte(`
default: deny
rules:
  - id: ports-only
    deny: {ports: [53]}
  - id: dns-evil
    deny: {protocols: [dns], hosts: ["*.evil.example"]}
  - id: cidr-only
    deny: {cidrs: ["0.0.0.0/0"]}
  - id: good
    allow: {hosts: ["good.example"]}
  - id: other-dns
    deny: {protocols: [dns]}
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		host, rule string
		allow      bool
	}{
		{"good.example", "good", true},
		{"x.evil.example", "dns-evil", false},
		{"other.example", "other-dns", false},
	} {
		got := policy.Evaluate(exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: tt.host})
		require.Equal(t, tt.allow, got.Allow)
		require.Equal(t, tt.rule, got.RuleID)
	}
	got := policy.Evaluate(exitnode.FlowInfo{Host: "x.evil.example", IP: ip("1.2.3.4"), Port: 443})
	require.False(t, got.Allow)
	require.Equal(t, "cidr-only", got.RuleID)
	policy, err = yamlpolicy.Parse([]byte("default: deny\nrules:\n  - deny: {cidrs: [\"0.0.0.0/0\"]}\n"))
	require.NoError(t, err)
	require.True(t, policy.Evaluate(exitnode.FlowInfo{Protocol: codersdk.ExitNodeProtocolDNS, Host: "random.example"}).Allow)
	require.False(t, policy.Evaluate(exitnode.FlowInfo{IP: ip("1.2.3.4"), Port: 443}).Allow)
}
func TestPolicyDefaultAllow(t *testing.T) {
	t.Parallel()
	policy, err := yamlpolicy.Parse([]byte("default: allow\nrules:\n  - deny: {hosts: [\"*.blocked.example\"]}\n"))
	require.NoError(t, err)
	require.True(t, policy.Evaluate(exitnode.FlowInfo{IP: ip("1.2.3.4"), Port: 443, HostUnknown: true}).Allow)
	got := policy.Evaluate(exitnode.FlowInfo{Host: "x.blocked.example", IP: ip("1.2.3.4"), Port: 443})
	require.False(t, got.Allow)
	require.Equal(t, "rule-1", got.RuleID)
}
func TestPolicyParseErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, yaml, want string }{
		{"default", "default: maybe\n", "default must be"},
		{"actions", "rules:\n  - allow: {ports: [1]}\n    deny: {ports: [2]}\n", "not both"},
		{"no action", "rules:\n  - id: x\n", "must specify allow or deny"},
		{"empty", "rules:\n  - allow: {}\n", "at least one of"},
		{"protocol", "rules:\n  - allow: {protocols: [quic]}\n", "invalid protocol"},
		{"glob", "rules:\n  - allow: {hosts: [\"foo.*.com\"]}\n", "invalid host glob"},
		{"cidr", "rules:\n  - allow: {cidrs: [\"nope\"]}\n", "invalid cidr"},
		{"port", "rules:\n  - allow: {ports: [70000]}\n", "invalid port"},
		{"range", "rules:\n  - allow: {ports: [\"90-80\"]}\n", "end before start"},
		{"duplicate", "rules:\n  - id: a\n    allow: {ports: [1]}\n  - id: a\n    allow: {ports: [2]}\n", "duplicate rule id"},
		{"field", "rules:\n  - allow: {ports: [1]}\n    priority: 3\n", "field priority not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := yamlpolicy.Parse([]byte(tt.yaml))
			require.ErrorContains(t, err, tt.want)
		})
	}
}
func TestPolicyHashAndReload(t *testing.T) {
	t.Parallel()
	data := []byte("default: allow\n")
	parsed, err := yamlpolicy.Parse(data)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	require.Equal(t, hex.EncodeToString(sum[:]), parsed.PolicyHash())
	second, err := yamlpolicy.Parse(data)
	require.NoError(t, err)
	require.Equal(t, parsed.PolicyHash(), second.PolicyHash())
	require.Error(t, parsed.Reload())
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte("default: deny\n"), 0o600))
	policy, err := yamlpolicy.Load(path)
	require.NoError(t, err)
	initialHash := policy.PolicyHash()
	flow := exitnode.FlowInfo{IP: ip("1.2.3.4"), Port: 443}
	require.False(t, policy.Evaluate(flow).Allow)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	require.NoError(t, policy.Reload())
	require.True(t, policy.Evaluate(flow).Allow)
	require.NotEqual(t, initialHash, policy.PolicyHash())
	reloadedHash := policy.PolicyHash()
	require.NoError(t, os.WriteFile(path, []byte("default: nonsense\n"), 0o600))
	require.Error(t, policy.Reload())
	require.True(t, policy.Evaluate(flow).Allow)
	require.Equal(t, reloadedHash, policy.PolicyHash())
}
