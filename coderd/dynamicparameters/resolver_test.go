package dynamicparameters_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/dynamicparameters"
	"github.com/coder/coder/v2/coderd/dynamicparameters/rendermock"
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
}
