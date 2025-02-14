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
	// TODO (sasswart): test case: what if immutable parameters are used in the preset?
	// TODO (sasswart): test case: what if presets are defined for a template version with no params?
	// TODO (sasswart): test case: what if a non active version is selected?
	// TODO (sasswart): test case: what if a preset is selected that has no parameters?
	// TODO (sasswart): what if we have preset params and autofill params on the same param?
	// TODO (sasswart): test case: if we move from preset to no preset, do we reset the params?
	// If so, how should it behave? Reset to initial value? reset to last set value?
	// TODO (sasswart): test case: rich parameters
	// TODO (sasswart): Test case: what if a user tries to read presets or preset parameters from a different org?

	t.Parallel()

	givenPreset := codersdk.Preset{
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
	}
	ctx := testutil.Context(t, testutil.WaitShort)

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

	// nolint:gocritic // This is a test
	provisionerCtx := dbauthz.AsProvisionerd(ctx)

	dbPreset, err := db.InsertPreset(provisionerCtx, database.InsertPresetParams{
		Name:              givenPreset.Name,
		TemplateVersionID: version.ID,
	})
	require.NoError(t, err)

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

	userSubject, _, err := httpmw.UserRBACSubject(ctx, db, user.UserID, rbac.ScopeAll)
	require.NoError(t, err)
	userCtx := dbauthz.As(ctx, userSubject)

	gotPresets, err := client.TemplateVersionPresets(userCtx, version.ID)
	require.NoError(t, err)

	require.Equal(t, 1, len(gotPresets))
	require.Equal(t, givenPreset.Name, gotPresets[0].Name)

	for _, presetParameter := range givenPreset.Parameters {
		require.Contains(t, gotPresets[0].Parameters, presetParameter)
	}
}
