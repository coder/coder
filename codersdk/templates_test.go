package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestModuleCacheDisabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		deploymentValues *codersdk.DeploymentValues
		templateDisabled bool
		expected         bool
	}{
		{
			name:             "NoDeploymentValues",
			deploymentValues: nil,
			expected:         false,
		},
		{
			name:             "NoDeploymentValuesTemplateDisabled",
			deploymentValues: nil,
			templateDisabled: true,
			expected:         true,
		},
		{
			name:             "BothEnabled",
			deploymentValues: deploymentValuesWithModuleCacheDisabled(false),
			expected:         false,
		},
		{
			name:             "TemplateDisabled",
			deploymentValues: deploymentValuesWithModuleCacheDisabled(false),
			templateDisabled: true,
			expected:         true,
		},
		{
			// The deployment cannot be overridden by a template.
			name:             "DeploymentDisabled",
			deploymentValues: deploymentValuesWithModuleCacheDisabled(true),
			expected:         true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, codersdk.ModuleCacheDisabled(tc.deploymentValues, tc.templateDisabled))
		})
	}
}

func deploymentValuesWithModuleCacheDisabled(disabled bool) *codersdk.DeploymentValues {
	dv := &codersdk.DeploymentValues{}
	dv.Provisioner.DisableModuleCache = serpent.Bool(disabled)
	return dv
}
