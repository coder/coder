package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestProvisionerDaemonTypes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value string
		valid bool
	}{
		{name: "Default", valid: true},
		{name: "Sandbox", value: "sandbox", valid: true},
		{name: "MixedSandbox", value: "terraform,sandbox"},
		{name: "Unknown", value: "unrecognized"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			values := &codersdk.DeploymentValues{}
			opts := values.Options()
			var env []serpent.EnvVar
			if tc.value != "" {
				env = append(env, serpent.EnvVar{Name: "CODER_PROVISIONER_TYPES", Value: tc.value})
			}
			err := opts.ParseEnv(env)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, opts.SetDefaults())
			want := tc.value
			if want == "" {
				want = "terraform"
			}
			require.Equal(t, []string{want}, []string(values.Provisioner.DaemonTypes))
		})
	}
}
