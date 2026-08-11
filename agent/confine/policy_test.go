package confine_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/confine"
	"github.com/coder/coder/v2/codersdk"
)

func TestPolicyEngineMatcher(t *testing.T) {
	t.Parallel()

	engine := confine.NewPolicyEngine("CODER.Example.COM.", 8443)
	engine.Update(codersdk.AIEgressPolicy{
		Revision: 7,
		Rules: []codersdk.AIEgressRule{
			{Host: "Example.COM."},
			{Host: "*.Services.Example.com", Ports: []int{1234}},
			{Host: "192.0.2.10", Ports: []int{8080}},
		},
	})

	tests := []struct {
		name    string
		host    string
		port    int
		allowed bool
	}{
		{name: "exact default http", host: "example.com", port: 80, allowed: true},
		{name: "exact default https", host: "EXAMPLE.COM.", port: 443, allowed: true},
		{name: "exact other port", host: "example.com", port: 22, allowed: false},
		{name: "wildcard one label", host: "api.services.example.com", port: 1234, allowed: true},
		{name: "wildcard apex", host: "services.example.com", port: 1234, allowed: false},
		{name: "wildcard multiple labels", host: "a.b.services.example.com", port: 1234, allowed: false},
		{name: "wildcard wrong port", host: "api.services.example.com", port: 443, allowed: false},
		{name: "ip exact", host: "192.0.2.10", port: 8080, allowed: true},
		{name: "default deny", host: "denied.example.com", port: 443, allowed: false},
		{name: "implicit control plane", host: "coder.example.com", port: 8443, allowed: true},
		{name: "implicit control plane wrong port", host: "coder.example.com", port: 443, allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decision := engine.Decide(tt.host, tt.port)
			require.Equal(t, tt.allowed, decision.Allowed)
			require.EqualValues(t, 7, decision.Revision)
		})
	}
}
