package cli_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
	"github.com/coder/serpent"
)

func TestProvisionerJobs(t *testing.T) {
	t.Parallel()

	t.Run("Cancel", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		values := coderdtest.DeploymentValues(t)
		values.Experiments = serpent.StringArray{string(codersdk.ExperimentTemplateResources)}
		client, _, coderdAPI := coderdtest.NewWithAPI(t, &coderdtest.Options{
			DeploymentValues:         values,
			IncludeProvisionerDaemon: false,
			Database:                 db,
			Pubsub:                   ps,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// These CLI tests are related to provisioner job CRUD operations and as such
		// do not require the overhead of starting a provisioner. Other provisioner job
		// functionalities (acquisition etc.) are tested elsewhere.
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
		// Test helper to create a provisioner job of a given type with a given input.
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

		// Test helper to create a workspace build job with a predefined input.
		prepareWorkspaceBuildJob := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			var (
				wbID     = uuid.New()
				input, _ = json.Marshal(map[string]string{"workspace_build_id": wbID.String()})
				job      = prepareJob(t, database.ProvisionerJobTypeWorkspaceBuild, input)
				w        = dbgen.Workspace(t, db, database.WorkspaceTable{
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

		// Test helper to create a template version import job with a predefined input.
		prepareTemplateVersionImportJob := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			var (
				tvID     = uuid.New()
				input, _ = json.Marshal(map[string]string{"template_version_id": tvID.String()})
				job      = prepareJob(t, database.ProvisionerJobTypeTemplateVersionImport, input)
				_        = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      templateAdmin.ID,
					ID:             tvID,
					TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
					JobID:          job.ID,
				})
			)
			return job
		}

		// Test helper to create a template version import dry run job with a predefined input.
		prepareTemplateVersionImportJobDryRun := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			var (
				tvID     = uuid.New()
				input, _ = json.Marshal(map[string]interface{}{
					"template_version_id": tvID.String(),
					"dry_run":             true,
				})
				job = prepareJob(t, database.ProvisionerJobTypeTemplateVersionDryRun, input)
				_   = dbgen.TemplateVersion(t, db, database.TemplateVersion{
					OrganizationID: owner.OrganizationID,
					CreatedBy:      templateAdmin.ID,
					ID:             tvID,
					TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
					JobID:          job.ID,
				})
			)
			return job
		}

		prepareTemplateResourceJob := func(t *testing.T) database.ProvisionerJob {
			t.Helper()
			template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
			revision, err := client.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# root " + template.ID.String()})
			require.NoError(t, err)
			run, err := client.CreateTemplateResourceRun(t.Context(), template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
			require.NoError(t, err)
			job, err := db.GetProvisionerJobByID(t.Context(), run.Job.ID)
			require.NoError(t, err)
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
			{"Owner", client, "TemplateResources", prepareTemplateResourceJob, true},
			{"TemplateAdmin", templateAdminClient, "TemplateResources", prepareTemplateResourceJob, true},
			{"Member", memberClient, "TemplateResources", prepareTemplateResourceJob, false},
			{"Owner", client, "WorkspaceBuild", prepareWorkspaceBuildJob, true},
			{"Owner", client, "TemplateVersionImport", prepareTemplateVersionImportJob, true},
			{"Owner", client, "TemplateVersionImportDryRun", prepareTemplateVersionImportJobDryRun, true},
			{"Owner", client, "NetworkPolicy", func(t *testing.T) database.ProvisionerJob {
				t.Helper()
				ctx := testutil.Context(t, testutil.WaitLong)
				now := coderdAPI.Clock.Now()
				job := prepareJob(t, database.ProvisionerJobTypeNetworkPolicy, json.RawMessage(`{}`))
				file := dbgen.File(t, db, database.File{ID: job.FileID, CreatedBy: owner.UserID})
				p, err := db.InsertNetworkPolicy(ctx, database.InsertNetworkPolicyParams{ID: uuid.New(), OrganizationID: job.OrganizationID, Name: uuid.NewString(), CreatedBy: owner.UserID, CreatedAt: now, UpdatedAt: now})
				require.NoError(t, err)
				_, err = db.InsertNetworkPolicyAttachment(ctx, database.InsertNetworkPolicyAttachmentParams{PolicyID: p.ID, TemplateID: template.ID, OrganizationID: job.OrganizationID, CreatedAt: now})
				require.NoError(t, err)
				revision, err := db.InsertNetworkPolicyRevision(ctx, database.InsertNetworkPolicyRevisionParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: job.OrganizationID, RevisionNumber: 1, Hcl: "policy {}", FileID: file.ID, CreatedBy: owner.UserID, CreatedAt: now})
				require.NoError(t, err)
				target, err := db.InsertNetworkPolicyTarget(ctx, database.InsertNetworkPolicyTargetParams{ID: uuid.New(), PolicyID: p.ID, OrganizationID: job.OrganizationID, Cluster: "cluster", Context: "context", Namespace: "namespace", Tags: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now})
				require.NoError(t, err)
				target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{ID: target.ID, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, Generation: target.Generation, Status: "planning", UpdatedAt: now})
				require.NoError(t, err)
				_, err = db.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{ID: uuid.New(), JobID: job.ID, PolicyID: p.ID, TargetID: target.ID, RevisionID: revision.ID, OrganizationID: job.OrganizationID, Generation: target.Generation, Operation: "plan", CreatedAt: now})
				require.NoError(t, err)
				return job
			}, false},
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
				if tt.name == "NetworkPolicy" {
					require.ErrorContains(t, err, `canceling provisioner job type "network_policy" is not supported`)
				}
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
					if tt.name == "TemplateResources" {
						assert.Contains(t, buf.String(), "Canceling shared resource job "+job.ID.String())
					}
					assert.Contains(t, buf.String(), "Job canceled")
				} else {
					assert.NotContains(t, buf.String(), "Job canceled")
				}
			})
		}
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()

		db, ps := dbtestutil.NewDB(t)
		client, _, coderdAPI := coderdtest.NewWithAPI(t, &coderdtest.Options{
			IncludeProvisionerDaemon: false,
			Database:                 db,
			Pubsub:                   ps,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// These CLI tests are related to provisioner job CRUD operations and as such
		// do not require the overhead of starting a provisioner. Other provisioner job
		// functionalities (acquisition etc.) are tested elsewhere.
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
		// Create some test jobs
		job1 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
			OrganizationID: owner.OrganizationID,
			InitiatorID:    owner.UserID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte(`{"template_version_id":"` + version.ID.String() + `"}`),
			Tags:           database.StringMap{provisionersdk.TagScope: provisionersdk.ScopeOrganization},
		})

		job2 := dbgen.ProvisionerJob(t, db, coderdAPI.Pubsub, database.ProvisionerJob{
			OrganizationID: owner.OrganizationID,
			InitiatorID:    member.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte(`{"workspace_build_id":"` + uuid.New().String() + `"}`),
			Tags:           database.StringMap{provisionersdk.TagScope: provisionersdk.ScopeOrganization},
		})
		// Test basic list command
		t.Run("Basic", func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, "provisioner", "jobs", "list")
			clitest.SetupConfig(t, client, root)
			var buf bytes.Buffer
			inv.Stdout = &buf
			err := inv.Run()
			require.NoError(t, err)

			// Should contain both jobs
			output := buf.String()
			assert.Contains(t, output, job1.ID.String())
			assert.Contains(t, output, job2.ID.String())
		})

		// Test list with JSON output
		t.Run("JSON", func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, "provisioner", "jobs", "list", "--output", "json")
			clitest.SetupConfig(t, client, root)
			var buf bytes.Buffer
			inv.Stdout = &buf
			err := inv.Run()
			require.NoError(t, err)

			// Parse JSON output
			var jobs []codersdk.ProvisionerJob
			err = json.Unmarshal(buf.Bytes(), &jobs)
			require.NoError(t, err)

			// Should contain both jobs
			jobIDs := make([]uuid.UUID, len(jobs))
			for i, job := range jobs {
				jobIDs[i] = job.ID
			}
			assert.Contains(t, jobIDs, job1.ID)
			assert.Contains(t, jobIDs, job2.ID)
		})

		// Test list with limit
		t.Run("Limit", func(t *testing.T) {
			t.Parallel()

			inv, root := clitest.New(t, "provisioner", "jobs", "list", "--limit", "1")
			clitest.SetupConfig(t, client, root)
			var buf bytes.Buffer
			inv.Stdout = &buf
			err := inv.Run()
			require.NoError(t, err)

			// Should contain at most 1 job
			output := buf.String()
			jobCount := 0
			if strings.Contains(output, job1.ID.String()) {
				jobCount++
			}
			if strings.Contains(output, job2.ID.String()) {
				jobCount++
			}
			assert.LessOrEqual(t, jobCount, 1)
		})

		// Test list with initiator filter
		t.Run("InitiatorFilter", func(t *testing.T) {
			t.Parallel()

			// Get owner user details to access username
			ctx := testutil.Context(t, testutil.WaitShort)
			ownerUser, err := client.User(ctx, owner.UserID.String())
			require.NoError(t, err)

			// Test filtering by initiator (using username)
			inv, root := clitest.New(t, "provisioner", "jobs", "list", "--initiator", ownerUser.Username)
			clitest.SetupConfig(t, client, root)
			var buf bytes.Buffer
			inv.Stdout = &buf
			err = inv.Run()
			require.NoError(t, err)

			// Should only contain job1 (initiated by owner)
			output := buf.String()
			assert.Contains(t, output, job1.ID.String())
			assert.NotContains(t, output, job2.ID.String())
		})

		// Test list with invalid user
		t.Run("InvalidUser", func(t *testing.T) {
			t.Parallel()

			// Test with non-existent user
			inv, root := clitest.New(t, "provisioner", "jobs", "list", "--initiator", "nonexistent-user")
			clitest.SetupConfig(t, client, root)
			var buf bytes.Buffer
			inv.Stdout = &buf
			err := inv.Run()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "initiator not found: nonexistent-user")
		})
	})
}
