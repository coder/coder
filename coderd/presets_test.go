package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateVersionPresets(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		presets []codersdk.Preset
	}{
		{
			name:    "no presets",
			presets: []codersdk.Preset{},
		},
		{
			name: "single preset with parameters",
			presets: []codersdk.Preset{
				{
					Name: "My Preset",
					Parameters: []codersdk.PresetParameter{
						{
							Name:  "preset_param1",
							Value: "A1B2C3",
						},
						{
							Name:  "preset_param2",
							Value: "D4E5F6",
						},
					},
				},
			},
		},
		{
			name: "multiple presets with overlapping parameters",
			presets: []codersdk.Preset{
				{
					Name: "Preset 1",
					Parameters: []codersdk.PresetParameter{
						{
							Name:  "shared_param",
							Value: "value1",
						},
						{
							Name:  "unique_param1",
							Value: "unique1",
						},
					},
				},
				{
					Name: "Preset 2",
					Parameters: []codersdk.PresetParameter{
						{
							Name:  "shared_param",
							Value: "value2",
						},
						{
							Name:  "unique_param2",
							Value: "unique2",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
			user := coderdtest.CreateFirstUser(t, client)
			version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

			// nolint:gocritic // This is a test
			provisionerCtx := dbauthz.AsProvisionerd(ctx)

			// Insert all presets for this test case
			for _, givenPreset := range tc.presets {
				dbPreset, err := db.InsertPreset(provisionerCtx, database.InsertPresetParams{
					Name:              givenPreset.Name,
					TemplateVersionID: version.ID,
				})
				require.NoError(t, err)

				if len(givenPreset.Parameters) > 0 {
					var presetParameterNames []string
					var presetParameterValues []string
					for _, presetParameter := range givenPreset.Parameters {
						presetParameterNames = append(presetParameterNames, presetParameter.Name)
						presetParameterValues = append(presetParameterValues, presetParameter.Value)
					}
					_, err = db.InsertPresetParameters(provisionerCtx, database.InsertPresetParametersParams{
						TemplateVersionPresetID: dbPreset.ID,
						Names:                   presetParameterNames,
						Values:                  presetParameterValues,
					})
					require.NoError(t, err)
				}
			}

			userSubject, _, err := httpmw.UserRBACSubject(ctx, db, user.UserID, rbac.ScopeAll)
			require.NoError(t, err)
			userCtx := dbauthz.As(ctx, userSubject)

			gotPresets, err := client.TemplateVersionPresets(userCtx, version.ID)
			require.NoError(t, err)

			require.Equal(t, len(tc.presets), len(gotPresets))

			for _, expectedPreset := range tc.presets {
				found := false
				for _, gotPreset := range gotPresets {
					if gotPreset.Name == expectedPreset.Name {
						found = true

						// verify not only that we get the right number of parameters, but that we get the right parameters
						// This ensures that we don't get extra parameters from other presets
						require.Equal(t, len(expectedPreset.Parameters), len(gotPreset.Parameters))
						for _, expectedParam := range expectedPreset.Parameters {
							require.Contains(t, gotPreset.Parameters, expectedParam)
						}
						break
					}
				}
				require.True(t, found, "Expected preset %s not found in results", expectedPreset.Name)
			}
		})
	}
}
