package coderd_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

func TestTemplateResourceJobListingAndCancellation(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"plan", "apply", "destroy"} {
		for _, acquired := range []bool{false, true} {
			name := action + "/pending"
			if acquired {
				name = action + "/acquired"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				client, db := newTemplateResourceClient(t)
				owner := coderdtest.CreateFirstUser(t, client)
				member, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(owner.OrganizationID))
				deniedMember, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
				ctx := t.Context()
				//nolint:gocritic // Simulate acquisition/completion without infrastructure.
				systemCtx := dbauthz.AsSystemRestricted(ctx)
				tags, err := json.Marshal(map[string]string{provisionersdk.TagScope: provisionersdk.ScopeOrganization, provisionersdk.TagOwner: "", provisionersdk.TagTemplateResources: provisionersdk.TemplateResourcesCapabilityVersion})
				require.NoError(t, err)
				template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID, DisplayName: "Resources", Icon: "/icon/code.svg", UserACL: database.TemplateACL{user.ID.String(): []policy.Action{policy.ActionRead, policy.ActionUpdate}}})
				templateFoundation := dbgen.Foundation(t, db, database.Foundation{OrganizationID: template.OrganizationID, TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}})
				revision, err := client.CreateFoundationRevision(ctx, template.OrganizationID, templateFoundation.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# root"})
				require.NoError(t, err)
				// Preserve an applied snapshot when canceling any operation.
				seed, err := client.CreateFoundationRun(ctx, template.OrganizationID, templateFoundation.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
				require.NoError(t, err)
				_, err = db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{
					FoundationID: templateFoundation.ID, RunID: uuid.NullUUID{UUID: seed.ID, Valid: true}, Status: "applied",
					State: []byte("private state"), Outputs: json.RawMessage(`{"value":"saved"}`), AppliedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, CheckpointRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: dbtime.Now(),
				})
				require.NoError(t, err)
				require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{ID: seed.Job.ID, CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, UpdatedAt: dbtime.Now()}))
				run, err := client.CreateFoundationRun(ctx, template.OrganizationID, templateFoundation.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: action})
				require.NoError(t, err)
				require.Equal(t, codersdk.ProvisionerJobTypeTemplateResources, run.Job.Type)
				require.Equal(t, &run.ID, run.Job.Input.TemplateResourceRunID)

				job, err := client.OrganizationProvisionerJob(ctx, owner.OrganizationID, run.Job.ID)
				require.NoError(t, err)
				require.Equal(t, codersdk.ProvisionerJobTypeTemplateResources, job.Type)
				require.Equal(t, &run.ID, job.Input.TemplateResourceRunID)
				require.Equal(t, template.ID, job.Metadata.TemplateID)
				require.Equal(t, template.Name, job.Metadata.TemplateName)
				require.Equal(t, template.DisplayName, job.Metadata.TemplateDisplayName)
				require.Equal(t, template.Icon, job.Metadata.TemplateIcon)
				jobs, err := client.OrganizationProvisionerJobs(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerJobsOptions{IDs: []uuid.UUID{run.Job.ID}, Status: []codersdk.ProvisionerJobStatus{codersdk.ProvisionerJobPending}})
				require.NoError(t, err)
				require.Len(t, jobs, 1)
				require.Equal(t, job, jobs[0])
				foreignOrg := dbgen.Organization(t, db, database.Organization{})
				_, err = client.OrganizationProvisionerJob(ctx, foreignOrg.ID, run.Job.ID)
				require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
				require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, deniedMember.CancelFoundationRun(ctx, template.OrganizationID, templateFoundation.ID, run.ID)).StatusCode())
				other := dbgen.Template(t, db, database.Template{OrganizationID: foreignOrg.ID, CreatedBy: owner.UserID})
				otherFoundation := dbgen.Foundation(t, db, database.Foundation{OrganizationID: other.OrganizationID, TemplateID: uuid.NullUUID{UUID: other.ID, Valid: true}})
				require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, client.CancelFoundationRun(ctx, other.OrganizationID, otherFoundation.ID, run.ID)).StatusCode())

				var daemon database.ProvisionerDaemon
				if acquired {
					daemon = dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{OrganizationID: owner.OrganizationID})
					acquiredJob, err := db.AcquireProvisionerJob(systemCtx, database.AcquireProvisionerJobParams{OrganizationID: owner.OrganizationID, Types: []database.ProvisionerType{database.ProvisionerTypeTerraform}, ProvisionerTags: tags, StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true}})
					require.NoError(t, err)
					require.Equal(t, run.Job.ID, acquiredJob.ID)
					daemons, err := client.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{IDs: []uuid.UUID{daemon.ID}, Offline: true})
					require.NoError(t, err)
					require.Len(t, daemons, 1)
					require.NotNil(t, daemons[0].CurrentJob)
					require.Equal(t, template.Name, daemons[0].CurrentJob.TemplateName)
					require.Equal(t, template.DisplayName, daemons[0].CurrentJob.TemplateDisplayName)
					require.Equal(t, template.Icon, daemons[0].CurrentJob.TemplateIcon)
				}
				// A template ACL admin can cancel without deployment-wide privileges.
				require.NoError(t, member.CancelFoundationRun(ctx, template.OrganizationID, templateFoundation.ID, run.ID))
				job, err = client.OrganizationProvisionerJob(ctx, owner.OrganizationID, run.Job.ID)
				require.NoError(t, err)
				state, err := db.GetTemplateResourceState(systemCtx, templateFoundation.ID)
				require.NoError(t, err)
				require.Equal(t, "applied", state.Status)
				require.Equal(t, []byte("private state"), state.State)
				require.JSONEq(t, `{"value":"saved"}`, string(state.Outputs))
				require.Equal(t, revision.ID, state.AppliedRevisionID.UUID)
				require.Equal(t, acquired, state.ActiveRunID.Valid)
				if acquired {
					require.Equal(t, run.ID, state.ActiveRunID.UUID)
					require.Equal(t, codersdk.ProvisionerJobCanceling, job.Status)
					require.Nil(t, job.CompletedAt)
					require.NoError(t, member.CancelFoundationRun(ctx, template.OrganizationID, templateFoundation.ID, run.ID), "repeated request is idempotent")
					_, err = client.CreateFoundationRun(ctx, template.OrganizationID, templateFoundation.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
					require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
					require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{ID: run.Job.ID, CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, UpdatedAt: dbtime.Now()}))
					daemons, err := client.OrganizationProvisionerDaemons(ctx, owner.OrganizationID, &codersdk.OrganizationProvisionerDaemonsOptions{IDs: []uuid.UUID{daemon.ID}, Offline: true})
					require.NoError(t, err)
					require.Len(t, daemons, 1)
					require.Nil(t, daemons[0].CurrentJob)
					require.NotNil(t, daemons[0].PreviousJob)
					require.Equal(t, template.Name, daemons[0].PreviousJob.TemplateName)
					require.Equal(t, template.DisplayName, daemons[0].PreviousJob.TemplateDisplayName)
					require.Equal(t, template.Icon, daemons[0].PreviousJob.TemplateIcon)
				} else {
					require.Equal(t, codersdk.ProvisionerJobCanceled, job.Status)
					require.NotNil(t, job.CompletedAt)
					// A canceled pending job is terminal and cannot be acquired.
					_, err = db.AcquireProvisionerJob(systemCtx, database.AcquireProvisionerJobParams{OrganizationID: owner.OrganizationID, Types: []database.ProvisionerType{database.ProvisionerTypeTerraform}, ProvisionerTags: tags})
					require.ErrorIs(t, err, sql.ErrNoRows)
				}
				require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, client.CancelFoundationRun(ctx, template.OrganizationID, templateFoundation.ID, run.ID)).StatusCode())
			})
		}
	}
}
