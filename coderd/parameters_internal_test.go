package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func Test_parameterProvisionerVersionDiagnostic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		version string
		warning bool
	}{
		{
			version: "",
			warning: true,
		},
		{
			version: "invalid",
			warning: true,
		},
		{
			version: "0.4",
			warning: true,
		},
		{
			version: "0.5",
			warning: true,
		},
		{
			version: "0.6",
			warning: true,
		},
		{
			version: "1.4",
			warning: true,
		},
		{
			version: "1.5",
			warning: false,
		},
		{
			version: "1.6",
			warning: false,
		},
		{
			version: "2.0",
			warning: false,
		},
		{
			version: "2.5",
			warning: false,
		},
		{
			version: "2.6",
			warning: false,
		},
	}

	for _, tc := range testCases {
		t.Run("Version_"+tc.version, func(t *testing.T) {
			t.Parallel()
			diags := parameterProvisionerVersionDiagnostic(database.TemplateVersionTerraformValue{
				ProvisionerdVersion: tc.version,
			})
			if tc.warning {
				require.Len(t, diags, 1, "expected warning")
			} else {
				require.Len(t, diags, 0, "expected no warning")
			}
		})
	}
}
