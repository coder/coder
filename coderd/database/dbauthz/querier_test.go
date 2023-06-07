package dbauthz_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/slice"
)

func (s *MethodTestSuite) TestAPIKey() {
	s.Run("DeleteAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(key.ID).Asserts(key, rbac.ActionDelete).Returns()
	}))
	s.Run("GetAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(key.ID).Asserts(key, rbac.ActionRead).Returns(key)
	}))
	s.Run("GetAPIKeyByName", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			TokenName: "marge-cat",
			LoginType: database.LoginTypeToken,
		})
		check.Args(database.GetAPIKeyByNameParams{
			TokenName: key.TokenName,
			UserID:    key.UserID,
		}).Asserts(key, rbac.ActionRead).Returns(key)
	}))
	s.Run("GetAPIKeysByLoginType", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypePassword})
		b, _ := dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypePassword})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypeGithub})
		check.Args(database.LoginTypePassword).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("GetAPIKeysByUserID", s.Subtest(func(db database.Store, check *expects) {
		idAB := uuid.New()
		idC := uuid.New()

		keyA, _ := dbgen.APIKey(s.T(), db, database.APIKey{UserID: idAB, LoginType: database.LoginTypeToken})
		keyB, _ := dbgen.APIKey(s.T(), db, database.APIKey{UserID: idAB, LoginType: database.LoginTypeToken})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{UserID: idC, LoginType: database.LoginTypeToken})

		check.Args(database.GetAPIKeysByUserIDParams{LoginType: database.LoginTypeToken, UserID: idAB}).
			Asserts(keyA, rbac.ActionRead, keyB, rbac.ActionRead).
			Returns(slice.New(keyA, keyB))
	}))
	s.Run("GetAPIKeysLastUsedAfter", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
		b, _ := dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("InsertAPIKey", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertAPIKeyParams{
			UserID:    u.ID,
			LoginType: database.LoginTypePassword,
			Scope:     database.APIKeyScopeAll,
		}).Asserts(rbac.ResourceAPIKey.WithOwner(u.ID.String()), rbac.ActionCreate)
	}))
	s.Run("UpdateAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(database.UpdateAPIKeyByIDParams{
			ID: a.ID,
		}).Asserts(a, rbac.ActionUpdate).Returns()
	}))
}

func (s *MethodTestSuite) TestAuditLogs() {
	s.Run("InsertAuditLog", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertAuditLogParams{
			ResourceType: database.ResourceTypeOrganization,
			Action:       database.AuditActionCreate,
		}).Asserts(rbac.ResourceAuditLog, rbac.ActionCreate)
	}))
	s.Run("GetAuditLogsOffset", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		check.Args(database.GetAuditLogsOffsetParams{
			Limit: 10,
		}).Asserts(rbac.ResourceAuditLog, rbac.ActionRead)
	}))
}

func (s *MethodTestSuite) TestFile() {
	s.Run("GetFileByHashAndCreator", s.Subtest(func(db database.Store, check *expects) {
		f := dbgen.File(s.T(), db, database.File{})
		check.Args(database.GetFileByHashAndCreatorParams{
			Hash:      f.Hash,
			CreatedBy: f.CreatedBy,
		}).Asserts(f, rbac.ActionRead).Returns(f)
	}))
	s.Run("GetFileByID", s.Subtest(func(db database.Store, check *expects) {
		f := dbgen.File(s.T(), db, database.File{})
		check.Args(f.ID).Asserts(f, rbac.ActionRead).Returns(f)
	}))
	s.Run("InsertFile", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertFileParams{
			CreatedBy: u.ID,
		}).Asserts(rbac.ResourceFile.WithOwner(u.ID.String()), rbac.ActionCreate)
	}))
}

func (s *MethodTestSuite) TestGroup() {
	s.Run("DeleteGroupByID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(g.ID).Asserts(g, rbac.ActionDelete).Returns()
	}))
	s.Run("DeleteGroupMemberFromGroup", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		m := dbgen.GroupMember(s.T(), db, database.GroupMember{
			GroupID: g.ID,
		})
		check.Args(database.DeleteGroupMemberFromGroupParams{
			UserID:  m.UserID,
			GroupID: g.ID,
		}).Asserts(g, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetGroupByID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(g.ID).Asserts(g, rbac.ActionRead).Returns(g)
	}))
	s.Run("GetGroupByOrgAndName", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(database.GetGroupByOrgAndNameParams{
			OrganizationID: g.OrganizationID,
			Name:           g.Name,
		}).Asserts(g, rbac.ActionRead).Returns(g)
	}))
	s.Run("GetGroupMembers", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMember{})
		check.Args(g.ID).Asserts(g, rbac.ActionRead)
	}))
	s.Run("InsertAllUsersGroup", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.ID).Asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionCreate)
	}))
	s.Run("InsertGroup", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(database.InsertGroupParams{
			OrganizationID: o.ID,
			Name:           "test",
		}).Asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionCreate)
	}))
	s.Run("InsertGroupMember", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(database.InsertGroupMemberParams{
			UserID:  uuid.New(),
			GroupID: g.ID,
		}).Asserts(g, rbac.ActionUpdate).Returns()
	}))
	s.Run("InsertUserGroupsByName", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u1 := dbgen.User(s.T(), db, database.User{})
		g1 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		g2 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMember{GroupID: g1.ID, UserID: u1.ID})
		check.Args(database.InsertUserGroupsByNameParams{
			OrganizationID: o.ID,
			UserID:         u1.ID,
			GroupNames:     slice.New(g1.Name, g2.Name),
		}).Asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionUpdate).Returns()
	}))
	s.Run("DeleteGroupMembersByOrgAndUser", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u1 := dbgen.User(s.T(), db, database.User{})
		g1 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		g2 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMember{GroupID: g1.ID, UserID: u1.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMember{GroupID: g2.ID, UserID: u1.ID})
		check.Args(database.DeleteGroupMembersByOrgAndUserParams{
			OrganizationID: o.ID,
			UserID:         u1.ID,
		}).Asserts(rbac.ResourceGroup.InOrg(o.ID), rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateGroupByID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(database.UpdateGroupByIDParams{
			ID: g.ID,
		}).Asserts(g, rbac.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestProvsionerJob() {
	s.Run("Build/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(j.ID).Asserts(w, rbac.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersion/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
		})
		check.Args(j.ID).Asserts(v.RBACObject(tpl), rbac.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersionDryRun/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(struct {
				TemplateVersionID uuid.UUID `json:"template_version_id"`
			}{TemplateVersionID: v.ID})),
		})
		check.Args(j.ID).Asserts(v.RBACObject(tpl), rbac.ActionRead).Returns(j)
	}))
	s.Run("Build/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{AllowUserCancelWorkspaceJobs: true})
		w := dbgen.Workspace(s.T(), db, database.Workspace{TemplateID: tpl.ID})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).Asserts(w, rbac.ActionUpdate).Returns()
	}))
	s.Run("BuildFalseCancel/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{AllowUserCancelWorkspaceJobs: false})
		w := dbgen.Workspace(s.T(), db, database.Workspace{TemplateID: tpl.ID})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).Asserts(w, rbac.ActionUpdate).Returns()
	}))
	s.Run("TemplateVersion/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
	}))
	s.Run("TemplateVersionNoTemplate/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: uuid.Nil, Valid: false},
			JobID:      j.ID,
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObjectNoTemplate(), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
	}))
	s.Run("TemplateVersionDryRun/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(struct {
				TemplateVersionID uuid.UUID `json:"template_version_id"`
			}{TemplateVersionID: v.ID})),
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
	}))
	s.Run("GetProvisionerJobsByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		b := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{})
		check.Args([]uuid.UUID{a.ID, b.ID}).Asserts().Returns(slice.New(a, b))
	}))
	s.Run("GetProvisionerLogsAfterID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		j := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.GetProvisionerLogsAfterIDParams{
			JobID: j.ID,
		}).Asserts(w, rbac.ActionRead).Returns([]database.ProvisionerJobLog{})
	}))
}

func (s *MethodTestSuite) TestLicense() {
	s.Run("GetLicenses", s.Subtest(func(db database.Store, check *expects) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			UUID: uuid.New(),
		})
		require.NoError(s.T(), err)
		check.Args().Asserts(l, rbac.ActionRead).
			Returns([]database.License{l})
	}))
	s.Run("InsertLicense", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertLicenseParams{}).
			Asserts(rbac.ResourceLicense, rbac.ActionCreate)
	}))
	s.Run("UpsertLogoURL", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceDeploymentValues, rbac.ActionCreate)
	}))
	s.Run("UpsertServiceBanner", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceDeploymentValues, rbac.ActionCreate)
	}))
	s.Run("GetLicenseByID", s.Subtest(func(db database.Store, check *expects) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			UUID: uuid.New(),
		})
		require.NoError(s.T(), err)
		check.Args(l.ID).Asserts(l, rbac.ActionRead).Returns(l)
	}))
	s.Run("DeleteLicense", s.Subtest(func(db database.Store, check *expects) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			UUID: uuid.New(),
		})
		require.NoError(s.T(), err)
		check.Args(l.ID).Asserts(l, rbac.ActionDelete)
	}))
	s.Run("GetDeploymentID", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts().Returns("")
	}))
	s.Run("GetLogoURL", s.Subtest(func(db database.Store, check *expects) {
		err := db.UpsertLogoURL(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts().Returns("value")
	}))
	s.Run("GetServiceBanner", s.Subtest(func(db database.Store, check *expects) {
		err := db.UpsertServiceBanner(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts().Returns("value")
	}))
}

func (s *MethodTestSuite) TestOrganization() {
	s.Run("GetGroupsByOrganizationID", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		a := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		b := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		check.Args(o.ID).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns([]database.Group{a, b})
	}))
	s.Run("GetOrganizationByID", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.ID).Asserts(o, rbac.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationByName", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.Name).Asserts(o, rbac.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationIDsByMemberIDs", s.Subtest(func(db database.Store, check *expects) {
		oa := dbgen.Organization(s.T(), db, database.Organization{})
		ob := dbgen.Organization(s.T(), db, database.Organization{})
		ma := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{OrganizationID: oa.ID})
		mb := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{OrganizationID: ob.ID})
		check.Args([]uuid.UUID{ma.UserID, mb.UserID}).
			Asserts(rbac.ResourceUser.WithID(ma.UserID), rbac.ActionRead, rbac.ResourceUser.WithID(mb.UserID), rbac.ActionRead)
	}))
	s.Run("GetOrganizationMemberByUserID", s.Subtest(func(db database.Store, check *expects) {
		mem := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{})
		check.Args(database.GetOrganizationMemberByUserIDParams{
			OrganizationID: mem.OrganizationID,
			UserID:         mem.UserID,
		}).Asserts(mem, rbac.ActionRead).Returns(mem)
	}))
	s.Run("GetOrganizationMembershipsByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		a := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID})
		b := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID})
		check.Args(u.ID).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetOrganizations", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.Organization(s.T(), db, database.Organization{})
		b := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args().Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetOrganizationsByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		a := dbgen.Organization(s.T(), db, database.Organization{})
		_ = dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID, OrganizationID: a.ID})
		b := dbgen.Organization(s.T(), db, database.Organization{})
		_ = dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID, OrganizationID: b.ID})
		check.Args(u.ID).Asserts(a, rbac.ActionRead, b, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("InsertOrganization", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertOrganizationParams{
			ID:   uuid.New(),
			Name: "random",
		}).Asserts(rbac.ResourceOrganization, rbac.ActionCreate)
	}))
	s.Run("InsertOrganizationMember", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})

		check.Args(database.InsertOrganizationMemberParams{
			OrganizationID: o.ID,
			UserID:         u.ID,
			Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
		}).Asserts(
			rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionCreate,
			rbac.ResourceOrganizationMember.InOrg(o.ID).WithID(u.ID), rbac.ActionCreate)
	}))
	s.Run("UpdateMemberRoles", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})
		mem := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{
			OrganizationID: o.ID,
			UserID:         u.ID,
			Roles:          []string{rbac.RoleOrgAdmin(o.ID)},
		})
		out := mem
		out.Roles = []string{}

		check.Args(database.UpdateMemberRolesParams{
			GrantedRoles: []string{},
			UserID:       u.ID,
			OrgID:        o.ID,
		}).Asserts(
			mem, rbac.ActionRead,
			rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionCreate, // org-mem
			rbac.ResourceRoleAssignment.InOrg(o.ID), rbac.ActionDelete, // org-admin
		).Returns(out)
	}))
}

func (s *MethodTestSuite) TestWorkspaceProxy() {
	s.Run("InsertWorkspaceProxy", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceProxyParams{
			ID: uuid.New(),
		}).Asserts(rbac.ResourceWorkspaceProxy, rbac.ActionCreate)
	}))
	s.Run("RegisterWorkspaceProxy", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(database.RegisterWorkspaceProxyParams{
			ID: p.ID,
		}).Asserts(p, rbac.ActionUpdate)
	}))
	s.Run("GetWorkspaceProxyByID", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(p.ID).Asserts(p, rbac.ActionRead).Returns(p)
	}))
	s.Run("UpdateWorkspaceProxyDeleted", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(database.UpdateWorkspaceProxyDeletedParams{
			ID:      p.ID,
			Deleted: true,
		}).Asserts(p, rbac.ActionDelete)
	}))
	s.Run("GetWorkspaceProxies", s.Subtest(func(db database.Store, check *expects) {
		p1, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		p2, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args().Asserts(p1, rbac.ActionRead, p2, rbac.ActionRead).Returns(slice.New(p1, p2))
	}))
}

func (s *MethodTestSuite) TestTemplate() {
	s.Run("GetPreviousTemplateVersion", s.Subtest(func(db database.Store, check *expects) {
		tvid := uuid.New()
		now := time.Now()
		o1 := dbgen.Organization(s.T(), db, database.Organization{})
		t1 := dbgen.Template(s.T(), db, database.Template{
			OrganizationID:  o1.ID,
			ActiveVersionID: tvid,
		})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			CreatedAt:      now.Add(-time.Hour),
			ID:             tvid,
			Name:           t1.Name,
			OrganizationID: o1.ID,
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		b := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			CreatedAt:      now.Add(-2 * time.Hour),
			Name:           t1.Name,
			OrganizationID: o1.ID,
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.GetPreviousTemplateVersionParams{
			Name:           t1.Name,
			OrganizationID: o1.ID,
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
		}).Asserts(t1, rbac.ActionRead).Returns(b)
	}))
	s.Run("GetTemplateByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateByOrganizationAndName", s.Subtest(func(db database.Store, check *expects) {
		o1 := dbgen.Organization(s.T(), db, database.Organization{})
		t1 := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: o1.ID,
		})
		check.Args(database.GetTemplateByOrganizationAndNameParams{
			Name:           t1.Name,
			OrganizationID: o1.ID,
		}).Asserts(t1, rbac.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateVersionByJobID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.JobID).Asserts(t1, rbac.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionByTemplateIDAndName", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.GetTemplateVersionByTemplateIDAndNameParams{
			Name:       tv.Name,
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		}).Asserts(t1, rbac.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionParameters", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.ID).Asserts(t1, rbac.ActionRead).Returns([]database.TemplateVersionParameter{})
	}))
	s.Run("GetTemplateVersionVariables", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		tvv1 := dbgen.TemplateVersionVariable(s.T(), db, database.TemplateVersionVariable{
			TemplateVersionID: tv.ID,
		})
		check.Args(tv.ID).Asserts(t1, rbac.ActionRead).Returns([]database.TemplateVersionVariable{tvv1})
	}))
	s.Run("GetTemplateGroupRoles", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionRead)
	}))
	s.Run("GetTemplateUserRoles", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionRead)
	}))
	s.Run("GetTemplateVersionByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.ID).Asserts(t1, rbac.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionsByTemplateID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		a := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		b := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.GetTemplateVersionsByTemplateIDParams{
			TemplateID: t1.ID,
		}).Asserts(t1, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("GetTemplateVersionsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		now := time.Now()
		t1 := dbgen.Template(s.T(), db, database.Template{})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			CreatedAt:  now.Add(-time.Hour),
		})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			CreatedAt:  now.Add(-2 * time.Hour),
		})
		check.Args(now.Add(-time.Hour)).Asserts(rbac.ResourceTemplate.All(), rbac.ActionRead)
	}))
	s.Run("GetTemplatesWithFilter", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.Template(s.T(), db, database.Template{})
		// No asserts because SQLFilter.
		check.Args(database.GetTemplatesWithFilterParams{}).
			Asserts().Returns(slice.New(a))
	}))
	s.Run("GetAuthorizedTemplates", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.Template(s.T(), db, database.Template{})
		// No asserts because SQLFilter.
		check.Args(database.GetTemplatesWithFilterParams{}, emptyPreparedAuthorized{}).
			Asserts().
			Returns(slice.New(a))
	}))
	s.Run("InsertTemplate", s.Subtest(func(db database.Store, check *expects) {
		orgID := uuid.New()
		check.Args(database.InsertTemplateParams{
			Provisioner:    "echo",
			OrganizationID: orgID,
		}).Asserts(rbac.ResourceTemplate.InOrg(orgID), rbac.ActionCreate)
	}))
	s.Run("InsertTemplateVersion", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.InsertTemplateVersionParams{
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
			OrganizationID: t1.OrganizationID,
		}).Asserts(t1, rbac.ActionRead, t1, rbac.ActionCreate)
	}))
	s.Run("SoftDeleteTemplateByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionDelete)
	}))
	s.Run("UpdateTemplateACLByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateACLByIDParams{
			ID: t1.ID,
		}).Asserts(t1, rbac.ActionCreate).Returns(t1)
	}))
	s.Run("UpdateTemplateActiveVersionByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{
			ActiveVersionID: uuid.New(),
		})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			ID:         t1.ActiveVersionID,
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.UpdateTemplateActiveVersionByIDParams{
			ID:              t1.ID,
			ActiveVersionID: tv.ID,
		}).Asserts(t1, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateDeletedByIDParams{
			ID:      t1.ID,
			Deleted: true,
		}).Asserts(t1, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateTemplateMetaByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateMetaByIDParams{
			ID: t1.ID,
		}).Asserts(t1, rbac.ActionUpdate)
	}))
	s.Run("UpdateTemplateVersionByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.UpdateTemplateVersionByIDParams{
			ID:         tv.ID,
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			Name:       tv.Name,
			UpdatedAt:  tv.UpdatedAt,
		}).Asserts(t1, rbac.ActionUpdate).Returns(tv)
	}))
	s.Run("UpdateTemplateVersionDescriptionByJobID", s.Subtest(func(db database.Store, check *expects) {
		jobID := uuid.New()
		t1 := dbgen.Template(s.T(), db, database.Template{})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			JobID:      jobID,
		})
		check.Args(database.UpdateTemplateVersionDescriptionByJobIDParams{
			JobID:  jobID,
			Readme: "foo",
		}).Asserts(t1, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateVersionGitAuthProvidersByJobID", s.Subtest(func(db database.Store, check *expects) {
		jobID := uuid.New()
		t1 := dbgen.Template(s.T(), db, database.Template{})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			JobID:      jobID,
		})
		check.Args(database.UpdateTemplateVersionGitAuthProvidersByJobIDParams{
			JobID:            jobID,
			GitAuthProviders: []string{},
		}).Asserts(t1, rbac.ActionUpdate).Returns()
	}))
}

func (s *MethodTestSuite) TestUser() {
	s.Run("DeleteAPIKeysByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(rbac.ResourceAPIKey.WithOwner(u.ID.String()), rbac.ActionDelete).Returns()
	}))
	s.Run("GetQuotaAllowanceForUser", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(int64(0))
	}))
	s.Run("GetQuotaConsumedForUser", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(int64(0))
	}))
	s.Run("GetUserByEmailOrUsername", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetUserByEmailOrUsernameParams{
			Username: u.Username,
			Email:    u.Email,
		}).Asserts(u, rbac.ActionRead).Returns(u)
	}))
	s.Run("GetUserByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionRead).Returns(u)
	}))
	s.Run("GetUsersByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), db, database.User{CreatedAt: database.Now()})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("GetAuthorizedUserCount", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetFilteredUserCountParams{}, emptyPreparedAuthorized{}).Asserts().Returns(int64(1))
	}))
	s.Run("GetFilteredUserCount", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetFilteredUserCountParams{}).Asserts().Returns(int64(1))
	}))
	s.Run("GetUsers", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{Username: "GetUsers-a-user"})
		b := dbgen.User(s.T(), db, database.User{Username: "GetUsers-b-user"})
		check.Args(database.GetUsersParams{}).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead)
	}))
	s.Run("GetUsersWithCount", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{Username: "GetUsersWithCount-a-user"})
		b := dbgen.User(s.T(), db, database.User{Username: "GetUsersWithCount-b-user"})
		check.Args(database.GetUsersParams{}).Asserts(a, rbac.ActionRead, b, rbac.ActionRead)
	}))
	s.Run("InsertUser", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertUserParams{
			ID:        uuid.New(),
			LoginType: database.LoginTypePassword,
		}).Asserts(rbac.ResourceRoleAssignment, rbac.ActionCreate, rbac.ResourceUser, rbac.ActionCreate)
	}))
	s.Run("InsertUserLink", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertUserLinkParams{
			UserID:    u.ID,
			LoginType: database.LoginTypeOIDC,
		}).Asserts(u, rbac.ActionUpdate)
	}))
	s.Run("SoftDeleteUserByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateUserDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{Deleted: true})
		check.Args(database.UpdateUserDeletedByIDParams{
			ID:      u.ID,
			Deleted: true,
		}).Asserts(u, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateUserHashedPassword", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserHashedPasswordParams{
			ID: u.ID,
		}).Asserts(u.UserDataRBACObject(), rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateUserLastSeenAt", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserLastSeenAtParams{
			ID:         u.ID,
			UpdatedAt:  u.UpdatedAt,
			LastSeenAt: u.LastSeenAt,
		}).Asserts(u, rbac.ActionUpdate).Returns(u)
	}))
	s.Run("UpdateUserProfile", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserProfileParams{
			ID:        u.ID,
			Email:     u.Email,
			Username:  u.Username,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u.UserDataRBACObject(), rbac.ActionUpdate).Returns(u)
	}))
	s.Run("UpdateUserStatus", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserStatusParams{
			ID:        u.ID,
			Status:    u.Status,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u, rbac.ActionUpdate).Returns(u)
	}))
	s.Run("DeleteGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(key.UserID).Asserts(key, rbac.ActionDelete).Returns()
	}))
	s.Run("GetGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(key.UserID).Asserts(key, rbac.ActionRead).Returns(key)
	}))
	s.Run("InsertGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertGitSSHKeyParams{
			UserID: u.ID,
		}).Asserts(rbac.ResourceUserData.WithID(u.ID).WithOwner(u.ID.String()), rbac.ActionCreate)
	}))
	s.Run("UpdateGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(database.UpdateGitSSHKeyParams{
			UserID:    key.UserID,
			UpdatedAt: key.UpdatedAt,
		}).Asserts(key, rbac.ActionUpdate).Returns(key)
	}))
	s.Run("GetGitAuthLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.GitAuthLink(s.T(), db, database.GitAuthLink{})
		check.Args(database.GetGitAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Asserts(link, rbac.ActionRead).Returns(link)
	}))
	s.Run("InsertGitAuthLink", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertGitAuthLinkParams{
			ProviderID: uuid.NewString(),
			UserID:     u.ID,
		}).Asserts(rbac.ResourceUserData.WithOwner(u.ID.String()).WithID(u.ID), rbac.ActionCreate)
	}))
	s.Run("UpdateGitAuthLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.GitAuthLink(s.T(), db, database.GitAuthLink{})
		check.Args(database.UpdateGitAuthLinkParams{
			ProviderID:        link.ProviderID,
			UserID:            link.UserID,
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
			UpdatedAt:         link.UpdatedAt,
		}).Asserts(link, rbac.ActionUpdate).Returns(link)
	}))
	s.Run("UpdateUserLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(database.UpdateUserLinkParams{
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
			UserID:            link.UserID,
			LoginType:         link.LoginType,
		}).Asserts(link, rbac.ActionUpdate).Returns(link)
	}))
	s.Run("UpdateUserRoles", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{RBACRoles: []string{rbac.RoleTemplateAdmin()}})
		o := u
		o.RBACRoles = []string{rbac.RoleUserAdmin()}
		check.Args(database.UpdateUserRolesParams{
			GrantedRoles: []string{rbac.RoleUserAdmin()},
			ID:           u.ID,
		}).Asserts(
			u, rbac.ActionRead,
			rbac.ResourceRoleAssignment, rbac.ActionCreate,
			rbac.ResourceRoleAssignment, rbac.ActionDelete,
		).Returns(o)
	}))
}

func (s *MethodTestSuite) TestWorkspace() {
	s.Run("GetWorkspaceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(ws.ID).Asserts(ws, rbac.ActionRead)
	}))
	s.Run("GetWorkspaces", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		// No asserts here because SQLFilter.
		check.Args(database.GetWorkspacesParams{}).Asserts()
	}))
	s.Run("GetAuthorizedWorkspaces", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		_ = dbgen.Workspace(s.T(), db, database.Workspace{})
		// No asserts here because SQLFilter.
		check.Args(database.GetWorkspacesParams{}, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("GetLatestWorkspaceBuildByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(ws.ID).Asserts(ws, rbac.ActionRead).Returns(b)
	}))
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args([]uuid.UUID{ws.ID}).Asserts(ws, rbac.ActionRead).Returns(slice.New(b))
	}))
	s.Run("GetWorkspaceAgentByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(agt)
	}))
	s.Run("GetWorkspaceAgentByInstanceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.AuthInstanceID.String).Asserts(ws, rbac.ActionRead).Returns(agt)
	}))
	s.Run("UpdateWorkspaceAgentLifecycleStateByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agt.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentStartupLogOverflowByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentStartupLogOverflowByIDParams{
			ID:                    agt.ID,
			StartupLogsOverflowed: true,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentStartupByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentStartupByIDParams{
			ID:        agt.ID,
			Subsystem: database.WorkspaceAgentSubsystemNone,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceAgentStartupLogsAfter", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.GetWorkspaceAgentStartupLogsAfterParams{
			AgentID: agt.ID,
		}).Asserts(ws, rbac.ActionRead).Returns([]database.WorkspaceAgentStartupLog{})
	}))
	s.Run("GetWorkspaceAppByAgentIDAndSlug", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})

		check.Args(database.GetWorkspaceAppByAgentIDAndSlugParams{
			AgentID: agt.ID,
			Slug:    app.Slug,
		}).Asserts(ws, rbac.ActionRead).Returns(app)
	}))
	s.Run("GetWorkspaceAppsByAgentID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		a := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		b := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})

		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetWorkspaceBuildByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.ID).Asserts(ws, rbac.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByJobID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.JobID).Asserts(ws, rbac.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByWorkspaceIDAndBuildNumber", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 10})
		check.Args(database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
			WorkspaceID: ws.ID,
			BuildNumber: build.BuildNumber,
		}).Asserts(ws, rbac.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.ID).Asserts(ws, rbac.ActionRead).
			Returns([]database.WorkspaceBuildParameter{})
	}))
	s.Run("GetWorkspaceBuildsByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 1})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 2})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 3})
		check.Args(database.GetWorkspaceBuildsByWorkspaceIDParams{WorkspaceID: ws.ID}).Asserts(ws, rbac.ActionRead) // ordering
	}))
	s.Run("GetWorkspaceByAgentID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaceByOwnerIDAndName", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: ws.OwnerID,
			Deleted: ws.Deleted,
			Name:    ws.Name,
		}).Asserts(ws, rbac.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaceResourceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args(res.ID).Asserts(ws, rbac.ActionRead).Returns(res)
	}))
	s.Run("Build/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args(job.ID).Asserts(ws, rbac.ActionRead).Returns([]database.WorkspaceResource{})
	}))
	s.Run("Template/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})
		check.Args(job.ID).Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionRead}).Returns([]database.WorkspaceResource{})
	}))
	s.Run("InsertWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(database.InsertWorkspaceParams{
			ID:             uuid.New(),
			OwnerID:        u.ID,
			OrganizationID: o.ID,
		}).Asserts(rbac.ResourceWorkspace.WithOwner(u.ID.String()).InOrg(o.ID), rbac.ActionCreate)
	}))
	s.Run("Start/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, rbac.ActionUpdate)
	}))
	s.Run("Delete/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionDelete,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, rbac.ActionDelete)
	}))
	s.Run("InsertWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: w.ID})
		check.Args(database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: b.ID,
			Name:             []string{"foo", "bar"},
			Value:            []string{"baz", "qux"},
		}).Asserts(w, rbac.ActionUpdate)
	}))
	s.Run("UpdateWorkspace", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		expected := w
		expected.Name = ""
		check.Args(database.UpdateWorkspaceParams{
			ID: w.ID,
		}).Asserts(w, rbac.ActionUpdate).Returns(expected)
	}))
	s.Run("InsertWorkspaceAgentStat", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertWorkspaceAgentStatParams{
			WorkspaceID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceAppHealthByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		check.Args(database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: database.WorkspaceAppHealthDisabled,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAutostart", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceAutostartParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceBuildByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		check.Args(database.UpdateWorkspaceBuildByIDParams{
			ID:               build.ID,
			UpdatedAt:        build.UpdatedAt,
			Deadline:         build.Deadline,
			ProvisionerState: []byte{},
		}).Asserts(ws, rbac.ActionUpdate).Returns(build)
	}))
	s.Run("SoftDeleteWorkspaceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		ws.Deleted = true
		check.Args(ws.ID).Asserts(ws, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{Deleted: true})
		check.Args(database.UpdateWorkspaceDeletedByIDParams{
			ID:      ws.ID,
			Deleted: true,
		}).Asserts(ws, rbac.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceLastUsedAt", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceLastUsedAtParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceTTL", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceTTLParams{
			ID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceByWorkspaceAppID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		check.Args(app.ID).Asserts(ws, rbac.ActionRead).Returns(ws)
	}))
}

func (s *MethodTestSuite) TestExtraMethods() {
	s.Run("GetProvisionerDaemons", s.Subtest(func(db database.Store, check *expects) {
		d, err := db.InsertProvisionerDaemon(context.Background(), database.InsertProvisionerDaemonParams{
			ID: uuid.New(),
		})
		s.NoError(err, "insert provisioner daemon")
		check.Args().Asserts(d, rbac.ActionRead)
	}))
}
