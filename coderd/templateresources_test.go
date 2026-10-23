package coderd_test

import (
	"archive/tar"
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

func newTemplateResourceClient(t *testing.T) (*codersdk.Client, database.Store) {
	t.Helper()
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentTemplateResources)}
	return coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: values})
}

func TestTemplateResourcesNativeLifecycle(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	//nolint:gocritic // Inspect persistence without provisioning cloud resources.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	initial, err := client.TemplateResources(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, "unconfigured", initial.Status)
	require.Empty(t, initial.Revisions)
	source := `resource "terraform_data" "example" { input = "hello" }
output "value" { value = terraform_data.example.output }
`
	revision, err := client.CreateTemplateResourceRevision(ctx, template.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: source})
	require.NoError(t, err)
	newer, err := client.CreateTemplateResourceRevision(ctx, template.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: source + "\n# future version\n"})
	require.NoError(t, err)
	run, err := client.CreateTemplateResourceRun(ctx, template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
	require.NoError(t, err)
	job, err := db.GetProvisionerJobByID(systemCtx, run.Job.ID)
	require.NoError(t, err)
	require.Equal(t, database.ProvisionerJobTypeTemplateResources, job.Type)
	require.Equal(t, "1", job.Tags[provisionersdk.TagTemplateResources])
	var input map[string]string
	require.NoError(t, json.Unmarshal(job.Input, &input))
	require.Equal(t, run.ID.String(), input["template_resource_run_id"])
	archive, err := db.GetFileByID(systemCtx, job.FileID)
	require.NoError(t, err)
	reader := tar.NewReader(bytes.NewReader(archive.Data))
	header, err := reader.Next()
	require.NoError(t, err)
	require.Equal(t, "main.tf", header.Name)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, source, string(content), "job must use immutable older source")
	_, err = reader.Next()
	require.ErrorIs(t, err, io.EOF)
	detail, err := client.TemplateResources(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", detail.Status)
	require.Equal(t, newer.ID, *detail.CurrentRevisionID)
	require.Nil(t, detail.AppliedRevisionID)
	_, err = client.CreateTemplateResourceRun(ctx, template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: newer.ID, Action: "plan"})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	err = client.DeleteTemplate(ctx, template.ID)
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())

	state, err := db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Status: "applied", State: []byte("private terraform state"), Outputs: json.RawMessage(`{"value":"hello","nested":{"list":[1,true]}}`), AppliedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	detail, err = client.TemplateResources(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, "applied", detail.Status)
	require.Equal(t, "hello", detail.Outputs["value"])
	require.Equal(t, revision.ID, *detail.AppliedRevisionID)
	require.Equal(t, newer.ID, *detail.CurrentRevisionID)
	err = client.DeleteTemplate(ctx, template.ID)
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	_, err = client.CreateTemplateResourceRun(ctx, template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: newer.ID, Action: "destroy"})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: template.ID, OrganizationID: template.OrganizationID, OwnerID: owner.UserID})
	_, err = client.CreateTemplateResourceRun(ctx, template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "destroy"})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	require.NoError(t, db.UpdateWorkspaceDeletedByID(systemCtx, database.UpdateWorkspaceDeletedByIDParams{ID: workspace.ID, Deleted: true}))
	destroy, err := client.CreateTemplateResourceRun(ctx, template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "destroy"})
	require.NoError(t, err)
	destroyJob, err := db.GetProvisionerJobByID(systemCtx, destroy.Job.ID)
	require.NoError(t, err)
	require.Equal(t, job.FileID, destroyJob.FileID)
	// Stale apply callbacks cannot clobber a later destroy reservation.
	_, err = db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Status: "applied", State: state.State, Outputs: state.Outputs, AppliedRevisionID: state.AppliedRevisionID, UpdatedAt: dbtime.Now()})
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: destroy.ID, Valid: true}, Status: "destroyed", State: []byte{}, Outputs: json.RawMessage(`{}`), UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	require.NoError(t, client.DeleteTemplate(ctx, template.ID))
	stored, err := db.GetTemplateResourceRevisionByID(systemCtx, revision.ID)
	require.NoError(t, err)
	require.Equal(t, source, stored.Source, "soft deletion retains history and archives")
}

func TestTemplateResourcesAuthorization(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	member, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID, UserACL: database.TemplateACL{user.ID.String(): []policy.Action{policy.ActionRead, policy.ActionUpdate}}})
	denied := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	revision, err := member.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# generic Terraform root"})
	require.NoError(t, err, "template ACL admins can edit resources")
	deniedRevision, err := client.CreateTemplateResourceRevision(t.Context(), denied.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# private"})
	require.NoError(t, err)
	_, err = member.TemplateResources(t.Context(), denied.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	_, err = member.CreateTemplateResourceRun(t.Context(), template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: deniedRevision.ID, Action: "apply"})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	run, err := member.CreateTemplateResourceRun(t.Context(), template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "plan"})
	require.NoError(t, err)
	_, err = member.TemplateResourceRunLogs(t.Context(), denied.ID, run.ID, 0)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	other := dbgen.Template(t, db, database.Template{OrganizationID: otherOrg.ID, CreatedBy: owner.UserID})
	foreign, err := client.CreateTemplateResourceRevision(t.Context(), other.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# different organization"})
	require.NoError(t, err)
	_, err = client.CreateTemplateResourceRun(t.Context(), denied.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: foreign.ID, Action: "apply"})
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
}

func TestTemplateResourcesConcurrentRuns(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	revision, err := client.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# root"})
	require.NoError(t, err)
	gate := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			_, err := client.CreateTemplateResourceRun(t.Context(), template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
			results <- err
		}()
	}
	close(gate)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
		}
	}
	require.Equal(t, 1, successes)
	detail, err := client.TemplateResources(t.Context(), template.ID)
	require.NoError(t, err)
	require.Len(t, detail.Runs, 1)
	//nolint:gocritic // Simulate a lost worker after an acquired mutation.
	systemCtx := dbauthz.AsSystemRestricted(t.Context())
	_, err = db.UpdateTemplateResourceState(systemCtx, database.UpdateTemplateResourceStateParams{
		TemplateID: template.ID, RunID: uuid.NullUUID{UUID: detail.Runs[0].ID, Valid: true}, Status: "unknown", State: []byte("retained checkpoint"), UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	detail, err = client.TemplateResources(t.Context(), template.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", detail.Status, "unknown takes precedence over the retained reservation")
	_, err = client.CreateTemplateResourceRun(t.Context(), template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
	err = client.DeleteTemplate(t.Context(), template.ID)
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())
}

func TestTemplateResourcesInMemoryWorkerRouting(t *testing.T) {
	t.Parallel()
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentTemplateResources)}
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{DeploymentValues: values})
	owner := coderdtest.CreateFirstUser(t, client)
	template := dbgen.Template(t, api.Database, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	revision, err := client.CreateTemplateResourceRevision(t.Context(), template.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# root"})
	require.NoError(t, err)
	run, err := client.CreateTemplateResourceRun(t.Context(), template.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "plan"})
	require.NoError(t, err)
	//nolint:gocritic // Exercise real SQL acquisition using registered worker tags.
	systemCtx := dbauthz.AsSystemRestricted(t.Context())
	for _, version := range []string{"1.22", "1.23"} {
		worker, err := api.CreateInMemoryTaggedProvisionerDaemon(t.Context(), "resources-"+version,
			[]codersdk.ProvisionerType{codersdk.ProvisionerTypeTerraform},
			map[string]string{provisionersdk.TagTemplateResources: provisionersdk.TemplateResourcesCapabilityVersion, "region": "test"},
			coderd.MemoryProvisionerWithVersionOverride(version))
		require.NoError(t, err)
		t.Cleanup(func() { _ = worker.DRPCConn().Close() })
		daemons, err := api.Database.GetProvisionerDaemonsByOrganization(systemCtx, database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: owner.OrganizationID})
		require.NoError(t, err)
		var daemon database.ProvisionerDaemon
		for _, candidate := range daemons {
			if candidate.Name == "resources-"+version {
				daemon = candidate
			}
		}
		require.NotEqual(t, uuid.Nil, daemon.ID)
		require.Equal(t, "test", daemon.Tags["region"], "version filtering must preserve ordinary routing")
		tags, err := json.Marshal(daemon.Tags)
		require.NoError(t, err)
		acquired, err := api.Database.AcquireProvisionerJob(systemCtx, database.AcquireProvisionerJobParams{
			OrganizationID: owner.OrganizationID, Types: []database.ProvisionerType{database.ProvisionerTypeTerraform}, ProvisionerTags: tags,
			WorkerID: uuid.NullUUID{UUID: daemon.ID, Valid: true}, StartedAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		})
		if version == "1.22" {
			require.NotContains(t, daemon.Tags, provisionersdk.TagTemplateResources)
			require.ErrorIs(t, err, sql.ErrNoRows)
		} else {
			require.Equal(t, provisionersdk.TemplateResourcesCapabilityVersion, daemon.Tags[provisionersdk.TagTemplateResources])
			require.NoError(t, err)
			require.Equal(t, run.Job.ID, acquired.ID)
		}
	}
}

func TestTemplateResourcesExperimentGate(t *testing.T) {
	t.Parallel()
	client, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	_, err := client.TemplateResources(t.Context(), template.ID)
	require.Equal(t, http.StatusForbidden, coderdtest.SDKError(t, err).StatusCode())
}
