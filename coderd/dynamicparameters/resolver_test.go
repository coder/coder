package dynamicparameters_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
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
			Return(renderResult(
				previewtypes.Parameter{
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
			), nil)
		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			AnyTimes().
			Return(renderResult(
				previewtypes.Parameter{
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
			), nil)
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
			Return(renderResult(
				previewtypes.Parameter{
					ParameterData: mutable,
					Value:         previewtypes.StringLiteral("foo"),
					Diagnostics:   nil,
				},
			), nil)

		render.EXPECT().
			Render(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			// Then the immutable param
			Return(renderResult(
				previewtypes.Parameter{
					ParameterData: immutable,
					//  The user set the value to bar
					Value:       previewtypes.StringLiteral("bar"),
					Diagnostics: nil,
				},
			), nil)

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
			// First build, not enforced
			{name: "increasing/first build", monotonic: "increasing", cur: "1", firstBuild: true},
			// No previous value, not enforced
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
					Return(renderResult(
						previewtypes.Parameter{
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
					), nil)
				render.EXPECT().
					Render(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					AnyTimes().
					Return(renderResult(
						previewtypes.Parameter{
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
					), nil)

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

	t.Run("BaselineRenderDoesNotRequestSecretRequirementsWhenDeactivatingRequirement", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)
		ownerID := uuid.New()

		gomock.InOrder(
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "true"}).
				Return(renderResult(stringParameter("use_github", "true")), nil),
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "false"}, gomock.Any()).
				Return(renderResult(stringParameter("use_github", "false")), nil),
		)

		ctx := testutil.Context(t, testutil.WaitShort)
		values, err := dynamicparameters.ResolveParameters(ctx, ownerID, render, false,
			[]database.WorkspaceBuildParameter{{Name: "use_github", Value: "true"}},
			[]codersdk.WorkspaceBuildParameter{{Name: "use_github", Value: "false"}},
			[]database.TemplateVersionPresetParameter{},
		)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"use_github": "false"}, values)
	})

	t.Run("SkipSecretRequirementsAllowsFinalMissingSecrets", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)
		ownerID := uuid.New()

		gomock.InOrder(
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "true"}).
				Return(renderResult(stringParameter("use_github", "true")), nil),
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "true"}).
				Return(renderResultWithSecretRequirements(
					[]codersdk.SecretRequirementStatus{{
						Env:         "GITHUB_TOKEN",
						HelpMessage: "Add a GitHub PAT",
						Satisfied:   false,
					}},
					stringParameter("use_github", "true"),
				), nil),
		)

		ctx := testutil.Context(t, testutil.WaitShort)
		values, err := dynamicparameters.ResolveParameters(ctx, ownerID, render, false,
			[]database.WorkspaceBuildParameter{{Name: "use_github", Value: "true"}},
			nil,
			nil,
			dynamicparameters.SkipSecretRequirements(),
		)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"use_github": "true"}, values)
	})

	t.Run("FinalMissingSecretsBlockByDefault", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)
		ownerID := uuid.New()

		gomock.InOrder(
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "true"}).
				Return(renderResult(stringParameter("use_github", "true")), nil),
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "true"}, gomock.Any()).
				Return(renderResultWithSecretRequirements(
					[]codersdk.SecretRequirementStatus{{
						Env:         "GITHUB_TOKEN",
						HelpMessage: "Add a GitHub PAT",
						Satisfied:   false,
					}},
					stringParameter("use_github", "true"),
				), nil),
		)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := dynamicparameters.ResolveParameters(ctx, ownerID, render, false,
			[]database.WorkspaceBuildParameter{{Name: "use_github", Value: "true"}},
			nil,
			nil,
		)
		require.Error(t, err)
		resp, ok := httperror.IsResponder(err)
		require.True(t, ok)
		_, respErr := resp.Response()
		require.Contains(t, respErr.Detail, "Missing required secrets")
		require.Contains(t, respErr.Detail, "env GITHUB_TOKEN: Add a GitHub PAT")
	})

	t.Run("FinalRenderErrorSuppressesMissingSecretSynthesis", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		render := rendermock.NewMockRenderer(ctrl)
		ownerID := uuid.New()

		gomock.InOrder(
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "true"}).
				Return(renderResult(stringParameter("use_github", "true")), nil),
			render.EXPECT().
				Render(gomock.Any(), ownerID, map[string]string{"use_github": "true"}, gomock.Any()).
				Return(renderResultWithSecretRequirements(
					[]codersdk.SecretRequirementStatus{{
						Env:         "GITHUB_TOKEN",
						HelpMessage: "Add a GitHub PAT",
						Satisfied:   false,
					}},
					stringParameter("use_github", "true"),
				), hcl.Diagnostics{{
					Severity: hcl.DiagError,
					Summary:  "Render failed",
					Detail:   "Template parameter expression failed.",
				}}),
		)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := dynamicparameters.ResolveParameters(ctx, ownerID, render, false,
			[]database.WorkspaceBuildParameter{{Name: "use_github", Value: "true"}},
			nil,
			nil,
		)
		require.Error(t, err)
		resp, ok := httperror.IsResponder(err)
		require.True(t, ok)
		_, respErr := resp.Response()
		require.Contains(t, respErr.Detail, "Render failed")
		require.NotContains(t, respErr.Detail, "Missing required secrets")
	})
}

func stringParameter(name string, value string) previewtypes.Parameter {
	return previewtypes.Parameter{
		ParameterData: previewtypes.ParameterData{
			Name:         name,
			Type:         previewtypes.ParameterTypeString,
			FormType:     provider.ParameterFormTypeInput,
			Mutable:      true,
			DefaultValue: previewtypes.StringLiteral(value),
		},
		Value: previewtypes.StringLiteral(value),
	}
}

func renderResult(params ...previewtypes.Parameter) *dynamicparameters.RenderResult {
	return &dynamicparameters.RenderResult{
		Output: &preview.Output{
			Parameters: params,
		},
	}
}

func renderResultWithSecretRequirements(reqs []codersdk.SecretRequirementStatus, params ...previewtypes.Parameter) *dynamicparameters.RenderResult {
	return &dynamicparameters.RenderResult{
		Output: &preview.Output{
			Parameters: params,
		},
		SecretRequirements: reqs,
	}
}
