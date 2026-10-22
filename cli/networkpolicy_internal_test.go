package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestNetworkPolicyProvisionerEnabled(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                   string
		experiment             bool
		module, explicitModule string
		daemons                int64
		want                   bool
	}{
		{name: "disabled", daemons: 3},
		{name: "experiment-only", experiment: true, daemons: 3},
		{name: "module-only", module: "/trusted/module", daemons: 3},
		{name: "opt-in", experiment: true, module: "/trusted/module", daemons: 3, want: true},
		{name: "explicit-option", experiment: true, explicitModule: "/trusted/module", daemons: 3, want: true},
		{name: "zero-daemons", experiment: true, module: "/trusted/module"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &codersdk.DeploymentValues{NetworkPolicyModulePath: serpent.String(tc.module)}
			cfg.Provisioner.Daemons = serpent.Int64(tc.daemons)
			api := &coderd.API{Options: &coderd.Options{DeploymentValues: cfg, NetworkPolicyModulePath: tc.explicitModule}}
			if tc.experiment {
				api.Experiments = codersdk.Experiments{codersdk.ExperimentNetworkPolicies}
			}
			require.Equal(t, tc.want, networkPolicyProvisionerEnabled(api))
		})
	}
}
