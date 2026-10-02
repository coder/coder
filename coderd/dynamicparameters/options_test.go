package dynamicparameters_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/dynamicparameters/rendermock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

func TestRenderReconciled(t *testing.T) {
	t.Parallel()

	t.Run("ValidValueIsLeftAlone", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)

		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Eq(map[string]string{"color": "blue"})).
			Times(1).
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					optionParameter("color", "blue", "blue", "green"),
				},
			}, nil)

		ctx := testutil.Context(t, testutil.WaitShort)
		output, diags := dynamicparameters.RenderReconciled(ctx, render, uuid.New(), map[string]string{"color": "blue"})
		require.False(t, diags.HasErrors())
		require.Empty(t, output.Parameters[0].Diagnostics)
	})

	t.Run("StaleValueFallsBackToDefault", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)

		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Eq(map[string]string{"color": "red"})).
			Times(1).
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					optionParameter("color", "red", "blue", "green"),
				},
			}, nil)

		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Eq(map[string]string{})).
			Times(1).
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					optionParameter("color", "blue", "blue", "green"),
				},
			}, nil)

		ctx := testutil.Context(t, testutil.WaitShort)
		output, diags := dynamicparameters.RenderReconciled(ctx, render, uuid.New(), map[string]string{"color": "red"})
		require.False(t, diags.HasErrors())

		require.Len(t, output.Parameters[0].Diagnostics, 1)
		warning := output.Parameters[0].Diagnostics[0]
		require.Equal(t, dynamicparameters.DiagnosticCodeStaleOption,
			previewtypes.ExtractDiagnosticExtra(warning).Code)
		require.Contains(t, warning.Detail, "red")
	})

	// Dropping a value can strand a second parameter whose options are derived
	// from the first, which only shows up in the render that follows the drop.
	t.Run("StrandedValueIsAlsoReconciled", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)

		// "eu-large" is still an option while region is "eu".
		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Eq(map[string]string{"region": "eu", "instance": "eu-large"})).
			Times(1).
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					optionParameter("region", "eu", "us"),
					optionParameter("instance", "eu-large", "eu-small", "eu-large"),
				},
			}, nil)

		// Once region falls back to "us", the instance options change under it.
		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Eq(map[string]string{"instance": "eu-large"})).
			Times(1).
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					optionParameter("region", "us", "us"),
					optionParameter("instance", "eu-large", "us-small", "us-large"),
				},
			}, nil)

		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Eq(map[string]string{})).
			Times(1).
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					optionParameter("region", "us", "us"),
					optionParameter("instance", "us-small", "us-small", "us-large"),
				},
			}, nil)

		ctx := testutil.Context(t, testutil.WaitShort)
		output, diags := dynamicparameters.RenderReconciled(ctx, render, uuid.New(),
			map[string]string{"region": "eu", "instance": "eu-large"})
		require.False(t, diags.HasErrors())

		for _, parameter := range output.Parameters {
			require.Len(t, parameter.Diagnostics, 1, "parameter %q", parameter.Name)
			require.Equal(t, dynamicparameters.DiagnosticCodeStaleOption,
				previewtypes.ExtractDiagnosticExtra(parameter.Diagnostics[0]).Code,
				"parameter %q", parameter.Name)
		}
	})
}

// optionParameter is a mutable dropdown holding value, offering options.
func optionParameter(name string, value string, options ...string) previewtypes.Parameter {
	opts := make([]*previewtypes.ParameterOption, 0, len(options))
	for _, option := range options {
		opts = append(opts, &previewtypes.ParameterOption{
			Name:  option,
			Value: previewtypes.StringLiteral(option),
		})
	}

	return previewtypes.Parameter{
		ParameterData: previewtypes.ParameterData{
			Name:     name,
			Type:     previewtypes.ParameterTypeString,
			FormType: provider.ParameterFormTypeDropdown,
			Mutable:  true,
			Options:  opts,
		},
		Value: previewtypes.StringLiteral(value),
	}
}
