package prebuilds

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestReconcilerNetworkPolicyStartupOptions(t *testing.T) {
	t.Parallel()
	values := &codersdk.DeploymentValues{NetworkPolicyStartupMode: "allow-last-applied", NetworkPolicyDriftAction: "allow"}
	option := WithNetworkPolicyStartup(values)
	// The factory must snapshot startup settings, not retain mutable config.
	values.NetworkPolicyStartupMode = "require-current"
	values.NetworkPolicyDriftAction = "block"
	reconciler := NewStoreReconciler(nil, nil, nil, codersdk.PrebuildsConfig{}, testutil.Logger(t), quartz.NewMock(t), nil, nil, nil, trace.NewNoopTracerProvider(), 1, nil, option)
	require.Equal(t, networkpolicy.StartupOptions{Mode: networkpolicy.StartupAllowLastApplied, DriftAction: networkpolicy.DriftAllow}, reconciler.networkPolicyStartup)

	defaults := NewStoreReconciler(nil, nil, nil, codersdk.PrebuildsConfig{}, testutil.Logger(t), quartz.NewMock(t), nil, nil, nil, trace.NewNoopTracerProvider(), 1, nil)
	require.Equal(t, networkpolicy.StartupOptions{}, defaults.networkPolicyStartup)
}
