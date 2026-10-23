package coderd_test

import (
	"bytes"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk"
)

func TestTemplateValidationJobsRequireResourceCapability(t *testing.T) {
	t.Parallel()
	client, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	data, err := echo.Tar(nil)
	require.NoError(t, err)
	file, err := client.Upload(t.Context(), codersdk.ContentTypeTar, bytes.NewReader(data))
	require.NoError(t, err)
	for _, mode := range []string{"Configured", "ConfiguredAfterImport", "Unconfigured"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			//nolint:gocritic // Inspect job routing without running a provisioner.
			systemCtx := dbauthz.AsSystemRestricted(ctx)
			template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
			configure := func() {
				_, err := db.InsertTemplateResourceState(systemCtx, database.InsertTemplateResourceStateParams{
					TemplateID: template.ID, OrganizationID: template.OrganizationID, UpdatedAt: dbtime.Now(),
				})
				require.NoError(t, err)
			}
			tags := map[string]string{"region": "example"}
			if mode == "Configured" {
				configure()
				// A caller cannot downgrade a server-required capability.
				tags[provisionersdk.TagTemplateResources] = "0"
			}
			version, err := client.CreateTemplateVersion(ctx, owner.OrganizationID, codersdk.CreateTemplateVersionRequest{
				TemplateID: template.ID, FileID: file.ID, StorageMethod: codersdk.ProvisionerStorageMethodFile,
				Provisioner: codersdk.ProvisionerTypeEcho, ProvisionerTags: tags,
			})
			require.NoError(t, err)
			importJob, err := db.GetProvisionerJobByID(systemCtx, version.Job.ID)
			require.NoError(t, err)
			require.Equal(t, "example", importJob.Tags["region"])
			if mode == "Configured" {
				require.Equal(t, provisionersdk.TemplateResourcesCapabilityVersion, importJob.Tags[provisionersdk.TagTemplateResources])
			} else {
				require.NotContains(t, importJob.Tags, provisionersdk.TagTemplateResources)
			}
			require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
				ID: importJob.ID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
			}))
			if mode == "ConfiguredAfterImport" {
				configure()
			}
			// A template user can validate without permission to read raw resource state.
			dryRun, err := member.CreateTemplateVersionDryRun(ctx, version.ID, codersdk.CreateTemplateVersionDryRunRequest{WorkspaceName: "validation"})
			require.NoError(t, err)
			dryJob, err := db.GetProvisionerJobByID(systemCtx, dryRun.ID)
			require.NoError(t, err)
			require.Equal(t, "example", dryJob.Tags["region"])
			require.Equal(t, importJob.Tags[provisionersdk.TagScope], dryJob.Tags[provisionersdk.TagScope])
			require.Equal(t, importJob.Tags[provisionersdk.TagOwner], dryJob.Tags[provisionersdk.TagOwner])
			if mode == "Unconfigured" {
				require.NotContains(t, dryJob.Tags, provisionersdk.TagTemplateResources)
			} else {
				require.Equal(t, provisionersdk.TemplateResourcesCapabilityVersion, dryJob.Tags[provisionersdk.TagTemplateResources])
			}
			storedImport, err := db.GetProvisionerJobByID(systemCtx, importJob.ID)
			require.NoError(t, err)
			require.Equal(t, importJob.Tags, storedImport.Tags)
		})
	}
}
