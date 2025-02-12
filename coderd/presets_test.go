package coderd_test

import (
	"testing"

	"github.com/google/uuid"
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

	sdkPreset := codersdk.Preset{
		ID:   uuid.New(),
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

	preset, err := db.InsertPreset(provisionerCtx, database.InsertPresetParams{
		ID:                sdkPreset.ID,
		Name:              sdkPreset.Name,
		TemplateVersionID: version.ID,
	})
	require.NoError(t, err)

	var presetParameterNames []string
	var presetParameterValues []string
	for _, presetParameter := range sdkPreset.Parameters {
		presetParameterNames = append(presetParameterNames, presetParameter.Name)
		presetParameterValues = append(presetParameterValues, presetParameter.Value)
	}
	_, err = db.InsertPresetParameters(provisionerCtx, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   presetParameterNames,
		Values:                  presetParameterValues,
	})
	require.NoError(t, err)

	userSubject, _, err := httpmw.UserRBACSubject(ctx, db, user.UserID, rbac.ScopeAll)
	require.NoError(t, err)
	userCtx := dbauthz.As(ctx, userSubject)

	presets, err := client.TemplateVersionPresets(userCtx, version.ID)
	require.NoError(t, err)

	require.Equal(t, 1, len(presets))
	require.Equal(t, sdkPreset.ID, presets[0].ID)
	require.Equal(t, sdkPreset.Name, presets[0].Name)

	for _, presetParameter := range sdkPreset.Parameters {
		require.Contains(t, presets[0].Parameters, presetParameter)
	}
}
