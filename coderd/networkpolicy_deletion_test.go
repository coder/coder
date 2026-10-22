package coderd_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/networkpolicy"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

func seedNetworkPolicyApply(t *testing.T, db database.Store, p codersdk.NetworkPolicy, targetID, userID uuid.UUID, legacy, canceled bool) database.NetworkPolicyRun {
	t.Helper()
	//nolint:gocritic // Seed an authoritative fixture without touching infrastructure.
	ctx := dbauthz.AsSystemRestricted(t.Context())
	now := dbtime.Now()
	revision, err := db.GetNetworkPolicyRevisionByID(ctx, p.Revisions[0].ID)
	require.NoError(t, err)
	target, err := db.GetNetworkPolicyTargetByID(ctx, targetID)
	require.NoError(t, err)
	daemon := dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{OrganizationID: p.OrganizationID})
	job, err := db.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID: uuid.New(), CreatedAt: now, UpdatedAt: now, OrganizationID: p.OrganizationID,
		InitiatorID: userID, FileID: revision.FileID, Type: database.ProvisionerJobTypeNetworkPolicy,
		Provisioner: database.ProvisionerTypeTerraform, StorageMethod: database.ProvisionerStorageMethodFile,
		Input: []byte(`{}`), Tags: database.StringMap{"fixture": targetID.String()},
	})
	require.NoError(t, err)
	acquired, err := db.AcquireProvisionerJob(ctx, database.AcquireProvisionerJobParams{
		StartedAt: sql.NullTime{Time: now, Valid: true}, WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
		OrganizationID: p.OrganizationID, Types: []database.ProvisionerType{database.ProvisionerTypeTerraform},
		ProvisionerTags: []byte(`{"fixture":"` + targetID.String() + `"}`),
	})
	require.NoError(t, err)
	require.Equal(t, job.ID, acquired.ID)
	require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID: job.ID, UpdatedAt: now, CompletedAt: sql.NullTime{Time: now, Valid: true},
	}))
	job.CompletedAt = sql.NullTime{Time: now, Valid: true}
	target, err = db.StartNetworkPolicyTargetJob(ctx, database.StartNetworkPolicyTargetJobParams{ID: target.ID, Generation: target.Generation, JobID: uuid.NullUUID{UUID: job.ID, Valid: true}, DesiredRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, Status: "pending", UpdatedAt: now})
	require.NoError(t, err)
	run, err := db.InsertNetworkPolicyRun(ctx, database.InsertNetworkPolicyRunParams{ID: uuid.New(), JobID: job.ID, PolicyID: p.ID, TargetID: target.ID, RevisionID: revision.ID, OrganizationID: p.OrganizationID, Generation: target.Generation, Operation: "apply", CreatedAt: now})
	require.NoError(t, err)
	require.NoError(t, db.UpdateProvisionerJobWithCancelByID(ctx, database.UpdateProvisionerJobWithCancelByIDParams{ID: job.ID, CanceledAt: sql.NullTime{Time: now, Valid: canceled}, CompletedAt: job.CompletedAt}))
	_, err = db.CompleteNetworkPolicyTargetJob(ctx, database.CompleteNetworkPolicyTargetJobParams{ID: target.ID, Generation: target.Generation, JobID: target.ActiveJobID, State: []byte("retained state"), StateRunID: uuid.NullUUID{UUID: run.ID, Valid: !legacy}, AppliedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: !canceled}, Status: "succeeded", UpdatedAt: now})
	require.NoError(t, err)
	return run
}

func TestNetworkPolicyDeleteProvenanceAndRetries(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	for _, tc := range []struct {
		name             string
		legacy, canceled bool
	}{
		{name: "Explicit"}, {name: "Legacy", legacy: true}, {name: "CanceledSuccessfulApply", canceled: true}, {name: "LegacyCanceledSuccessfulApply", legacy: true, canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			//nolint:gocritic // Fixture state is persisted without running Terraform.
			systemCtx := dbauthz.AsSystemRestricted(ctx)
			oldTemplate := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
			newTemplate := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
			p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: uuid.NewString(), HCL: testNetworkPolicyHCL + "\n# " + tc.name, TemplateIDs: []uuid.UUID{oldTemplate.ID}})
			require.NoError(t, err)
			target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "local", Context: "local", Namespace: "tenant", ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			source := seedNetworkPolicyApply(t, db, p, target.ID, owner.UserID, tc.legacy, tc.canceled)
			p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
			require.NoError(t, err)
			_, err = client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "unused", Context: "unused", Namespace: "unused", ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
			require.NoError(t, err)
			p, err = client.SetNetworkPolicyTemplates(ctx, owner.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{TemplateIDs: []uuid.UUID{newTemplate.ID}, ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			latest, err := client.CreateNetworkPolicyRevision(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyRevisionRequest{HCL: testNetworkPolicyHCL + "\n# latest " + tc.name, ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			_, err = db.PublishNetworkPolicyRevision(systemCtx, database.PublishNetworkPolicyRevisionParams{ID: p.ID, PublishedRevisionID: uuid.NullUUID{UUID: latest.ID, Valid: true}, UpdatedAt: dbtime.Now()})
			require.NoError(t, err)
			p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
			require.NoError(t, err)
			_, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation - 1})
			require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
			p, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			require.NotNil(t, p.DeletionRequestedAt)
			require.Nil(t, p.DeletedAt)
			require.Len(t, p.Runs, 2)
			destroy := p.Runs[0]
			require.Equal(t, codersdk.NetworkPolicyOperationDestroy, destroy.Operation)
			require.Equal(t, source.RevisionID, destroy.RevisionID)
			require.Equal(t, source.TemplateIDs, destroy.TemplateIDs)
			persisted, err := db.GetNetworkPolicyTargetByID(systemCtx, target.ID)
			require.NoError(t, err)
			require.Equal(t, source.ID, persisted.StateRunID.UUID)
			require.Equal(t, []byte("retained state"), persisted.State)
			again, err := client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			require.Equal(t, p.Generation, again.Generation)
			require.Len(t, again.Runs, 2)
			assertNetworkPolicyMutationBlocked(t, client, p)
			// A proven pre-apply failure can be retried with the same source.
			require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{ID: destroy.JobID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, Error: sql.NullString{String: "init failed", Valid: true}, ErrorCode: sql.NullString{String: networkpolicy.ErrorCodeStateUnchanged, Valid: true}}))
			_, err = db.CompleteNetworkPolicyTargetJob(systemCtx, database.CompleteNetworkPolicyTargetJobParams{ID: target.ID, JobID: persisted.ActiveJobID, Generation: persisted.Generation, State: persisted.State, AppliedRevisionID: persisted.AppliedRevisionID, Status: "failed", UpdatedAt: dbtime.Now()})
			require.NoError(t, err)
			p, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			require.Len(t, p.Runs, 3)
			require.Equal(t, source.RevisionID, p.Runs[0].RevisionID)
			require.Equal(t, source.TemplateIDs, p.Runs[0].TemplateIDs)
			for _, candidate := range p.Targets {
				if candidate.ID != target.ID {
					require.NotNil(t, candidate.DestroyedAt, "never-applied targets stay destroyed on retry")
				}
			}
			// Simulate committed completion; finalizer validation is covered separately.
			persisted, err = db.GetNetworkPolicyTargetByID(systemCtx, target.ID)
			require.NoError(t, err)
			_, err = db.CompleteNetworkPolicyTargetJob(systemCtx, database.CompleteNetworkPolicyTargetJobParams{
				ID: target.ID, JobID: persisted.ActiveJobID, Generation: persisted.Generation,
				State:             []byte(`{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"test","resources":[]}`),
				AppliedRevisionID: persisted.AppliedRevisionID, DestroyedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, Status: "succeeded", UpdatedAt: dbtime.Now(),
			})
			require.NoError(t, err)
			require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{ID: p.Runs[0].JobID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}}))
			_, err = db.CompleteNetworkPolicyDeletion(systemCtx, database.CompleteNetworkPolicyDeletionParams{ID: p.ID, UpdatedAt: dbtime.Now()})
			require.NoError(t, err)
			p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
			require.NoError(t, err)
			require.NotNil(t, p.DeletedAt)
			again, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			require.Equal(t, p.Generation, again.Generation)
			require.Len(t, again.Runs, 3)
			require.Len(t, again.TemplateIDs, 1)
			_, err = client.NetworkPolicyRunLogs(ctx, owner.OrganizationID, p.ID, source.ID, 0)
			require.NoError(t, err)
		})
	}
}

func assertNetworkPolicyMutationBlocked(t *testing.T, client *codersdk.Client, p codersdk.NetworkPolicy) {
	t.Helper()
	ctx := t.Context()
	_, err := client.UpdateNetworkPolicy(ctx, p.OrganizationID, p.ID, codersdk.UpdateNetworkPolicyRequest{Name: p.Name, ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.CreateNetworkPolicyRevision(ctx, p.OrganizationID, p.ID, codersdk.CreateNetworkPolicyRevisionRequest{HCL: testNetworkPolicyHCL, ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.CreateNetworkPolicyTarget(ctx, p.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "other", Context: "other", Namespace: "other", ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.SetNetworkPolicyTemplates(ctx, p.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.PublishNetworkPolicy(ctx, p.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{RevisionID: p.Revisions[0].ID, TargetIDs: []uuid.UUID{p.Targets[0].ID}, Operation: codersdk.NetworkPolicyOperationApply, ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
}

func TestNetworkPolicyDeleteReservationBlocks(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	for _, operation := range []codersdk.NetworkPolicyOperation{codersdk.NetworkPolicyOperationApply, codersdk.NetworkPolicyOperationPlan} {
		t.Run(string(operation), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: uuid.NewString(), HCL: testNetworkPolicyHCL + "\n# " + string(operation)})
			require.NoError(t, err)
			target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "local", Context: "local", Namespace: "tenant", ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
			require.NoError(t, err)
			_, err = client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "unused", Context: "unused", Namespace: "unused", ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
			require.NoError(t, err)
			runs, err := client.PublishNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{RevisionID: p.Revisions[0].ID, TargetIDs: []uuid.UUID{target.ID}, Operation: operation, ExpectedGeneration: p.Generation})
			require.NoError(t, err)
			p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
			require.NoError(t, err)
			for _, unknown := range []bool{false, true} {
				if unknown {
					//nolint:gocritic // Model a reaper quarantine without contacting Terraform.
					systemCtx := dbauthz.AsSystemRestricted(ctx)
					require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{ID: runs[0].JobID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, ErrorCode: sql.NullString{String: networkpolicy.ErrorCodeStateUnknown, Valid: true}}))
				}
				_, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
				require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
				retained, err := client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
				require.NoError(t, err)
				require.Equal(t, p.Generation, retained.Generation)
				require.Nil(t, retained.DeletionRequestedAt)
				require.Len(t, retained.Runs, 1)
				for _, candidate := range retained.Targets {
					require.Nil(t, candidate.DestroyedAt)
				}
			}
		})
	}
}

func TestNetworkPolicyDeleteUnknownDestroy(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: uuid.NewString(), HCL: testNetworkPolicyHCL})
	require.NoError(t, err)
	target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "local", Context: "local", Namespace: "tenant", ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	seedNetworkPolicyApply(t, db, p, target.ID, owner.UserID, false, false)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	p, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	//nolint:gocritic // Seed the reaper's completed-but-reserved outcome.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{ID: p.Runs[0].JobID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true}, ErrorCode: sql.NullString{String: networkpolicy.ErrorCodeStateUnknown, Valid: true}}))
	_, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	retained, err := client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.Generation, retained.Generation)
	require.NotNil(t, retained.DeletionRequestedAt)
	require.Nil(t, retained.DeletedAt)
	require.Len(t, retained.Runs, 2)
	stored, err := db.GetNetworkPolicyTargetByID(systemCtx, target.ID)
	require.NoError(t, err)
	require.Equal(t, p.Runs[0].JobID, stored.ActiveJobID.UUID)
	require.Equal(t, []byte("retained state"), stored.State)
}

func TestNetworkPolicyDeleteNeverApplied(t *testing.T) {
	t.Parallel()
	client, _ := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: uuid.NewString(), HCL: testNetworkPolicyHCL})
	require.NoError(t, err)
	_, err = client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "local", Context: "local", Namespace: "tenant", ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	p, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	require.NotNil(t, p.DeletedAt)
	require.NotNil(t, p.DeletionRequestedAt)
	require.NotNil(t, p.Targets[0].DestroyedAt)
	require.Empty(t, p.Runs)
	assertNetworkPolicyMutationBlocked(t, client, p)
	again, err := client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	require.Equal(t, p.Generation, again.Generation)
	list, err := client.NetworkPolicies(ctx, owner.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, list)
	retained, err := client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ID, retained.ID)
	require.Len(t, retained.Revisions, 1)
	require.Len(t, retained.Targets, 1)
}

func TestNetworkPolicyDeleteHistoricalAuthorization(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	ctx := t.Context()
	allowed := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID, UserACL: database.TemplateACL{memberUser.ID.String(): {policy.ActionRead, policy.ActionUpdate}}})
	denied := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: uuid.NewString(), HCL: testNetworkPolicyHCL, TemplateIDs: []uuid.UUID{denied.ID}})
	require.NoError(t, err)
	target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "local", Context: "local", Namespace: "tenant", ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	seedNetworkPolicyApply(t, db, p, target.ID, owner.UserID, false, false)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	p, err = client.SetNetworkPolicyTemplates(ctx, owner.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{TemplateIDs: []uuid.UUID{allowed.ID}, ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	_, err = member.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	_, err = client.DeleteNetworkPolicy(ctx, otherOrg.ID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Nil(t, p.DeletionRequestedAt)
	require.Len(t, p.Runs, 1)
}
