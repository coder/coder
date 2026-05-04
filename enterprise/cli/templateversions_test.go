package cli_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateVersionsPromoteInvalidatePrebuilds(t *testing.T) {
	t.Parallel()

	client, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureWorkspacePrebuilds: 1,
			},
		},
	})

	version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prebuildPresetResponses("old-preset"))
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

	version2 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, prebuildPresetResponses("new-preset"), func(req *codersdk.CreateTemplateVersionRequest) {
		req.TemplateID = template.ID
		req.Name = "2.0.0"
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version2.ID)

	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitLong))
	presets, err := db.GetTemplatePresetsWithPrebuilds(ctx, uuid.NullUUID{UUID: template.ID, Valid: true})
	require.NoError(t, err)
	require.False(t, presetForVersion(t, presets, version1.ID).LastInvalidatedAt.Valid)
	require.False(t, presetForVersion(t, presets, version2.ID).LastInvalidatedAt.Valid)

	inv, root := newCLI(t,
		"templates", "versions", "promote",
		"--template", template.Name,
		"--template-version", version2.Name,
		"--invalidate-prebuilds",
	)
	var stdout bytes.Buffer
	inv.Stdout = &stdout
	//nolint:gocritic // Creating and promoting template versions requires owner permissions.
	clitest.SetupConfig(t, client, root)

	err = inv.Run()
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "Invalidated 1 prebuild preset(s).")
	require.Contains(t, stdout.String(), `Successfully promoted version "2.0.0"`)

	presets, err = db.GetTemplatePresetsWithPrebuilds(ctx, uuid.NullUUID{UUID: template.ID, Valid: true})
	require.NoError(t, err)
	require.False(t, presetForVersion(t, presets, version1.ID).LastInvalidatedAt.Valid)
	require.True(t, presetForVersion(t, presets, version2.ID).LastInvalidatedAt.Valid)
}

func prebuildPresetResponses(name string) *echo.Responses {
	return &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionGraph: []*proto.Response{{
			Type: &proto.Response_Graph{
				Graph: &proto.GraphComplete{
					Presets: []*proto.Preset{{
						Name:     name,
						Prebuild: &proto.Prebuild{Instances: 1},
					}},
				},
			},
		}},
		ProvisionApply: echo.ApplyComplete,
	}
}

func presetForVersion(t *testing.T, presets []database.GetTemplatePresetsWithPrebuildsRow, versionID uuid.UUID) database.GetTemplatePresetsWithPrebuildsRow {
	t.Helper()
	for _, preset := range presets {
		if preset.TemplateVersionID == versionID {
			return preset
		}
	}
	require.Failf(t, "missing preset", "template version ID: %s", versionID)
	return database.GetTemplatePresetsWithPrebuildsRow{}
}
