package dbgen_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
)

func TestGenerator(t *testing.T) {
	t.Parallel()

	t.Run("AuditLog", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		_ = dbgen.AuditLog(t, db, database.AuditLog{})
		logs := must(db.GetAuditLogsOffset(context.Background(), database.GetAuditLogsOffsetParams{Limit: 1}))
		require.Len(t, logs, 1)
	})

	t.Run("APIKey", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp, _ := dbgen.APIKey(t, db, database.APIKey{})
		require.Equal(t, exp, must(db.GetAPIKeyByID(context.Background(), exp.ID)))
	})

	t.Run("File", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.File(t, db, database.File{})
		require.Equal(t, exp, must(db.GetFileByID(context.Background(), exp.ID)))
	})

	t.Run("UserLink", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.UserLink(t, db, database.UserLink{})
		require.Equal(t, exp, must(db.GetUserLinkByLinkedID(context.Background(), exp.LinkedID)))
	})

	t.Run("GitAuthLink", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.GitAuthLink(t, db, database.GitAuthLink{})
		require.Equal(t, exp, must(db.GetGitAuthLink(context.Background(), database.GetGitAuthLinkParams{
			ProviderID: exp.ProviderID,
			UserID:     exp.UserID,
		})))
	})

	t.Run("WorkspaceResource", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{})
		require.Equal(t, exp, must(db.GetWorkspaceResourceByID(context.Background(), exp.ID)))
	})

	t.Run("WorkspaceApp", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{})
		require.Equal(t, exp, must(db.GetWorkspaceAppsByAgentID(context.Background(), exp.AgentID))[0])
	})

	t.Run("WorkspaceResourceMetadata", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.WorkspaceResourceMetadatums(t, db, database.WorkspaceResourceMetadatum{})
		require.Equal(t, exp, must(db.GetWorkspaceResourceMetadataByResourceIDs(context.Background(), []uuid.UUID{exp[0].WorkspaceResourceID})))
	})

	t.Run("WorkspaceProxy", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		require.Equal(t, exp, must(db.GetWorkspaceProxyByID(context.Background(), exp.ID)))
	})

	t.Run("Job", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.ProvisionerJob(t, db, database.ProvisionerJob{})
		require.Equal(t, exp, must(db.GetProvisionerJobByID(context.Background(), exp.ID)))
	})

	t.Run("Group", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.Group(t, db, database.Group{})
		require.Equal(t, exp, must(db.GetGroupByID(context.Background(), exp.ID)))
	})

	t.Run("GroupMember", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		g := dbgen.Group(t, db, database.Group{})
		u := dbgen.User(t, db, database.User{})
		exp := []database.User{u}
		dbgen.GroupMember(t, db, database.GroupMember{GroupID: g.ID, UserID: u.ID})

		require.Equal(t, exp, must(db.GetGroupMembers(context.Background(), g.ID)))
	})

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.Organization(t, db, database.Organization{})
		require.Equal(t, exp, must(db.GetOrganizationByID(context.Background(), exp.ID)))
	})

	t.Run("OrganizationMember", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.OrganizationMember(t, db, database.OrganizationMember{})
		require.Equal(t, exp, must(db.GetOrganizationMemberByUserID(context.Background(), database.GetOrganizationMemberByUserIDParams{
			OrganizationID: exp.OrganizationID,
			UserID:         exp.UserID,
		})))
	})

	t.Run("Workspace", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.Workspace(t, db, database.Workspace{})
		require.Equal(t, exp, must(db.GetWorkspaceByID(context.Background(), exp.ID)))
	})

	t.Run("WorkspaceAgent", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
		require.Equal(t, exp, must(db.GetWorkspaceAgentByID(context.Background(), exp.ID)))
	})

	t.Run("Template", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.Template(t, db, database.Template{})
		require.Equal(t, exp, must(db.GetTemplateByID(context.Background(), exp.ID)))
	})

	t.Run("TemplateVersion", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.TemplateVersion(t, db, database.TemplateVersion{})
		require.Equal(t, exp, must(db.GetTemplateVersionByID(context.Background(), exp.ID)))
	})

	t.Run("ParameterSchema", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.ParameterSchema(t, db, database.ParameterSchema{})
		require.Equal(t, []database.ParameterSchema{exp}, must(db.GetParameterSchemasByJobID(context.Background(), exp.JobID)))
	})

	t.Run("ParameterValue", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.ParameterValue(t, db, database.ParameterValue{})
		require.Equal(t, exp, must(db.GetParameterValueByScopeAndName(context.Background(), database.GetParameterValueByScopeAndNameParams{
			Scope:   exp.Scope,
			ScopeID: exp.ScopeID,
			Name:    exp.Name,
		})))
	})

	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
		require.Equal(t, exp, must(db.GetWorkspaceBuildByID(context.Background(), exp.ID)))
	})

	t.Run("User", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.User(t, db, database.User{})
		require.Equal(t, exp, must(db.GetUserByID(context.Background(), exp.ID)))
	})

	t.Run("SSHKey", func(t *testing.T) {
		t.Parallel()
		db := dbfake.New()
		exp := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
		require.Equal(t, exp, must(db.GetGitSSHKey(context.Background(), exp.UserID)))
	})
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
