package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestTemplateVersionPresets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
	user := coderdtest.CreateFirstUser(t, client)
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

	// nolint:gocritic // This is a test
	provisionerCtx := dbauthz.AsProvisionerd(ctx)

	preset, err := db.InsertPreset(provisionerCtx, database.InsertPresetParams{
		ID:                uuid.New(),
		Name:              "My Preset",
		TemplateVersionID: version.ID,
	})
	require.NoError(t, err)
	_, err = db.InsertPresetParameters(provisionerCtx, database.InsertPresetParametersParams{
		ID:                      uuid.New(),
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"preset_param1", "preset_param2"},
		Values:                  []string{"A1B2C3", "D4E5F6"},
	})
	require.NoError(t, err)

	userSubject, _, err := httpmw.UserRBACSubject(ctx, db, user.UserID, rbac.ScopeAll)
	require.NoError(t, err)
	userCtx := dbauthz.As(ctx, userSubject)

	presets, err := client.TemplateVersionPresets(userCtx, version.ID)
	require.NoError(t, err)
	require.Equal(t, 1, len(presets))
	require.Equal(t, "My Preset", presets[0].Name)

	presetParams, err := client.TemplateVersionPresetParameters(userCtx, version.ID)
	require.NoError(t, err)
	require.Equal(t, 2, len(presetParams))
	require.Equal(t, "preset_param1", presetParams[0].Name)
	require.Equal(t, "A1B2C3", presetParams[0].Value)
	require.Equal(t, "preset_param2", presetParams[1].Name)
	require.Equal(t, "D4E5F6", presetParams[1].Value)
}
