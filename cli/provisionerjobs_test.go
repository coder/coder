package cli_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestProvisionerJobs(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	client, _, coderdAPI := coderdtest.NewWithAPI(t, &coderdtest.Options{
		IncludeProvisionerDaemon: false,
		Database:                 db,
		Pubsub:                   ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	// Create minimal template setup without provisioner overhead
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:               owner.OrganizationID,
		CreatedBy:                    owner.UserID,
		AllowUserCancelWorkspaceJobs: true,
	})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: owner.OrganizationID,
		CreatedBy:      owner.UserID,
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
	})

	t.Run("Cancel", func(t *testing.T) {
		t.Parallel()

		// Set up test helpers - simplified to avoid provisioner daemon overhead.
		prepareJob := func(t *testing.T, jobType database.ProvisionerJobType, input json.RawMessage) database.ProvisionerJob {
			t.Helper()
			return dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
				InitiatorID: member.ID,
				Input:       input,
				Type:        jobType,
				StartedAt:   sql.NullTime{Time: coderdAPI.Clock.Now().Add(-time.Minute), Valid: true},
				Tags:        database.StringMap{provisionersdk.TagOwner: "", provisionersdk.TagScope: provisionersdk.ScopeOrganization, "foo": uuid.NewString()},
			})
		}

		prepareWorkspaceBuildJob := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			wbID := uuid.New()
			input, _ := json.Marshal(map[string]string{"workspace_build_id": wbID.String()})
			job := prepareJob(t, database.ProvisionerJobTypeWorkspaceBuild, input)

			w := dbgen.Workspace(t, db, database.WorkspaceTable{
				OrganizationID: owner.OrganizationID,
				OwnerID:        member.ID,
				TemplateID:     template.ID,
			})
			_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				ID:                wbID,
				InitiatorID:       member.ID,
				WorkspaceID:       w.ID,
				TemplateVersionID: version.ID,
				JobID:             job.ID,
			})
			return job
		}

		prepareTemplateVersionImportJob := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			tvID := uuid.New()
			input, _ := json.Marshal(map[string]string{"template_version_id": tvID.String()})
			job := prepareJob(t, database.ProvisionerJobTypeTemplateVersionImport, input)

			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: owner.OrganizationID,
				CreatedBy:      templateAdmin.ID,
				ID:             tvID,
				TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
				JobID:          job.ID,
			})
			return job
		}

		prepareTemplateVersionImportJobDryRun := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			tvID := uuid.New()
			input, _ := json.Marshal(map[string]interface{}{
				"template_version_id": tvID.String(),
				"dry_run":             true,
			})
			job := prepareJob(t, database.ProvisionerJobTypeTemplateVersionDryRun, input)

			_ = dbgen.TemplateVersion(t, db, database.TemplateVersion{
				OrganizationID: owner.OrganizationID,
				CreatedBy:      templateAdmin.ID,
				ID:             tvID,
				TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
				JobID:          job.ID,
			})
			return job
		}

		// Run the cancellation test suite.
		for _, tt := range []struct {
			role          string
			client        *codersdk.Client
			name          string
			prepare       func(*testing.T) database.ProvisionerJob
			wantCancelled bool
		}{
			{"Owner", client, "WorkspaceBuild", prepareWorkspaceBuildJob, true},
			{"Owner", client, "TemplateVersionImport", prepareTemplateVersionImportJob, true},
			{"Owner", client, "TemplateVersionImportDryRun", prepareTemplateVersionImportJobDryRun, true},
			{"TemplateAdmin", templateAdminClient, "WorkspaceBuild", prepareWorkspaceBuildJob, false},
			{"TemplateAdmin", templateAdminClient, "TemplateVersionImport", prepareTemplateVersionImportJob, true},
			{"TemplateAdmin", templateAdminClient, "TemplateVersionImportDryRun", prepareTemplateVersionImportJobDryRun, false},
			{"Member", memberClient, "WorkspaceBuild", prepareWorkspaceBuildJob, false},
			{"Member", memberClient, "TemplateVersionImport", prepareTemplateVersionImportJob, false},
			{"Member", memberClient, "TemplateVersionImportDryRun", prepareTemplateVersionImportJobDryRun, false},
		} {
			wantMsg := "OK"
			if !tt.wantCancelled {
				wantMsg = "FAIL"
			}
			t.Run(fmt.Sprintf("%s/%s/%v", tt.role, tt.name, wantMsg), func(t *testing.T) {
				t.Parallel()

				job := tt.prepare(t)
				require.False(t, job.CanceledAt.Valid, "job.CanceledAt.Valid")

				inv, root := clitest.New(t, "provisioner", "jobs", "cancel", job.ID.String())
				clitest.SetupConfig(t, tt.client, root)
				var buf bytes.Buffer
				inv.Stdout = &buf
				err := inv.Run()
				if tt.wantCancelled {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}

				job, err = db.GetProvisionerJobByID(testutil.Context(t, testutil.WaitShort), job.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.wantCancelled, job.CanceledAt.Valid, "job.CanceledAt.Valid")
				assert.Equal(t, tt.wantCancelled, job.CanceledAt.Time.After(job.StartedAt.Time), "job.CanceledAt.Time")
				if tt.wantCancelled {
					assert.Contains(t, buf.String(), "Job canceled")
				} else {
					assert.NotContains(t, buf.String(), "Job canceled")
				}
			})
		}
	})
}
