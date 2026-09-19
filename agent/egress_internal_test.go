package agent

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestEgressChangeRequiresRestart(t *testing.T) {
	t.Parallel()

	base := agentsdk.EgressConfig{
		ExitNodeIDs:       []uuid.UUID{uuid.New()},
		ExitNodePort:      3128,
		Enforce:           true,
		ControlPlaneHosts: []string{"tcp/control.example:443"},
	}
	for _, tt := range []struct {
		name string
		next agentsdk.EgressConfig
		want bool
	}{
		{name: "unchanged", next: base},
		{name: "selector", next: agentsdk.EgressConfig{ExitNodeIDs: []uuid.UUID{uuid.New()}, ExitNodePort: 4128, Enforce: true, ControlPlaneHosts: base.ControlPlaneHosts}},
		{name: "enforcement", next: agentsdk.EgressConfig{ExitNodeIDs: base.ExitNodeIDs, ExitNodePort: base.ExitNodePort, Enforce: false, ControlPlaneHosts: base.ControlPlaneHosts}, want: true},
		{name: "control plane hosts", next: agentsdk.EgressConfig{ExitNodeIDs: base.ExitNodeIDs, ExitNodePort: base.ExitNodePort, Enforce: true, ControlPlaneHosts: []string{"tcp/other.example:443"}}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, egressChangeRequiresRestart(base, tt.next))
		})
	}
}
