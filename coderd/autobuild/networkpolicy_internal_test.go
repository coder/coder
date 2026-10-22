package autobuild

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestExecutorNetworkPolicyStartupOptions(t *testing.T) {
	t.Parallel()
	values := &codersdk.DeploymentValues{NetworkPolicyStartupMode: "allow-last-applied", NetworkPolicyDriftAction: "allow"}
	option := WithNetworkPolicyStartup(values)
	// The factory must snapshot startup settings, not retain mutable config.
	values.NetworkPolicyStartupMode = "require-current"
	values.NetworkPolicyDriftAction = "block"
	executor := NewExecutor(t.Context(), nil, nil, nil, prometheus.NewRegistry(), nil, nil, nil, nil, testutil.Logger(t), nil, nil, nil, nil, option)
	require.Equal(t, networkpolicy.StartupOptions{Mode: networkpolicy.StartupAllowLastApplied, DriftAction: networkpolicy.DriftAllow}, executor.networkPolicyStartup)

	defaults := NewExecutor(t.Context(), nil, nil, nil, prometheus.NewRegistry(), nil, nil, nil, nil, testutil.Logger(t), nil, nil, nil, nil)
	require.Equal(t, networkpolicy.StartupOptions{}, defaults.networkPolicyStartup)
}
