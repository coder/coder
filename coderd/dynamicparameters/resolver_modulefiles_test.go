package dynamicparameters_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

// missingModulesRenderer renders a single parameter and reports whether coderd
// is missing the module files for the version.
type missingModulesRenderer struct {
	missingModuleFiles bool
	diags              hcl.Diagnostics
}

func (r missingModulesRenderer) Render(_ context.Context, _ uuid.UUID, _ map[string]string) (*preview.Output, hcl.Diagnostics) {
	return &preview.Output{
		Parameters: []previewtypes.Parameter{
			{
				ParameterData: previewtypes.ParameterData{
					Name:         "rendered",
					Type:         previewtypes.ParameterTypeString,
					FormType:     provider.ParameterFormTypeInput,
					Mutable:      true,
					DefaultValue: previewtypes.StringLiteral("foo"),
				},
				Value: previewtypes.StringLiteral("foo"),
			},
		},
	}, r.diags
}

func (missingModulesRenderer) Close() {}

func (r missingModulesRenderer) MissingModuleFiles() bool { return r.missingModuleFiles }

// TestResolveParametersMissingModuleFiles asserts that a version with no cached
// module files keeps stored values for parameters the render cannot reach, even
// when the render reports no diagnostics at all. Without this, the guard relies
// entirely on a warning the renderer infers.
func TestResolveParametersMissingModuleFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		missingModuleFiles bool
		expect             map[string]string
	}{
		{
			name:               "module files missing preserves unrendered value",
			missingModuleFiles: true,
			expect:             map[string]string{"rendered": "foo", "unrendered": "1000Gi"},
		},
		{
			// The complete render is the only case that may narrow the set.
			name:               "module files present drops unrendered value",
			missingModuleFiles: false,
			expect:             map[string]string{"rendered": "foo"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			values, err := dynamicparameters.ResolveParameters(ctx, uuid.New(),
				missingModulesRenderer{missingModuleFiles: tc.missingModuleFiles},
				false,
				database.WorkspaceTransitionStart,
				[]database.WorkspaceBuildParameter{
					{Name: "rendered", Value: "foo"},
					{Name: "unrendered", Value: "1000Gi"},
				},
				[]codersdk.WorkspaceBuildParameter{},
				[]database.TemplateVersionPresetParameter{},
			)
			require.NoError(t, err)
			require.Equal(t, tc.expect, values)
		})
	}
}
