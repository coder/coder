package dynamicparameters_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/dynamicparameters/rendermock"
	"github.com/coder/coder/v2/coderd/httpapi/httperror"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/preview"
	previewtypes "github.com/coder/preview/types"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

func TestResolveParameters(t *testing.T) {
	t.Parallel()

	t.Run("NewImmutable", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)

		// A single immutable parameter with no previous value.
		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Any()).
			AnyTimes().
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					{
						ParameterData: previewtypes.ParameterData{
							Name:         "immutable",
							Type:         previewtypes.ParameterTypeString,
							FormType:     provider.ParameterFormTypeInput,
							Mutable:      false,
							DefaultValue: previewtypes.StringLiteral("foo"),
							Required:     true,
						},
						Value:       previewtypes.StringLiteral("foo"),
						Diagnostics: nil,
					},
				},
			}, nil)

		ctx := testutil.Context(t, testutil.WaitShort)
		values, err := dynamicparameters.ResolveParameters(ctx, uuid.New(), render, false,
			[]database.WorkspaceBuildParameter{},        // No previous values
			[]codersdk.WorkspaceBuildParameter{},        // No new build values
			[]database.TemplateVersionPresetParameter{}, // No preset values
		)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"immutable": "foo"}, values)
	})

	// Tests a parameter going from mutable -> immutable
	t.Run("BecameImmutable", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)

		mutable := previewtypes.ParameterData{
			Name:         "immutable",
			Type:         previewtypes.ParameterTypeString,
			FormType:     provider.ParameterFormTypeInput,
			Mutable:      true,
			DefaultValue: previewtypes.StringLiteral("foo"),
			Required:     true,
		}
		immutable := mutable
		immutable.Mutable = false

		// A single immutable parameter with no previous value.
		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Any()).
			// Return the mutable param first
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					{
						ParameterData: mutable,
						Value:         previewtypes.StringLiteral("foo"),
						Diagnostics:   nil,
					},
				},
			}, nil)

		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Any()).
			// Then the immutable param
			Return(&preview.Output{
				Parameters: []previewtypes.Parameter{
					{
						ParameterData: immutable,
						//  The user set the value to bar
						Value:       previewtypes.StringLiteral("bar"),
						Diagnostics: nil,
					},
				},
			}, nil)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := dynamicparameters.ResolveParameters(ctx, uuid.New(), render, false,
			[]database.WorkspaceBuildParameter{
				{Name: "immutable", Value: "foo"}, // Previous value foo
			},
			[]codersdk.WorkspaceBuildParameter{
				{Name: "immutable", Value: "bar"}, // New value
			},
			[]database.TemplateVersionPresetParameter{}, // No preset values
		)
		require.Error(t, err)
		resp, ok := httperror.IsResponder(err)
		require.True(t, ok)

		_, respErr := resp.Response()
		require.Len(t, respErr.Validations, 1)
		require.Contains(t, respErr.Validations[0].Error(), "is not mutable")
	})
}
