package coderd_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/serpent"
)

const testNetworkPolicyHCL = `module "coder_network_policy" {
 source = "./module"
 allowed_https_hosts = ["github.com"]
 direct_p2p_enabled = false
}`

func newNetworkPolicyClient(t *testing.T) (*codersdk.Client, database.Store) {
	t.Helper()
	module := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte("# trusted module\n"), 0600))
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentNetworkPolicies)}
	values.NetworkPolicyModulePath = serpent.String(module)
	return coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: values})
}

func TestNetworkPolicyExperimentGate(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	user := coderdtest.CreateFirstUser(t, client)
	_, err := client.NetworkPolicies(t.Context(), user.OrganizationID)
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
}

func TestNetworkPolicyNativePublication(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	user := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	template := dbgen.Template(t, db, database.Template{OrganizationID: user.OrganizationID, CreatedBy: user.UserID})
	p, err := client.CreateNetworkPolicy(ctx, user.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "egress", HCL: testNetworkPolicyHCL, TemplateIDs: []uuid.UUID{template.ID}})
	require.NoError(t, err)
	require.EqualValues(t, 1, p.Generation)
	require.Len(t, p.Revisions, 1)
	require.Equal(t, testNetworkPolicyHCL, p.Revisions[0].HCL)
	require.Empty(t, p.Targets)

	// Editing never mutates the previous revision or provisions infrastructure.
	revision, err := client.CreateNetworkPolicyRevision(ctx, user.OrganizationID, p.ID, codersdk.CreateNetworkPolicyRevisionRequest{HCL: testNetworkPolicyHCL, ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	require.NotEqual(t, p.Revisions[0].ID, revision.ID)
	p, err = client.NetworkPolicy(ctx, user.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Len(t, p.Revisions, 2)
	require.Empty(t, p.Runs)
	_, err = client.UpdateNetworkPolicy(ctx, user.OrganizationID, p.ID, codersdk.UpdateNetworkPolicyRequest{Name: "stale", ExpectedGeneration: 1})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())

	target, err := client.CreateNetworkPolicyTarget(ctx, user.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "local", Context: "local", Namespace: "workspaces", ProvisionerTags: map[string]string{"cluster": "local", provisionersdk.TagNetworkPolicy: "0"}, ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	require.Nil(t, target.AppliedRevisionID)
	require.Equal(t, "idle", target.Status)
	require.Equal(t, "1", target.ProvisionerTags[provisionersdk.TagNetworkPolicy])
	p, err = client.NetworkPolicy(ctx, user.OrganizationID, p.ID)
	require.NoError(t, err)
	runs, err := client.PublishNetworkPolicy(ctx, user.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{RevisionID: revision.ID, TargetIDs: []uuid.UUID{target.ID}, Operation: codersdk.NetworkPolicyOperationApply, ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "pending", runs[0].Status)
	//nolint:gocritic // Inspect persisted jobs without exercising a real cluster.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	job, err := db.GetProvisionerJobByID(systemCtx, runs[0].JobID)
	require.NoError(t, err)
	require.Equal(t, database.ProvisionerJobTypeNetworkPolicy, job.Type)
	require.Equal(t, database.ProvisionerTypeTerraform, job.Provisioner)
	require.Equal(t, "1", job.Tags[provisionersdk.TagNetworkPolicy])
	var input map[string]string
	require.NoError(t, json.Unmarshal(job.Input, &input))
	require.Equal(t, runs[0].ID.String(), input["network_policy_run_id"])
	storedRevision, err := db.GetNetworkPolicyRevisionByID(systemCtx, revision.ID)
	require.NoError(t, err)
	require.Equal(t, storedRevision.FileID, job.FileID)
	file, err := db.GetFileByID(systemCtx, job.FileID)
	require.NoError(t, err)
	require.NotEmpty(t, file.Data)
	p, err = client.NetworkPolicy(ctx, user.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Nil(t, p.Targets[0].AppliedRevisionID, "queueing must not claim successful apply")
	require.Equal(t, revision.ID, *p.Targets[0].DesiredRevisionID)
	require.Equal(t, runs[0].JobID, *p.Targets[0].LastJobID)
	_, err = client.PublishNetworkPolicy(ctx, user.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{RevisionID: revision.ID, TargetIDs: []uuid.UUID{target.ID}, Operation: codersdk.NetworkPolicyOperationPlan, ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.SetNetworkPolicyTemplates(ctx, user.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	// Quarantined runs retain their target reservation even after the job is
	// marked complete. Surface that state without exposing Terraform state.
	require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID: job.ID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		Error:     sql.NullString{String: "Apply outcome requires reconciliation.", Valid: true},
		ErrorCode: sql.NullString{String: "NETWORK_POLICY_STATE_UNKNOWN", Valid: true},
	}))
	p, err = client.NetworkPolicy(ctx, user.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", p.Targets[0].Status)
	require.True(t, p.Targets[0].Drifted)
	require.Equal(t, "Apply outcome requires reconciliation.", p.Targets[0].LastError)
	_, err = client.PublishNetworkPolicy(ctx, user.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{RevisionID: revision.ID, TargetIDs: []uuid.UUID{target.ID}, Operation: codersdk.NetworkPolicyOperationApply, ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
}

func TestNetworkPolicyAuthorizationAndIsolation(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	member, memberUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	ctx := t.Context()
	allowed := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID, UserACL: database.TemplateACL{memberUser.ID.String(): []policy.Action{policy.ActionRead, policy.ActionUpdate}}})
	denied := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	administrable, err := member.NetworkPolicyTemplates(ctx, owner.OrganizationID)
	require.NoError(t, err)
	require.Len(t, administrable, 1)
	require.Equal(t, allowed.ID, administrable[0].ID)
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "shared", HCL: testNetworkPolicyHCL, TemplateIDs: []uuid.UUID{allowed.ID, denied.ID}})
	require.NoError(t, err)
	_, err = member.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	list, err := member.NetworkPolicies(ctx, owner.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, list)
	// Removing the denied attachment must not bypass old-set authorization.
	_, err = member.SetNetworkPolicyTemplates(ctx, owner.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{TemplateIDs: []uuid.UUID{allowed.ID}, ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	// A template ACL admin can create and manage policies on that template only.
	single, err := member.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "mine", HCL: testNetworkPolicyHCL, TemplateIDs: []uuid.UUID{allowed.ID}})
	require.NoError(t, err)
	_, err = member.SetNetworkPolicyTemplates(ctx, owner.OrganizationID, single.ID, codersdk.SetNetworkPolicyTemplatesRequest{TemplateIDs: []uuid.UUID{allowed.ID, denied.ID}, ExpectedGeneration: single.Generation})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	_, err = client.NetworkPolicy(ctx, otherOrg.ID, p.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	foreign := dbgen.Template(t, db, database.Template{OrganizationID: otherOrg.ID, CreatedBy: owner.UserID})
	_, err = client.SetNetworkPolicyTemplates(ctx, owner.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{TemplateIDs: []uuid.UUID{foreign.ID}, ExpectedGeneration: p.Generation})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
}

func TestNetworkPolicyConcurrentPublication(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "concurrent", HCL: testNetworkPolicyHCL})
	require.NoError(t, err)
	target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{Cluster: "local", Context: "local", Namespace: "workspaces", ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	request := codersdk.PublishNetworkPolicyRequest{RevisionID: p.Revisions[0].ID, TargetIDs: []uuid.UUID{target.ID}, Operation: codersdk.NetworkPolicyOperationPlan, ExpectedGeneration: p.Generation}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			_, err := client.PublishNetworkPolicy(ctx, owner.OrganizationID, p.ID, request)
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
			conflicts++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflicts)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Len(t, p.Runs, 1)
	require.Nil(t, p.Targets[0].AppliedRevisionID)
	log := dbgen.ProvisionerJobLog(t, db, database.ProvisionerJobLog{JobID: p.Runs[0].JobID, Output: "Plan: 1 to add, 0 to change, 0 to destroy."})
	//nolint:gocritic // Complete a fixture job without executing Terraform.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
		ID: p.Runs[0].JobID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
	}))
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	require.Contains(t, p.Runs[0].Plan, log.Output)
	logs, err := client.NetworkPolicyRunLogs(ctx, owner.OrganizationID, p.ID, p.Runs[0].ID, 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, log.Output, logs[0].Output)
}

func TestNetworkPolicyLiveAttachmentPublication(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	first := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	second := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{
		Name: "live-selectors", HCL: testNetworkPolicyHCL, TemplateIDs: []uuid.UUID{first.ID, second.ID},
	})
	require.NoError(t, err)
	target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{
		Cluster: "local", Context: "local", Namespace: "workspaces", ExpectedGeneration: p.Generation,
	})
	require.NoError(t, err)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	runs, err := client.PublishNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{
		RevisionID: p.Revisions[0].ID, TargetIDs: []uuid.UUID{target.ID}, Operation: codersdk.NetworkPolicyOperationApply, ExpectedGeneration: p.Generation,
	})
	require.NoError(t, err)
	// Seed completed infrastructure without contacting a cluster. The API must
	// preserve this state and enqueue a selector update, never a destroy job.
	//nolint:gocritic // Test fixture for a previously completed Terraform apply.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	complete := func(run codersdk.NetworkPolicyRun) {
		t.Helper()
		stored, err := db.GetNetworkPolicyTargetByID(systemCtx, target.ID)
		require.NoError(t, err)
		_, err = db.CompleteNetworkPolicyTargetJob(systemCtx, database.CompleteNetworkPolicyTargetJobParams{
			ID: target.ID, JobID: uuid.NullUUID{UUID: run.JobID, Valid: true}, Generation: stored.Generation,
			State:             []byte(`{"resources":[{"type":"kubernetes_manifest"}]}`),
			AppliedRevisionID: uuid.NullUUID{UUID: run.RevisionID, Valid: true}, Status: "succeeded", UpdatedAt: dbtime.Now(),
		})
		require.NoError(t, err)
		require.NoError(t, db.UpdateProvisionerJobWithCompleteByID(systemCtx, database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: run.JobID, UpdatedAt: dbtime.Now(), CompletedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}))
	}
	complete(runs[0])
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	p, err = client.SetNetworkPolicyTemplates(ctx, owner.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{
		TemplateIDs: []uuid.UUID{second.ID}, ExpectedGeneration: p.Generation,
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{second.ID}, p.TemplateIDs)
	require.Len(t, p.Runs, 2)
	require.Equal(t, "pending", p.Targets[0].Status)
	require.Equal(t, p.Revisions[0].ID, *p.Targets[0].AppliedRevisionID)
	stored, err := db.GetNetworkPolicyTargetByID(systemCtx, target.ID)
	require.NoError(t, err)
	require.NotEmpty(t, stored.State, "selector update must preserve Terraform state")
	newRun, err := db.GetNetworkPolicyRunByJobID(systemCtx, *p.Targets[0].LastJobID)
	require.NoError(t, err)
	require.Equal(t, "apply", newRun.Operation)
	require.Equal(t, []uuid.UUID{second.ID}, newRun.TemplateIDs)
	oldRun, err := db.GetNetworkPolicyRunByID(systemCtx, runs[0].ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{first.ID, second.ID}, oldRun.TemplateIDs)
	complete(codersdk.NetworkPolicyRun{JobID: newRun.JobID, RevisionID: newRun.RevisionID})
	p, err = client.SetNetworkPolicyTemplates(ctx, owner.OrganizationID, p.ID, codersdk.SetNetworkPolicyTemplatesRequest{ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	require.Empty(t, p.TemplateIDs)
	lastRun, err := db.GetNetworkPolicyRunByJobID(systemCtx, *p.Targets[0].LastJobID)
	require.NoError(t, err)
	require.Equal(t, "apply", lastRun.Operation)
	require.Empty(t, lastRun.TemplateIDs, "the module removes its resource via an empty desired selector set")
}

func TestNetworkPolicyQueueCapability(t *testing.T) {
	t.Parallel()
	client, db := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "capability", HCL: testNetworkPolicyHCL})
	require.NoError(t, err)
	// Seed a target written before the required capability was enforced.
	//nolint:gocritic // Fixture verifies queueing repairs unsafe historical routing tags.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	target, err := db.InsertNetworkPolicyTarget(systemCtx, database.InsertNetworkPolicyTargetParams{
		ID: uuid.New(), PolicyID: p.ID, OrganizationID: owner.OrganizationID,
		Cluster: "local", Context: "local", Namespace: "workspaces", Tags: json.RawMessage(`{"coder.network_policy":"0"}`),
		CreatedAt: dbtime.Now(), UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	runs, err := client.PublishNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{
		RevisionID: p.Revisions[0].ID, TargetIDs: []uuid.UUID{target.ID}, Operation: codersdk.NetworkPolicyOperationPlan, ExpectedGeneration: p.Generation,
	})
	require.NoError(t, err)
	job, err := db.GetProvisionerJobByID(systemCtx, runs[0].JobID)
	require.NoError(t, err)
	require.Equal(t, "1", job.Tags[provisionersdk.TagNetworkPolicy])
}

func TestNetworkPolicyBuiltInCapability(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		experiment, module bool
	}{
		{"disabled", false, false},
		{"experiment-only", true, false},
		{"module-only", false, true},
		{"explicit-opt-in", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			values := coderdtest.DeploymentValues(t)
			if tc.experiment {
				values.Experiments = serpent.StringArray{string(codersdk.ExperimentNetworkPolicies)}
			}
			if tc.module {
				values.NetworkPolicyModulePath = serpent.String(t.TempDir())
			}
			client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: values})
			owner := coderdtest.CreateFirstUser(t, client)
			daemon, err := api.CreateInMemoryProvisionerDaemon(t.Context(), "native-test", []codersdk.ProvisionerType{codersdk.ProvisionerTypeTerraform})
			require.NoError(t, err)
			t.Cleanup(func() { _ = daemon.DRPCConn().Close() })
			//nolint:gocritic // Inspect the daemon capability registered by the server.
			daemons, err := api.Database.GetProvisionerDaemonsByOrganization(dbauthz.AsSystemRestricted(t.Context()), database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: owner.OrganizationID})
			require.NoError(t, err)
			require.Len(t, daemons, 1)
			require.NotContains(t, daemons[0].Tags, provisionersdk.TagNetworkPolicy,
				"ordinary built-ins must retain exact tag matching for untagged jobs")
		})
	}
}

func TestNetworkPolicyDedicatedWorkerRouting(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"1.21", "1.22"} {
		for _, operation := range []codersdk.NetworkPolicyOperation{codersdk.NetworkPolicyOperationPlan, codersdk.NetworkPolicyOperationApply} {
			t.Run(version+"/"+string(operation), func(t *testing.T) {
				t.Parallel()
				assertNetworkPolicyDedicatedWorkerRouting(t, version, operation)
			})
		}
	}
}

func assertNetworkPolicyDedicatedWorkerRouting(t *testing.T, version string, operation codersdk.NetworkPolicyOperation) {
	t.Helper()
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentNetworkPolicies)}
	module := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte("# trusted module\n"), 0600))
	values.NetworkPolicyModulePath = serpent.String(module)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: values})
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	//nolint:gocritic // Exercise the real SQL acquisition path with registered worker tags.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "routing", HCL: testNetworkPolicyHCL})
	require.NoError(t, err)
	revision, err := api.Database.GetNetworkPolicyRevisionByID(systemCtx, p.Revisions[0].ID)
	require.NoError(t, err)
	ordinaryJob, err := api.Database.InsertProvisionerJob(systemCtx, database.InsertProvisionerJobParams{
		ID: uuid.New(), CreatedAt: dbtime.Now(), UpdatedAt: dbtime.Now(),
		OrganizationID: owner.OrganizationID, InitiatorID: owner.UserID, FileID: revision.FileID,
		Provisioner: database.ProvisionerTypeTerraform, Type: database.ProvisionerJobTypeTemplateVersionImport,
		StorageMethod: database.ProvisionerStorageMethodFile, Input: json.RawMessage(`{}`),
		Tags: provisionersdk.MutateTags(owner.UserID, nil), TraceMetadata: pqtype.NullRawMessage{}, LogsOverflowed: false,
	})
	require.NoError(t, err)
	target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{
		Cluster: "local", Context: "local", Namespace: "workspaces", ExpectedGeneration: p.Generation,
		ProvisionerTags: map[string]string{provisionersdk.TagNetworkPolicyDestroy: "1"},
	})
	require.NoError(t, err)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	runs, err := client.PublishNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.PublishNetworkPolicyRequest{
		RevisionID: revision.ID, TargetIDs: []uuid.UUID{target.ID}, Operation: operation, ExpectedGeneration: p.Generation,
	})
	require.NoError(t, err)
	ordinary, err := api.CreateInMemoryProvisionerDaemon(ctx, "ordinary", []codersdk.ProvisionerType{codersdk.ProvisionerTypeTerraform})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ordinary.DRPCConn().Close() })
	dedicated, err := api.CreateInMemoryTaggedProvisionerDaemon(ctx, "policy", []codersdk.ProvisionerType{codersdk.ProvisionerTypeTerraform}, map[string]string{provisionersdk.TagNetworkPolicy: "1"}, coderd.MemoryProvisionerWithVersionOverride(version))
	require.NoError(t, err)
	t.Cleanup(func() { _ = dedicated.DRPCConn().Close() })
	daemons, err := api.Database.GetProvisionerDaemonsByOrganization(systemCtx, database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: owner.OrganizationID})
	require.NoError(t, err)
	require.Len(t, daemons, 2)
	for _, daemon := range daemons {
		tags, err := json.Marshal(daemon.Tags)
		require.NoError(t, err)
		args := database.AcquireProvisionerJobParams{
			OrganizationID: owner.OrganizationID, Types: []database.ProvisionerType{database.ProvisionerTypeTerraform},
			ProvisionerTags: tags, WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
			StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}
		job, err := api.Database.AcquireProvisionerJob(systemCtx, args)
		require.NoError(t, err)
		if daemon.Tags[provisionersdk.TagNetworkPolicy] == "1" {
			require.NotContains(t, job.Tags, provisionersdk.TagNetworkPolicyDestroy)
			require.Equal(t, runs[0].JobID, job.ID)
		} else {
			require.Equal(t, ordinaryJob.ID, job.ID)
		}
		_, err = api.Database.AcquireProvisionerJob(systemCtx, args)
		require.ErrorIs(t, err, sql.ErrNoRows, "neither worker can acquire the other class of job")
	}
}

func TestNetworkPolicyDestroyWorkerRouting(t *testing.T) {
	t.Parallel()
	module := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(module, "main.tf"), []byte("# trusted module\n"), 0600))
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentNetworkPolicies)}
	values.NetworkPolicyModulePath = serpent.String(module)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: values})
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	//nolint:gocritic // Exercise SQL acquisition with real registered worker tags.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	p, err := client.CreateNetworkPolicy(ctx, owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "destroy-routing", HCL: testNetworkPolicyHCL})
	require.NoError(t, err)
	target, err := client.CreateNetworkPolicyTarget(ctx, owner.OrganizationID, p.ID, codersdk.CreateNetworkPolicyTargetRequest{
		Cluster: "local", Context: "local", Namespace: "workspaces", ExpectedGeneration: p.Generation,
		ProvisionerTags: map[string]string{provisionersdk.TagNetworkPolicyDestroy: "0"},
	})
	require.NoError(t, err)
	seedNetworkPolicyApply(t, api.Database, p, target.ID, owner.UserID, false, false)
	p, err = client.NetworkPolicy(ctx, owner.OrganizationID, p.ID)
	require.NoError(t, err)
	p, err = client.DeleteNetworkPolicy(ctx, owner.OrganizationID, p.ID, codersdk.DeleteNetworkPolicyRequest{ExpectedGeneration: p.Generation})
	require.NoError(t, err)
	destroy := p.Runs[0]
	require.Equal(t, codersdk.NetworkPolicyOperationDestroy, destroy.Operation)
	job, err := api.Database.GetProvisionerJobByID(systemCtx, destroy.JobID)
	require.NoError(t, err)
	require.Equal(t, "1", job.Tags[provisionersdk.TagNetworkPolicyDestroy])

	for _, version := range []string{"1.21", "1.22"} {
		worker, err := api.CreateInMemoryTaggedProvisionerDaemon(ctx, "policy-"+version,
			[]codersdk.ProvisionerType{codersdk.ProvisionerTypeTerraform},
			map[string]string{provisionersdk.TagNetworkPolicy: "1", provisionersdk.TagNetworkPolicyDestroy: "1"},
			coderd.MemoryProvisionerWithVersionOverride(version))
		require.NoError(t, err)
		t.Cleanup(func() { _ = worker.DRPCConn().Close() })
		daemons, err := api.Database.GetProvisionerDaemonsByOrganization(systemCtx, database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: owner.OrganizationID})
		require.NoError(t, err)
		var daemon database.ProvisionerDaemon
		for _, candidate := range daemons {
			if candidate.Name == "policy-"+version {
				daemon = candidate
			}
		}
		require.NotEqual(t, uuid.Nil, daemon.ID)
		tags, err := json.Marshal(daemon.Tags)
		require.NoError(t, err)
		acquired, err := api.Database.AcquireProvisionerJob(systemCtx, database.AcquireProvisionerJobParams{
			OrganizationID: owner.OrganizationID, Types: []database.ProvisionerType{database.ProvisionerTypeTerraform},
			ProvisionerTags: tags, WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true},
			StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		})
		if version == "1.21" {
			require.NotContains(t, daemon.Tags, provisionersdk.TagNetworkPolicyDestroy)
			require.ErrorIs(t, err, sql.ErrNoRows, "plan/apply workers must not acquire destroy")
			pending, err := api.Database.GetProvisionerJobByID(systemCtx, destroy.JobID)
			require.NoError(t, err)
			require.False(t, pending.StartedAt.Valid)
			require.False(t, pending.WorkerID.Valid)
		} else {
			require.Equal(t, "1", daemon.Tags[provisionersdk.TagNetworkPolicyDestroy])
			require.NoError(t, err)
			require.Equal(t, destroy.JobID, acquired.ID)
		}
	}
}

func TestNetworkPolicyRejectsUnsafeHCL(t *testing.T) {
	t.Parallel()
	client, _ := newNetworkPolicyClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	for _, hcl := range []string{`provider "kubernetes" {}`, `module "coder_network_policy" { source = "https://example.com/module" }`, `resource "null_resource" "exec" {}`} {
		_, err := client.CreateNetworkPolicy(t.Context(), owner.OrganizationID, codersdk.CreateNetworkPolicyRequest{Name: "invalid", HCL: hcl})
		require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())
	}
	list, err := client.NetworkPolicies(t.Context(), owner.OrganizationID)
	require.NoError(t, err)
	require.Empty(t, list)
}
