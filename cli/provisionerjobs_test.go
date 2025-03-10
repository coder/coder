package cli_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/smithy-go/ptr"
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

	// Create initial resources with a running provisioner.
	firstProvisioner := coderdtest.NewTaggedProvisionerDaemon(t, coderdAPI, "default-provisioner", map[string]string{"owner": "", "scope": "organization"})
	t.Cleanup(func() { _ = firstProvisioner.Close() })
	version := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, completeWithAgent())
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version.ID, func(req *codersdk.CreateTemplateRequest) {
		req.AllowUserCancelWorkspaceJobs = ptr.Bool(true)
	})

	// Stop the provisioner so it doesn't grab any more jobs.
	firstProvisioner.Close()

	t.Run("Cancel", func(t *testing.T) {
		t.Parallel()

		// Set up test helpers.
		type jobInput struct {
			WorkspaceBuildID  string `json:"workspace_build_id,omitempty"`
			TemplateVersionID string `json:"template_version_id,omitempty"`
			DryRun            bool   `json:"dry_run,omitempty"`
		}
		prepareJob := func(t *testing.T, input jobInput) database.ProvisionerJob {
			t.Helper()

			inputBytes, err := json.Marshal(input)
			require.NoError(t, err)

			var typ database.ProvisionerJobType
			switch {
			case input.WorkspaceBuildID != "":
				typ = database.ProvisionerJobTypeWorkspaceBuild
			case input.TemplateVersionID != "":
				if input.DryRun {
					typ = database.ProvisionerJobTypeTemplateVersionDryRun
				} else {
					typ = database.ProvisionerJobTypeTemplateVersionImport
				}
			default:
				t.Fatal("invalid input")
			}

			var (
				tags = database.StringMap{"owner": "", "scope": "organization", "foo": uuid.New().String()}
				_    = dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{Tags: tags})
				job  = dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
					InitiatorID: member.ID,
					Input:       json.RawMessage(inputBytes),
					Type:        typ,
					Tags:        tags,
					StartedAt:   sql.NullTime{Time: coderdAPI.Clock.Now().Add(-time.Minute), Valid: true},
				})
			)
			return job
		}

		prepareWorkspaceBuildJob := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			var (
				wbID = uuid.New()
				job  = prepareJob(t, jobInput{WorkspaceBuildID: wbID.String()})
				w    = dbgen.Workspace(t, db, database.WorkspaceTable{
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
			)
			return job
		}

		prepareTemplateVersionImportJobBuilder := func(t *testing.T, dryRun bool) database.ProvisionerJob {
			t.Helper()
			var (
				tvID = uuid.New()
				job  = prepareJob(t, jobInput{TemplateVersionID: tvID.String(), DryRun: dryRun})
				_    = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      templateAdmin.ID,
					ID:             tvID,
					TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
					JobID:          job.ID,
				})
			)
			return job
		}
		prepareTemplateVersionImportJob := func(t *testing.T) database.ProvisionerJob {
			return prepareTemplateVersionImportJobBuilder(t, false)
		}
		prepareTemplateVersionImportJobDryRun := func(t *testing.T) database.ProvisionerJob {
			return prepareTemplateVersionImportJobBuilder(t, true)
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
			tt := tt
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
