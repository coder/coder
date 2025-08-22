package dbgen_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

func TestGenerator(t *testing.T) {
	t.Parallel()

	t.Run("AuditLog", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		_ = dbgen.AuditLog(t, db, database.AuditLog{})
		logs := must(db.GetAuditLogsOffset(context.Background(), database.GetAuditLogsOffsetParams{LimitOpt: 1}))
		require.Len(t, logs, 1)
	})

	t.Run("APIKey", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp, _ := dbgen.APIKey(t, db, database.APIKey{})
		require.Equal(t, exp, must(db.GetAPIKeyByID(context.Background(), exp.ID)))
	})

	t.Run("File", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.File(t, db, database.File{})
		require.Equal(t, exp, must(db.GetFileByID(context.Background(), exp.ID)))
	})

	t.Run("UserLink", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		u := dbgen.User(t, db, database.User{})
		exp := dbgen.UserLink(t, db, database.UserLink{UserID: u.ID})
		require.Equal(t, exp, must(db.GetUserLinkByLinkedID(context.Background(), exp.LinkedID)))
	})

	t.Run("GitAuthLink", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{})
		require.Equal(t, exp, must(db.GetExternalAuthLink(context.Background(), database.GetExternalAuthLinkParams{
			ProviderID: exp.ProviderID,
			UserID:     exp.UserID,
		})))
	})

	t.Run("WorkspaceResource", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{})
		require.Equal(t, exp, must(db.GetWorkspaceResourceByID(context.Background(), exp.ID)))
	})

	t.Run("WorkspaceApp", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{})
		require.Equal(t, exp, must(db.GetWorkspaceAppsByAgentID(context.Background(), exp.AgentID))[0])
	})

	t.Run("WorkspaceResourceMetadata", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceResourceMetadatums(t, db, database.WorkspaceResourceMetadatum{})
		require.Equal(t, exp, must(db.GetWorkspaceResourceMetadataByResourceIDs(context.Background(), []uuid.UUID{exp[0].WorkspaceResourceID})))
	})

	t.Run("WorkspaceProxy", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp, secret := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		require.Len(t, secret, 64)
		require.Equal(t, exp, must(db.GetWorkspaceProxyByID(context.Background(), exp.ID)))
	})

	t.Run("Job", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		require.Equal(t, exp, must(db.GetProvisionerJobByID(context.Background(), exp.ID)))
	})

	t.Run("Group", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.Group(t, db, database.Group{})
		require.Equal(t, exp, must(db.GetGroupByID(context.Background(), exp.ID)))
	})

	t.Run("GroupMember", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		g := dbgen.Group(t, db, database.Group{})
		u := dbgen.User(t, db, database.User{})
		gm := dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: g.ID, UserID: u.ID})
		exp := []database.GroupMember{gm}

		require.Equal(t, exp, must(db.GetGroupMembersByGroupID(context.Background(), database.GetGroupMembersByGroupIDParams{
			GroupID:       g.ID,
			IncludeSystem: false,
		})))
	})

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.Organization(t, db, database.Organization{})
		require.Equal(t, exp, must(db.GetOrganizationByID(context.Background(), exp.ID)))
	})

	t.Run("OrganizationMember", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		o := dbgen.Organization(t, db, database.Organization{})
		u := dbgen.User(t, db, database.User{})
		exp := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID})
		require.Equal(t, exp, must(database.ExpectOne(db.OrganizationMembers(context.Background(), database.OrganizationMembersParams{
			OrganizationID: exp.OrganizationID,
			UserID:         exp.UserID,
		}))).OrganizationMember)
	})

	t.Run("Workspace", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		u := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		exp := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		w := must(db.GetWorkspaceByID(context.Background(), exp.ID))
		table := database.WorkspaceTable{
			ID:                w.ID,
			CreatedAt:         w.CreatedAt,
			UpdatedAt:         w.UpdatedAt,
			OwnerID:           w.OwnerID,
			OrganizationID:    w.OrganizationID,
			TemplateID:        w.TemplateID,
			Deleted:           w.Deleted,
			Name:              w.Name,
			AutostartSchedule: w.AutostartSchedule,
			Ttl:               w.Ttl,
			LastUsedAt:        w.LastUsedAt,
			DormantAt:         w.DormantAt,
			DeletingAt:        w.DeletingAt,
			AutomaticUpdates:  w.AutomaticUpdates,
			Favorite:          w.Favorite,
			GroupACL:          database.WorkspaceACL{},
			UserACL:           database.WorkspaceACL{},
		}
		require.Equal(t, exp, table)
	})

	t.Run("WorkspaceAgent", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
		require.Equal(t, exp, must(db.GetWorkspaceAgentByID(context.Background(), exp.ID)))
	})

	t.Run("Template", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.Template(t, db, database.Template{})
		require.Equal(t, exp, must(db.GetTemplateByID(context.Background(), exp.ID)))
	})

	t.Run("TemplateVersion", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.TemplateVersion(t, db, database.TemplateVersion{})
		require.Equal(t, exp, must(db.GetTemplateVersionByID(context.Background(), exp.ID)))
	})

	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
		require.Equal(t, exp, must(db.GetWorkspaceBuildByID(context.Background(), exp.ID)))
	})

	t.Run("User", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.User(t, db, database.User{})
		require.Equal(t, exp, must(db.GetUserByID(context.Background(), exp.ID)))
	})

	t.Run("SSHKey", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
		require.Equal(t, exp, must(db.GetGitSSHKey(context.Background(), exp.UserID)))
	})

	t.Run("WorkspaceBuildParameters", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{{Name: "name1", Value: "value1"}, {Name: "name2", Value: "value2"}, {Name: "name3", Value: "value3"}})
		require.Equal(t, exp, must(db.GetWorkspaceBuildParameters(context.Background(), exp[0].WorkspaceBuildID)))
	})

	t.Run("TemplateVersionParameter", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{})
		actual := must(db.GetTemplateVersionParameters(context.Background(), exp.TemplateVersionID))
		require.Len(t, actual, 1)
		require.Equal(t, exp, actual[0])
	})
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
