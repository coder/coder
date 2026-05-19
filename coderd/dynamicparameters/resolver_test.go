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
	"github.com/coder/coder/v2/coderd/util/ptr"
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

	t.Run("Monotonic", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			monotonic  string
			prev       string // empty means no previous value
			cur        string
			firstBuild bool
			expectErr  string // empty means no error expected
		}{
			// Increasing
			{name: "increasing/increase allowed", monotonic: "increasing", prev: "5", cur: "10"},
			{name: "increasing/same allowed", monotonic: "increasing", prev: "5", cur: "5"},
			{name: "increasing/decrease rejected", monotonic: "increasing", prev: "10", cur: "5", expectErr: "must be equal or greater than previous value"},
			// Decreasing
			{name: "decreasing/decrease allowed", monotonic: "decreasing", prev: "10", cur: "5"},
			{name: "decreasing/same allowed", monotonic: "decreasing", prev: "5", cur: "5"},
			{name: "decreasing/increase rejected", monotonic: "decreasing", prev: "5", cur: "10", expectErr: "must be equal or lower than previous value"},
			// First build — not enforced
			{name: "increasing/first build", monotonic: "increasing", cur: "1", firstBuild: true},
			// No previous value — not enforced
			{name: "increasing/no previous", monotonic: "increasing", cur: "5"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctrl := gomock.NewController(t)
				render := rendermock.NewMockRenderer(ctrl)

				render.EXPECT().
					Render(gomock.Any(), gomock.Any(), gomock.Any()).
					AnyTimes().
					Return(&preview.Output{
						Parameters: []previewtypes.Parameter{
							{
								ParameterData: previewtypes.ParameterData{
									Name:     "param",
									Type:     previewtypes.ParameterTypeNumber,
									FormType: provider.ParameterFormTypeInput,
									Mutable:  true,
									Validations: []*previewtypes.ParameterValidation{
										{Monotonic: ptr.Ref(tc.monotonic)},
									},
								},
								Value:       previewtypes.StringLiteral(tc.cur),
								Diagnostics: nil,
							},
						},
					}, nil)

				var previousValues []database.WorkspaceBuildParameter
				if tc.prev != "" {
					previousValues = []database.WorkspaceBuildParameter{
						{Name: "param", Value: tc.prev},
					}
				}

				ctx := testutil.Context(t, testutil.WaitShort)
				_, err := dynamicparameters.ResolveParameters(ctx, uuid.New(), render, tc.firstBuild,
					previousValues,
					[]codersdk.WorkspaceBuildParameter{
						{Name: "param", Value: tc.cur},
					},
					[]database.TemplateVersionPresetParameter{},
				)
				if tc.expectErr != "" {
					require.Error(t, err)
					resp, ok := httperror.IsResponder(err)
					require.True(t, ok)
					_, respErr := resp.Response()
					require.Len(t, respErr.Validations, 1)
					require.Contains(t, respErr.Validations[0].Error(), tc.expectErr)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})
}
