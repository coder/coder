package agentegress_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentegress"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestProxyEnv(t *testing.T) {
	t.Parallel()

	env := agentegress.ProxyEnv("127.0.0.1:3333", agentsdk.EgressConfig{
		ControlPlaneHosts: []string{
			"coder.example.com:443",
			"stun.l.google.com:19302",
			"[2001:db8::1]:443",
			"derp.example.com",
			"coder.example.com",
			"127.0.0.1:8080",
		},
	})

	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		require.Equal(t, "http://127.0.0.1:3333", env[key], key)
	}
	want := "localhost,127.0.0.1,::1,coder.example.com,stun.l.google.com,2001:db8::1,derp.example.com"
	require.Equal(t, want, env["NO_PROXY"])
	require.Equal(t, want, env["no_proxy"])
	require.Len(t, env, 8)
}
