package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

// staleFoundationSettingsStore simulates a settings change between the handler's
// initial lookup and transaction. InTx retains the real, current settings.
type staleFoundationSettingsStore struct {
	database.Store
	stale atomic.Bool
}

func (s *staleFoundationSettingsStore) GetFoundationByID(ctx context.Context, id uuid.UUID) (database.Foundation, error) {
	f, err := s.Store.GetFoundationByID(ctx, id)
	if s.stale.Load() {
		f.PreventDestroyWithWorkspaces = !f.PreventDestroyWithWorkspaces
	}
	return f, err
}

func TestFoundationDestroyProtection(t *testing.T) {
	t.Parallel()
	rawDB, ps := dbtestutil.NewDB(t)
	store := &staleFoundationSettingsStore{Store: rawDB}
	values := coderdtest.DeploymentValues(t)
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentTemplateResources)}
	client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{Database: store, Pubsub: ps, DeploymentValues: values})
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	//nolint:gocritic // Simulate authoritative worker results without external infrastructure.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	f, err := client.CreateFoundation(ctx, owner.OrganizationID, codersdk.CreateFoundationRequest{Name: "protected"})
	require.NoError(t, err)
	require.True(t, f.PreventDestroyWithWorkspaces)
	explicit, err := client.CreateFoundation(ctx, owner.OrganizationID, codersdk.CreateFoundationRequest{Name: "unprotected", PreventDestroyWithWorkspaces: new(false)})
	require.NoError(t, err)
	require.False(t, explicit.PreventDestroyWithWorkspaces)
	template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	_, err = client.SetTemplateFoundation(ctx, template.ID, codersdk.SetTemplateFoundationRequest{FoundationID: f.ID})
	require.NoError(t, err)
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: template.ID, OrganizationID: owner.OrganizationID, OwnerID: owner.UserID})
	f, err = client.Foundation(ctx, owner.OrganizationID, f.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.FoundationConsumers{Total: 1, Unknown: 1}, f.Consumers, "unconfigured Foundations still report consumers")
	reader, readerUser := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	role, err := db.InsertCustomRole(dbauthz.As(ctx, rbac.Subject{ID: owner.UserID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleOwner()}, Scope: rbac.ScopeAll}), database.InsertCustomRoleParams{Name: "foundation-reader", OrganizationID: uuid.NullUUID{UUID: owner.OrganizationID, Valid: true}, SitePermissions: database.CustomRolePermissions{}, UserPermissions: database.CustomRolePermissions{}, OrgPermissions: database.CustomRolePermissions{{ResourceType: "foundation", Action: "read"}}})
	require.NoError(t, err)
	_, err = db.UpdateMemberRoles(systemCtx, database.UpdateMemberRolesParams{UserID: readerUser.ID, OrgID: owner.OrganizationID, GrantedRoles: []string{role.Name}})
	require.NoError(t, err)
	_, err = reader.Workspace(ctx, workspace.ID)
	require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
	visible, err := reader.Foundation(ctx, owner.OrganizationID, f.ID)
	require.NoError(t, err)
	require.Equal(t, f.Consumers, visible.Consumers, "aggregate counts include consumers hidden from a Foundation reader")
	revision, err := client.CreateFoundationRevision(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRevisionRequest{Source: "# root"})
	require.NoError(t, err)
	apply, err := client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "apply"})
	require.NoError(t, err)
	_, err = db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{FoundationID: f.ID, RunID: uuid.NullUUID{UUID: apply.ID, Valid: true}, Status: "applied", State: []byte(`{"resources":[]}`), Outputs: json.RawMessage(`{}`), AppliedRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, CheckpointRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}, OrganizationID: owner.OrganizationID, JobID: apply.Job.ID, CreatedBy: owner.UserID})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: workspace.ID, TemplateVersionID: version.ID, JobID: apply.Job.ID, InitiatorID: owner.UserID})
	snapshot := json.RawMessage(`{"retained":"historical input"}`)
	require.NoError(t, db.PinWorkspaceBuildTemplateResources(systemCtx, database.PinWorkspaceBuildTemplateResourcesParams{ID: build.ID, TemplateResourceRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, TemplateResourceOutputs: snapshot, StateTemplateResourceRevisionID: uuid.NullUUID{UUID: revision.ID, Valid: true}, StateTemplateResourceOutputs: snapshot}))
	store.stale.Store(true)
	_, err = client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "destroy"})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode(), "destroy checks fresh settings under lock, not the stale initial lookup")
	store.stale.Store(false)
	f, err = client.UpdateFoundation(ctx, owner.OrganizationID, f.ID, codersdk.UpdateFoundationRequest{Name: f.Name, PreventDestroyWithWorkspaces: new(false)})
	require.NoError(t, err)
	require.False(t, f.PreventDestroyWithWorkspaces)
	f, err = client.UpdateFoundation(ctx, owner.OrganizationID, f.ID, codersdk.UpdateFoundationRequest{Name: f.Name})
	require.NoError(t, err)
	require.False(t, f.PreventDestroyWithWorkspaces, "omitted setting preserves explicit false")
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, client.DeleteFoundation(ctx, owner.OrganizationID, f.ID)).StatusCode(), "disabling protection does not bypass cleanup")
	store.stale.Store(true)
	destroy, err := client.CreateFoundationRun(ctx, owner.OrganizationID, f.ID, codersdk.CreateTemplateResourceRunRequest{RevisionID: revision.ID, Action: "destroy"})
	require.NoError(t, err, "stale enabled protection does not override the locked record")
	store.stale.Store(false)
	_, err = db.CompleteTemplateResourceRun(systemCtx, database.CompleteTemplateResourceRunParams{FoundationID: f.ID, RunID: uuid.NullUUID{UUID: destroy.ID, Valid: true}, Status: "destroyed", State: []byte(`{"resources":[]}`), Outputs: json.RawMessage(`{}`), UpdatedAt: dbtime.Now()})
	require.NoError(t, err)
	require.NoError(t, client.DeleteFoundation(ctx, owner.OrganizationID, f.ID), "successful cleanup permits removal with surviving workspaces")
	bound, err := client.TemplateFoundation(ctx, template.ID)
	require.NoError(t, err)
	require.Nil(t, bound)
	_, err = client.SetTemplateFoundation(ctx, template.ID, codersdk.SetTemplateFoundationRequest{FoundationID: explicit.ID})
	require.NoError(t, err, "cleaned-up Foundation can be replaced")
	retained, err := db.GetWorkspaceBuildByID(systemCtx, build.ID)
	require.NoError(t, err)
	require.Equal(t, revision.ID, retained.TemplateResourceRevisionID.UUID)
	require.JSONEq(t, string(snapshot), string(retained.TemplateResourceOutputs))
	require.Equal(t, revision.ID, retained.StateTemplateResourceRevisionID.UUID)
	require.JSONEq(t, string(snapshot), string(retained.StateTemplateResourceOutputs))
}

func TestFoundationBindingRequiresUse(t *testing.T) {
	t.Parallel()
	client, db := newTemplateResourceClient(t)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := t.Context()
	//nolint:gocritic // Configure role fixtures without enterprise role-management APIs.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	f, err := client.CreateFoundation(ctx, owner.OrganizationID, codersdk.CreateFoundationRequest{Name: "binding"})
	require.NoError(t, err)
	template := dbgen.Template(t, db, database.Template{OrganizationID: owner.OrganizationID, CreatedBy: owner.UserID})
	for _, action := range []string{"update", "use"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			member, user := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
			role, err := db.InsertCustomRole(dbauthz.As(ctx, rbac.Subject{ID: owner.UserID.String(), Roles: rbac.RoleIdentifiers{rbac.RoleOwner()}, Scope: rbac.ScopeAll}), database.InsertCustomRoleParams{Name: "binder-" + action, OrganizationID: uuid.NullUUID{UUID: owner.OrganizationID, Valid: true}, SitePermissions: database.CustomRolePermissions{}, UserPermissions: database.CustomRolePermissions{}, OrgPermissions: database.CustomRolePermissions{{ResourceType: "foundation", Action: "read"}, {ResourceType: "foundation", Action: policy.Action(action)}, {ResourceType: "template", Action: "read"}, {ResourceType: "template", Action: "update"}}})
			require.NoError(t, err)
			_, err = db.UpdateMemberRoles(systemCtx, database.UpdateMemberRolesParams{UserID: user.ID, OrgID: owner.OrganizationID, GrantedRoles: []string{role.Name}})
			require.NoError(t, err)
			_, err = member.SetTemplateFoundation(ctx, template.ID, codersdk.SetTemplateFoundationRequest{FoundationID: f.ID})
			if action == "use" {
				require.NoError(t, err)
			} else {
				require.Equal(t, http.StatusNotFound, coderdtest.SDKError(t, err).StatusCode())
			}
		})
	}
}
