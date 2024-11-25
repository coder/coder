package dbauthz_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAsNoActor(t *testing.T) {
	t.Parallel()

	t.Run("NoError", func(t *testing.T) {
		t.Parallel()
		require.False(t, dbauthz.IsNotAuthorizedError(nil), "no error")
	})

	t.Run("AsRemoveActor", func(t *testing.T) {
		t.Parallel()
		_, ok := dbauthz.ActorFromContext(context.Background())
		require.False(t, ok, "no actor should be present")
	})

	t.Run("AsActor", func(t *testing.T) {
		t.Parallel()
		ctx := dbauthz.As(context.Background(), coderdtest.RandomRBACSubject())
		_, ok := dbauthz.ActorFromContext(ctx)
		require.True(t, ok, "actor present")
	})

	t.Run("DeleteActor", func(t *testing.T) {
		t.Parallel()
		// First set an actor
		ctx := dbauthz.As(context.Background(), coderdtest.RandomRBACSubject())
		_, ok := dbauthz.ActorFromContext(ctx)
		require.True(t, ok, "actor present")

		// Delete the actor
		ctx = dbauthz.As(ctx, dbauthz.AsRemoveActor)
		_, ok = dbauthz.ActorFromContext(ctx)
		require.False(t, ok, "actor should be deleted")
	})
}

func TestPing(t *testing.T) {
	t.Parallel()

	q := dbauthz.New(dbmem.New(), &coderdtest.RecordingAuthorizer{}, slog.Make(), coderdtest.AccessControlStorePointer())
	_, err := q.Ping(context.Background())
	require.NoError(t, err, "must not error")
}

// TestInTX is not perfect, just checks that it properly checks auth.
func TestInTX(t *testing.T) {
	t.Parallel()

	db := dbmem.New()
	q := dbauthz.New(db, &coderdtest.RecordingAuthorizer{
		Wrapped: (&coderdtest.FakeAuthorizer{}).AlwaysReturn(xerrors.New("custom error")),
	}, slog.Make(), coderdtest.AccessControlStorePointer())
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleIdentifiers{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}

	w := dbgen.Workspace(t, db, database.WorkspaceTable{})
	ctx := dbauthz.As(context.Background(), actor)
	err := q.InTx(func(tx database.Store) error {
		// The inner tx should use the parent's authz
		_, err := tx.GetWorkspaceByID(ctx, w.ID)
		return err
	}, nil)
	require.Error(t, err, "must error")
	require.ErrorAs(t, err, &dbauthz.NotAuthorizedError{}, "must be an authorized error")
	require.True(t, dbauthz.IsNotAuthorizedError(err), "must be an authorized error")
}

// TestNew should not double wrap a querier.
func TestNew(t *testing.T) {
	t.Parallel()

	var (
		db  = dbmem.New()
		exp = dbgen.Workspace(t, db, database.WorkspaceTable{})
		rec = &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		subj = rbac.Subject{}
		ctx  = dbauthz.As(context.Background(), rbac.Subject{})
	)

	// Double wrap should not cause an actual double wrap. So only 1 rbac call
	// should be made.
	az := dbauthz.New(db, rec, slog.Make(), coderdtest.AccessControlStorePointer())
	az = dbauthz.New(az, rec, slog.Make(), coderdtest.AccessControlStorePointer())

	w, err := az.GetWorkspaceByID(ctx, exp.ID)
	require.NoError(t, err, "must not error")
	require.Equal(t, exp, w.WorkspaceTable(), "must be equal")

	rec.AssertActor(t, subj, rec.Pair(policy.ActionRead, exp))
	require.NoError(t, rec.AllAsserted(), "should only be 1 rbac call")
}

// TestDBAuthzRecursive is a simple test to search for infinite recursion
// bugs. It isn't perfect, and only catches a subset of the possible bugs
// as only the first db call will be made. But it is better than nothing.
func TestDBAuthzRecursive(t *testing.T) {
	t.Parallel()
	q := dbauthz.New(dbmem.New(), &coderdtest.RecordingAuthorizer{
		Wrapped: &coderdtest.FakeAuthorizer{},
	}, slog.Make(), coderdtest.AccessControlStorePointer())
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleIdentifiers{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}
	for i := 0; i < reflect.TypeOf(q).NumMethod(); i++ {
		var ins []reflect.Value
		ctx := dbauthz.As(context.Background(), actor)

		ins = append(ins, reflect.ValueOf(ctx))
		method := reflect.TypeOf(q).Method(i)
		for i := 2; i < method.Type.NumIn(); i++ {
			ins = append(ins, reflect.New(method.Type.In(i)).Elem())
		}
		if method.Name == "InTx" ||
			method.Name == "Ping" ||
			method.Name == "Wrappers" ||
			method.Name == "PGLocks" {
			continue
		}
		// Log the name of the last method, so if there is a panic, it is
		// easy to know which method failed.
		// t.Log(method.Name)
		// Call the function. Any infinite recursion will stack overflow.
		reflect.ValueOf(q).Method(i).Call(ins)
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func (s *MethodTestSuite) TestAPIKey() {
	s.Run("DeleteAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(key.ID).Asserts(key, policy.ActionDelete).Returns()
	}))
	s.Run("GetAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(key.ID).Asserts(key, policy.ActionRead).Returns(key)
	}))
	s.Run("GetAPIKeyByName", s.Subtest(func(db database.Store, check *expects) {
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			TokenName: "marge-cat",
			LoginType: database.LoginTypeToken,
		})
		check.Args(database.GetAPIKeyByNameParams{
			TokenName: key.TokenName,
			UserID:    key.UserID,
		}).Asserts(key, policy.ActionRead).Returns(key)
	}))
	s.Run("GetAPIKeysByLoginType", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypePassword})
		b, _ := dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypePassword})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{LoginType: database.LoginTypeGithub})
		check.Args(database.LoginTypePassword).
			Asserts(a, policy.ActionRead, b, policy.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("GetAPIKeysByUserID", s.Subtest(func(db database.Store, check *expects) {
		idAB := uuid.New()
		idC := uuid.New()

		keyA, _ := dbgen.APIKey(s.T(), db, database.APIKey{UserID: idAB, LoginType: database.LoginTypeToken})
		keyB, _ := dbgen.APIKey(s.T(), db, database.APIKey{UserID: idAB, LoginType: database.LoginTypeToken})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{UserID: idC, LoginType: database.LoginTypeToken})

		check.Args(database.GetAPIKeysByUserIDParams{LoginType: database.LoginTypeToken, UserID: idAB}).
			Asserts(keyA, policy.ActionRead, keyB, policy.ActionRead).
			Returns(slice.New(keyA, keyB))
	}))
	s.Run("GetAPIKeysLastUsedAfter", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
		b, _ := dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(time.Hour)})
		_, _ = dbgen.APIKey(s.T(), db, database.APIKey{LastUsed: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).
			Asserts(a, policy.ActionRead, b, policy.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("InsertAPIKey", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertAPIKeyParams{
			UserID:    u.ID,
			LoginType: database.LoginTypePassword,
			Scope:     database.APIKeyScopeAll,
		}).Asserts(rbac.ResourceApiKey.WithOwner(u.ID.String()), policy.ActionCreate)
	}))
	s.Run("UpdateAPIKeyByID", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{})
		check.Args(database.UpdateAPIKeyByIDParams{
			ID: a.ID,
		}).Asserts(a, policy.ActionUpdate).Returns()
	}))
	s.Run("DeleteApplicationConnectAPIKeysByUserID", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			Scope: database.APIKeyScopeApplicationConnect,
		})
		check.Args(a.UserID).Asserts(rbac.ResourceApiKey.WithOwner(a.UserID.String()), policy.ActionDelete).Returns()
	}))
	s.Run("DeleteExternalAuthLink", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		check.Args(database.DeleteExternalAuthLinkParams{
			ProviderID: a.ProviderID,
			UserID:     a.UserID,
		}).Asserts(rbac.ResourceUserObject(a.UserID), policy.ActionUpdatePersonal).Returns()
	}))
	s.Run("GetExternalAuthLinksByUserID", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		b := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{
			UserID: a.UserID,
		})
		check.Args(a.UserID).Asserts(
			rbac.ResourceUserObject(a.UserID), policy.ActionReadPersonal,
			rbac.ResourceUserObject(b.UserID), policy.ActionReadPersonal)
	}))
}

func (s *MethodTestSuite) TestAuditLogs() {
	s.Run("InsertAuditLog", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertAuditLogParams{
			ResourceType: database.ResourceTypeOrganization,
			Action:       database.AuditActionCreate,
		}).Asserts(rbac.ResourceAuditLog, policy.ActionCreate)
	}))
	s.Run("GetAuditLogsOffset", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		check.Args(database.GetAuditLogsOffsetParams{
			LimitOpt: 10,
		}).Asserts(rbac.ResourceAuditLog, policy.ActionRead)
	}))
	s.Run("GetAuthorizedAuditLogsOffset", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		_ = dbgen.AuditLog(s.T(), db, database.AuditLog{})
		check.Args(database.GetAuditLogsOffsetParams{
			LimitOpt: 10,
		}, emptyPreparedAuthorized{}).Asserts(rbac.ResourceAuditLog, policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestFile() {
	s.Run("GetFileByHashAndCreator", s.Subtest(func(db database.Store, check *expects) {
		f := dbgen.File(s.T(), db, database.File{})
		check.Args(database.GetFileByHashAndCreatorParams{
			Hash:      f.Hash,
			CreatedBy: f.CreatedBy,
		}).Asserts(f, policy.ActionRead).Returns(f)
	}))
	s.Run("GetFileByID", s.Subtest(func(db database.Store, check *expects) {
		f := dbgen.File(s.T(), db, database.File{})
		check.Args(f.ID).Asserts(f, policy.ActionRead).Returns(f)
	}))
	s.Run("InsertFile", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertFileParams{
			CreatedBy: u.ID,
		}).Asserts(rbac.ResourceFile.WithOwner(u.ID.String()), policy.ActionCreate)
	}))
}

func (s *MethodTestSuite) TestGroup() {
	s.Run("DeleteGroupByID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(g.ID).Asserts(g, policy.ActionDelete).Returns()
	}))
	s.Run("DeleteGroupMemberFromGroup", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		u := dbgen.User(s.T(), db, database.User{})
		m := dbgen.GroupMember(s.T(), db, database.GroupMemberTable{
			GroupID: g.ID,
			UserID:  u.ID,
		})
		check.Args(database.DeleteGroupMemberFromGroupParams{
			UserID:  m.UserID,
			GroupID: g.ID,
		}).Asserts(g, policy.ActionUpdate).Returns()
	}))
	s.Run("GetGroupByID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(g.ID).Asserts(g, policy.ActionRead).Returns(g)
	}))
	s.Run("GetGroupByOrgAndName", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(database.GetGroupByOrgAndNameParams{
			OrganizationID: g.OrganizationID,
			Name:           g.Name,
		}).Asserts(g, policy.ActionRead).Returns(g)
	}))
	s.Run("GetGroupMembersByGroupID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		u := dbgen.User(s.T(), db, database.User{})
		gm := dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g.ID, UserID: u.ID})
		check.Args(g.ID).Asserts(gm, policy.ActionRead)
	}))
	s.Run("GetGroupMembersCountByGroupID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(g.ID).Asserts(g, policy.ActionRead)
	}))
	s.Run("GetGroupMembers", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		u := dbgen.User(s.T(), db, database.User{})
		dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g.ID, UserID: u.ID})
		check.Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("System/GetGroups", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Group(s.T(), db, database.Group{})
		check.Args(database.GetGroupsParams{}).
			Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetGroups", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		u := dbgen.User(s.T(), db, database.User{})
		gm := dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g.ID, UserID: u.ID})
		check.Args(database.GetGroupsParams{
			OrganizationID: g.OrganizationID,
			HasMemberID:    gm.UserID,
		}).Asserts(rbac.ResourceSystem, policy.ActionRead, g, policy.ActionRead).
			// Fail the system resource skip
			FailSystemObjectChecks()
	}))
	s.Run("InsertAllUsersGroup", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.ID).Asserts(rbac.ResourceGroup.InOrg(o.ID), policy.ActionCreate)
	}))
	s.Run("InsertGroup", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(database.InsertGroupParams{
			OrganizationID: o.ID,
			Name:           "test",
		}).Asserts(rbac.ResourceGroup.InOrg(o.ID), policy.ActionCreate)
	}))
	s.Run("InsertGroupMember", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(database.InsertGroupMemberParams{
			UserID:  uuid.New(),
			GroupID: g.ID,
		}).Asserts(g, policy.ActionUpdate).Returns()
	}))
	s.Run("InsertUserGroupsByName", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u1 := dbgen.User(s.T(), db, database.User{})
		g1 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		g2 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g1.ID, UserID: u1.ID})
		check.Args(database.InsertUserGroupsByNameParams{
			OrganizationID: o.ID,
			UserID:         u1.ID,
			GroupNames:     slice.New(g1.Name, g2.Name),
		}).Asserts(rbac.ResourceGroup.InOrg(o.ID), policy.ActionUpdate).Returns()
	}))
	s.Run("InsertUserGroupsByID", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u1 := dbgen.User(s.T(), db, database.User{})
		g1 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		g2 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g1.ID, UserID: u1.ID})
		check.Args(database.InsertUserGroupsByIDParams{
			UserID:   u1.ID,
			GroupIds: slice.New(g1.ID, g2.ID),
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns(slice.New(g1.ID, g2.ID))
	}))
	s.Run("RemoveUserFromAllGroups", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u1 := dbgen.User(s.T(), db, database.User{})
		g1 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		g2 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g1.ID, UserID: u1.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g2.ID, UserID: u1.ID})
		check.Args(u1.ID).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))
	s.Run("RemoveUserFromGroups", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u1 := dbgen.User(s.T(), db, database.User{})
		g1 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		g2 := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g1.ID, UserID: u1.ID})
		_ = dbgen.GroupMember(s.T(), db, database.GroupMemberTable{GroupID: g2.ID, UserID: u1.ID})
		check.Args(database.RemoveUserFromGroupsParams{
			UserID:   u1.ID,
			GroupIds: []uuid.UUID{g1.ID, g2.ID},
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns(slice.New(g1.ID, g2.ID))
	}))
	s.Run("UpdateGroupByID", s.Subtest(func(db database.Store, check *expects) {
		g := dbgen.Group(s.T(), db, database.Group{})
		check.Args(database.UpdateGroupByIDParams{
			ID: g.ID,
		}).Asserts(g, policy.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestProvisionerJob() {
	s.Run("ArchiveUnusedTemplateVersions", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
			Error: sql.NullString{
				String: "failed",
				Valid:  true,
			},
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
		})
		check.Args(database.ArchiveUnusedTemplateVersionsParams{
			UpdatedAt:         dbtime.Now(),
			TemplateID:        tpl.ID,
			TemplateVersionID: uuid.Nil,
			JobStatus:         database.NullProvisionerJobStatus{},
		}).Asserts(v.RBACObject(tpl), policy.ActionUpdate)
	}))
	s.Run("UnarchiveTemplateVersion", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
			Archived:   true,
		})
		check.Args(database.UnarchiveTemplateVersionParams{
			UpdatedAt:         dbtime.Now(),
			TemplateVersionID: v.ID,
		}).Asserts(v.RBACObject(tpl), policy.ActionUpdate)
	}))
	s.Run("Build/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(j.ID).Asserts(w, policy.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersion/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
		})
		check.Args(j.ID).Asserts(v.RBACObject(tpl), policy.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersionDryRun/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(struct {
				TemplateVersionID uuid.UUID `json:"template_version_id"`
			}{TemplateVersionID: v.ID})),
		})
		check.Args(j.ID).Asserts(v.RBACObject(tpl), policy.ActionRead).Returns(j)
	}))
	s.Run("Build/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{AllowUserCancelWorkspaceJobs: true})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{TemplateID: tpl.ID})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("BuildFalseCancel/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{AllowUserCancelWorkspaceJobs: false})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{TemplateID: tpl.ID})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("TemplateVersion/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
			JobID:      j.ID,
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObject(tpl), []policy.Action{policy.ActionRead, policy.ActionUpdate}).Returns()
	}))
	s.Run("TemplateVersionNoTemplate/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionImport,
		})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: uuid.Nil, Valid: false},
			JobID:      j.ID,
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObjectNoTemplate(), []policy.Action{policy.ActionRead, policy.ActionUpdate}).Returns()
	}))
	s.Run("TemplateVersionDryRun/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeTemplateVersionDryRun,
			Input: must(json.Marshal(struct {
				TemplateVersionID uuid.UUID `json:"template_version_id"`
			}{TemplateVersionID: v.ID})),
		})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).
			Asserts(v.RBACObject(tpl), []policy.Action{policy.ActionRead, policy.ActionUpdate}).Returns()
	}))
	s.Run("GetProvisionerJobsByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		b := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args([]uuid.UUID{a.ID, b.ID}).Asserts().Returns(slice.New(a, b))
	}))
	s.Run("GetProvisionerLogsAfterID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.GetProvisionerLogsAfterIDParams{
			JobID: j.ID,
		}).Asserts(w, policy.ActionRead).Returns([]database.ProvisionerJobLog{})
	}))
}

func (s *MethodTestSuite) TestLicense() {
	s.Run("GetLicenses", s.Subtest(func(db database.Store, check *expects) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			UUID: uuid.New(),
		})
		require.NoError(s.T(), err)
		check.Args().Asserts(l, policy.ActionRead).
			Returns([]database.License{l})
	}))
	s.Run("InsertLicense", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertLicenseParams{}).
			Asserts(rbac.ResourceLicense, policy.ActionCreate)
	}))
	s.Run("UpsertLogoURL", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("UpsertAnnouncementBanners", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetLicenseByID", s.Subtest(func(db database.Store, check *expects) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			UUID: uuid.New(),
		})
		require.NoError(s.T(), err)
		check.Args(l.ID).Asserts(l, policy.ActionRead).Returns(l)
	}))
	s.Run("DeleteLicense", s.Subtest(func(db database.Store, check *expects) {
		l, err := db.InsertLicense(context.Background(), database.InsertLicenseParams{
			UUID: uuid.New(),
		})
		require.NoError(s.T(), err)
		check.Args(l.ID).Asserts(l, policy.ActionDelete)
	}))
	s.Run("GetDeploymentID", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts().Returns("")
	}))
	s.Run("GetDefaultProxyConfig", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts().Returns(database.GetDefaultProxyConfigRow{
			DisplayName: "Default",
			IconUrl:     "/emojis/1f3e1.png",
		})
	}))
	s.Run("GetLogoURL", s.Subtest(func(db database.Store, check *expects) {
		err := db.UpsertLogoURL(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts().Returns("value")
	}))
	s.Run("GetAnnouncementBanners", s.Subtest(func(db database.Store, check *expects) {
		err := db.UpsertAnnouncementBanners(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts().Returns("value")
	}))
}

func (s *MethodTestSuite) TestOrganization() {
	s.Run("Deployment/OIDCClaimFields", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.Nil).Asserts(rbac.ResourceIdpsyncSettings, policy.ActionRead).Returns([]string{})
	}))
	s.Run("Organization/OIDCClaimFields", s.Subtest(func(db database.Store, check *expects) {
		id := uuid.New()
		check.Args(id).Asserts(rbac.ResourceIdpsyncSettings.InOrg(id), policy.ActionRead).Returns([]string{})
	}))
	s.Run("Deployment/OIDCClaimFieldValues", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.OIDCClaimFieldValuesParams{
			ClaimField:     "claim-field",
			OrganizationID: uuid.Nil,
		}).Asserts(rbac.ResourceIdpsyncSettings, policy.ActionRead).Returns([]string{})
	}))
	s.Run("Organization/OIDCClaimFieldValues", s.Subtest(func(db database.Store, check *expects) {
		id := uuid.New()
		check.Args(database.OIDCClaimFieldValuesParams{
			ClaimField:     "claim-field",
			OrganizationID: id,
		}).Asserts(rbac.ResourceIdpsyncSettings.InOrg(id), policy.ActionRead).Returns([]string{})
	}))
	s.Run("ByOrganization/GetGroups", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		a := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		b := dbgen.Group(s.T(), db, database.Group{OrganizationID: o.ID})
		check.Args(database.GetGroupsParams{
			OrganizationID: o.ID,
		}).Asserts(rbac.ResourceSystem, policy.ActionRead, a, policy.ActionRead, b, policy.ActionRead).
			Returns([]database.GetGroupsRow{
				{Group: a, OrganizationName: o.Name, OrganizationDisplayName: o.DisplayName},
				{Group: b, OrganizationName: o.Name, OrganizationDisplayName: o.DisplayName},
			}).
			// Fail the system check shortcut
			FailSystemObjectChecks()
	}))
	s.Run("GetOrganizationByID", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.ID).Asserts(o, policy.ActionRead).Returns(o)
	}))
	s.Run("GetDefaultOrganization", s.Subtest(func(db database.Store, check *expects) {
		o, _ := db.GetDefaultOrganization(context.Background())
		check.Args().Asserts(o, policy.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationByName", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(o.Name).Asserts(o, policy.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationIDsByMemberIDs", s.Subtest(func(db database.Store, check *expects) {
		oa := dbgen.Organization(s.T(), db, database.Organization{})
		ob := dbgen.Organization(s.T(), db, database.Organization{})
		ma := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{OrganizationID: oa.ID})
		mb := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{OrganizationID: ob.ID})
		check.Args([]uuid.UUID{ma.UserID, mb.UserID}).
			Asserts(rbac.ResourceUserObject(ma.UserID), policy.ActionRead, rbac.ResourceUserObject(mb.UserID), policy.ActionRead)
	}))
	s.Run("GetOrganizations", s.Subtest(func(db database.Store, check *expects) {
		def, _ := db.GetDefaultOrganization(context.Background())
		a := dbgen.Organization(s.T(), db, database.Organization{})
		b := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(database.GetOrganizationsParams{}).Asserts(def, policy.ActionRead, a, policy.ActionRead, b, policy.ActionRead).Returns(slice.New(def, a, b))
	}))
	s.Run("GetOrganizationsByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		a := dbgen.Organization(s.T(), db, database.Organization{})
		_ = dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID, OrganizationID: a.ID})
		b := dbgen.Organization(s.T(), db, database.Organization{})
		_ = dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID, OrganizationID: b.ID})
		check.Args(u.ID).Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("InsertOrganization", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertOrganizationParams{
			ID:   uuid.New(),
			Name: "new-org",
		}).Asserts(rbac.ResourceOrganization, policy.ActionCreate)
	}))
	s.Run("InsertOrganizationMember", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})

		check.Args(database.InsertOrganizationMemberParams{
			OrganizationID: o.ID,
			UserID:         u.ID,
			Roles:          []string{codersdk.RoleOrganizationAdmin},
		}).Asserts(
			rbac.ResourceAssignOrgRole.InOrg(o.ID), policy.ActionAssign,
			rbac.ResourceOrganizationMember.InOrg(o.ID).WithID(u.ID), policy.ActionCreate)
	}))
	s.Run("DeleteOrganizationMember", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})
		member := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{UserID: u.ID, OrganizationID: o.ID})

		check.Args(database.DeleteOrganizationMemberParams{
			OrganizationID: o.ID,
			UserID:         u.ID,
		}).Asserts(
			// Reads the org member before it tries to delete it
			member, policy.ActionRead,
			member, policy.ActionDelete).
			// SQL Filter returns a 404
			WithNotAuthorized("no rows").
			WithCancelled("no rows").
			Errors(sql.ErrNoRows)
	}))
	s.Run("UpdateOrganization", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{
			Name: "something-unique",
		})
		check.Args(database.UpdateOrganizationParams{
			ID:   o.ID,
			Name: "something-different",
		}).Asserts(o, policy.ActionUpdate)
	}))
	s.Run("DeleteOrganization", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{
			Name: "doomed",
		})
		check.Args(
			o.ID,
		).Asserts(o, policy.ActionDelete)
	}))
	s.Run("OrganizationMembers", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})
		mem := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{
			OrganizationID: o.ID,
			UserID:         u.ID,
			Roles:          []string{rbac.RoleOrgAdmin()},
		})

		check.Args(database.OrganizationMembersParams{
			OrganizationID: uuid.UUID{},
			UserID:         uuid.UUID{},
		}).Asserts(
			mem, policy.ActionRead,
		)
	}))
	s.Run("UpdateMemberRoles", s.Subtest(func(db database.Store, check *expects) {
		o := dbgen.Organization(s.T(), db, database.Organization{})
		u := dbgen.User(s.T(), db, database.User{})
		mem := dbgen.OrganizationMember(s.T(), db, database.OrganizationMember{
			OrganizationID: o.ID,
			UserID:         u.ID,
			Roles:          []string{codersdk.RoleOrganizationAdmin},
		})
		out := mem
		out.Roles = []string{}

		check.Args(database.UpdateMemberRolesParams{
			GrantedRoles: []string{},
			UserID:       u.ID,
			OrgID:        o.ID,
		}).
			WithNotAuthorized(sql.ErrNoRows.Error()).
			WithCancelled(sql.ErrNoRows.Error()).
			Asserts(
				mem, policy.ActionRead,
				rbac.ResourceAssignOrgRole.InOrg(o.ID), policy.ActionAssign, // org-mem
				rbac.ResourceAssignOrgRole.InOrg(o.ID), policy.ActionDelete, // org-admin
			).Returns(out)
	}))
}

func (s *MethodTestSuite) TestWorkspaceProxy() {
	s.Run("InsertWorkspaceProxy", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceProxyParams{
			ID: uuid.New(),
		}).Asserts(rbac.ResourceWorkspaceProxy, policy.ActionCreate)
	}))
	s.Run("RegisterWorkspaceProxy", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(database.RegisterWorkspaceProxyParams{
			ID: p.ID,
		}).Asserts(p, policy.ActionUpdate)
	}))
	s.Run("GetWorkspaceProxyByID", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(p.ID).Asserts(p, policy.ActionRead).Returns(p)
	}))
	s.Run("GetWorkspaceProxyByName", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(p.Name).Asserts(p, policy.ActionRead).Returns(p)
	}))
	s.Run("UpdateWorkspaceProxyDeleted", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(database.UpdateWorkspaceProxyDeletedParams{
			ID:      p.ID,
			Deleted: true,
		}).Asserts(p, policy.ActionDelete)
	}))
	s.Run("UpdateWorkspaceProxy", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(database.UpdateWorkspaceProxyParams{
			ID: p.ID,
		}).Asserts(p, policy.ActionUpdate)
	}))
	s.Run("GetWorkspaceProxies", s.Subtest(func(db database.Store, check *expects) {
		p1, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		p2, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args().Asserts(p1, policy.ActionRead, p2, policy.ActionRead).Returns(slice.New(p1, p2))
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
		}).Asserts(t1, policy.ActionRead).Returns(b)
	}))
	s.Run("GetTemplateByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, policy.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateByOrganizationAndName", s.Subtest(func(db database.Store, check *expects) {
		o1 := dbgen.Organization(s.T(), db, database.Organization{})
		t1 := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: o1.ID,
		})
		check.Args(database.GetTemplateByOrganizationAndNameParams{
			Name:           t1.Name,
			OrganizationID: o1.ID,
		}).Asserts(t1, policy.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateVersionByJobID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.JobID).Asserts(t1, policy.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionByTemplateIDAndName", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(database.GetTemplateVersionByTemplateIDAndNameParams{
			Name:       tv.Name,
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		}).Asserts(t1, policy.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionParameters", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns([]database.TemplateVersionParameter{})
	}))
	s.Run("GetTemplateVersionVariables", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		tvv1 := dbgen.TemplateVersionVariable(s.T(), db, database.TemplateVersionVariable{
			TemplateVersionID: tv.ID,
		})
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns([]database.TemplateVersionVariable{tvv1})
	}))
	s.Run("GetTemplateVersionWorkspaceTags", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		wt1 := dbgen.TemplateVersionWorkspaceTag(s.T(), db, database.TemplateVersionWorkspaceTag{
			TemplateVersionID: tv.ID,
		})
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns([]database.TemplateVersionWorkspaceTag{wt1})
	}))
	s.Run("GetTemplateGroupRoles", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("GetTemplateUserRoles", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("GetTemplateVersionByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns(tv)
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
		}).Asserts(t1, policy.ActionRead).
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
		check.Args(now.Add(-time.Hour)).Asserts(rbac.ResourceTemplate.All(), policy.ActionRead)
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
			Provisioner:         "echo",
			OrganizationID:      orgID,
			MaxPortSharingLevel: database.AppSharingLevelOwner,
		}).Asserts(rbac.ResourceTemplate.InOrg(orgID), policy.ActionCreate)
	}))
	s.Run("InsertTemplateVersion", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.InsertTemplateVersionParams{
			TemplateID:     uuid.NullUUID{UUID: t1.ID, Valid: true},
			OrganizationID: t1.OrganizationID,
		}).Asserts(t1, policy.ActionRead, t1, policy.ActionCreate)
	}))
	s.Run("SoftDeleteTemplateByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, policy.ActionDelete)
	}))
	s.Run("UpdateTemplateACLByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateACLByIDParams{
			ID: t1.ID,
		}).Asserts(t1, policy.ActionCreate)
	}))
	s.Run("UpdateTemplateAccessControlByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateAccessControlByIDParams{
			ID: t1.ID,
		}).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateScheduleByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateScheduleByIDParams{
			ID: t1.ID,
		}).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateWorkspacesLastUsedAt", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateWorkspacesLastUsedAtParams{
			TemplateID: t1.ID,
		}).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspacesDormantDeletingAtByTemplateID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateWorkspacesDormantDeletingAtByTemplateIDParams{
			TemplateID: t1.ID,
		}).Asserts(t1, policy.ActionUpdate)
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
		}).Asserts(t1, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateDeletedByIDParams{
			ID:      t1.ID,
			Deleted: true,
		}).Asserts(t1, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateTemplateMetaByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateMetaByIDParams{
			ID:                  t1.ID,
			MaxPortSharingLevel: "owner",
		}).Asserts(t1, policy.ActionUpdate)
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
		}).Asserts(t1, policy.ActionUpdate)
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
		}).Asserts(t1, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateVersionExternalAuthProvidersByJobID", s.Subtest(func(db database.Store, check *expects) {
		jobID := uuid.New()
		t1 := dbgen.Template(s.T(), db, database.Template{})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			JobID:      jobID,
		})
		check.Args(database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams{
			JobID: jobID,
		}).Asserts(t1, policy.ActionUpdate).Returns()
	}))
	s.Run("GetTemplateInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateInsightsParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetUserLatencyInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetUserLatencyInsightsParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetUserActivityInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetUserActivityInsightsParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights).Errors(sql.ErrNoRows)
	}))
	s.Run("GetTemplateParameterInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateParameterInsightsParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateInsightsByInterval", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateInsightsByIntervalParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateInsightsByTemplate", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateInsightsByTemplateParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateAppInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateAppInsightsParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateAppInsightsByTemplate", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateAppInsightsByTemplateParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateUsageStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateUsageStatsParams{}).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights).Errors(sql.ErrNoRows)
	}))
	s.Run("UpsertTemplateUsageStats", s.Subtest(func(db database.Store, check *expects) {
		check.Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestUser() {
	s.Run("GetAuthorizedUsers", s.Subtest(func(db database.Store, check *expects) {
		dbgen.User(s.T(), db, database.User{})
		// No asserts because SQLFilter.
		check.Args(database.GetUsersParams{}, emptyPreparedAuthorized{}).
			Asserts()
	}))
	s.Run("DeleteAPIKeysByUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(rbac.ResourceApiKey.WithOwner(u.ID.String()), policy.ActionDelete).Returns()
	}))
	s.Run("GetQuotaAllowanceForUser", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetQuotaAllowanceForUserParams{
			UserID:         u.ID,
			OrganizationID: uuid.New(),
		}).Asserts(u, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetQuotaConsumedForUser", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetQuotaConsumedForUserParams{
			OwnerID:        u.ID,
			OrganizationID: uuid.New(),
		}).Asserts(u, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetUserByEmailOrUsername", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.GetUserByEmailOrUsernameParams{
			Username: u.Username,
			Email:    u.Email,
		}).Asserts(u, policy.ActionRead).Returns(u)
	}))
	s.Run("GetUserByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, policy.ActionRead).Returns(u)
	}))
	s.Run("GetUsersByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{CreatedAt: dbtime.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), db, database.User{CreatedAt: dbtime.Now()})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(a, policy.ActionRead, b, policy.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("GetUsers", s.Subtest(func(db database.Store, check *expects) {
		dbgen.User(s.T(), db, database.User{Username: "GetUsers-a-user"})
		dbgen.User(s.T(), db, database.User{Username: "GetUsers-b-user"})
		check.Args(database.GetUsersParams{}).
			// Asserts are done in a SQL filter
			Asserts()
	}))
	s.Run("InsertUser", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertUserParams{
			ID:        uuid.New(),
			LoginType: database.LoginTypePassword,
		}).Asserts(rbac.ResourceAssignRole, policy.ActionAssign, rbac.ResourceUser, policy.ActionCreate)
	}))
	s.Run("InsertUserLink", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertUserLinkParams{
			UserID:    u.ID,
			LoginType: database.LoginTypeOIDC,
		}).Asserts(u, policy.ActionUpdate)
	}))
	s.Run("UpdateUserDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(u, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateUserGithubComUserID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserGithubComUserIDParams{
			ID: u.ID,
		}).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateUserHashedPassword", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserHashedPasswordParams{
			ID: u.ID,
		}).Asserts(u, policy.ActionUpdatePersonal).Returns()
	}))
	s.Run("UpdateUserHashedOneTimePasscode", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserHashedOneTimePasscodeParams{
			ID: u.ID,
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateUserQuietHoursSchedule", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserQuietHoursScheduleParams{
			ID: u.ID,
		}).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateUserLastSeenAt", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserLastSeenAtParams{
			ID:         u.ID,
			UpdatedAt:  u.UpdatedAt,
			LastSeenAt: u.LastSeenAt,
		}).Asserts(u, policy.ActionUpdate).Returns(u)
	}))
	s.Run("UpdateUserProfile", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserProfileParams{
			ID:        u.ID,
			Email:     u.Email,
			Username:  u.Username,
			Name:      u.Name,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u, policy.ActionUpdatePersonal).Returns(u)
	}))
	s.Run("GetUserWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(
			database.GetUserWorkspaceBuildParametersParams{
				OwnerID:    u.ID,
				TemplateID: uuid.UUID{},
			},
		).Asserts(u, policy.ActionReadPersonal).Returns(
			[]database.GetUserWorkspaceBuildParametersRow{},
		)
	}))
	s.Run("UpdateUserAppearanceSettings", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserAppearanceSettingsParams{
			ID:              u.ID,
			ThemePreference: u.ThemePreference,
			UpdatedAt:       u.UpdatedAt,
		}).Asserts(u, policy.ActionUpdatePersonal).Returns(u)
	}))
	s.Run("UpdateUserStatus", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserStatusParams{
			ID:        u.ID,
			Status:    u.Status,
			UpdatedAt: u.UpdatedAt,
		}).Asserts(u, policy.ActionUpdate).Returns(u)
	}))
	s.Run("DeleteGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(key.UserID).Asserts(rbac.ResourceUserObject(key.UserID), policy.ActionUpdatePersonal).Returns()
	}))
	s.Run("GetGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(key.UserID).Asserts(rbac.ResourceUserObject(key.UserID), policy.ActionReadPersonal).Returns(key)
	}))
	s.Run("InsertGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertGitSSHKeyParams{
			UserID: u.ID,
		}).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateGitSSHKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.GitSSHKey(s.T(), db, database.GitSSHKey{})
		check.Args(database.UpdateGitSSHKeyParams{
			UserID:    key.UserID,
			UpdatedAt: key.UpdatedAt,
		}).Asserts(rbac.ResourceUserObject(key.UserID), policy.ActionUpdatePersonal).Returns(key)
	}))
	s.Run("GetExternalAuthLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		check.Args(database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Asserts(rbac.ResourceUserObject(link.UserID), policy.ActionReadPersonal).Returns(link)
	}))
	s.Run("InsertExternalAuthLink", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertExternalAuthLinkParams{
			ProviderID: uuid.NewString(),
			UserID:     u.ID,
		}).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("RemoveRefreshToken", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		check.Args(database.RemoveRefreshTokenParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
			UpdatedAt:  link.UpdatedAt,
		}).Asserts(rbac.ResourceUserObject(link.UserID), policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateExternalAuthLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		check.Args(database.UpdateExternalAuthLinkParams{
			ProviderID:        link.ProviderID,
			UserID:            link.UserID,
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
			UpdatedAt:         link.UpdatedAt,
		}).Asserts(rbac.ResourceUserObject(link.UserID), policy.ActionUpdatePersonal).Returns(link)
	}))
	s.Run("UpdateUserLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(database.UpdateUserLinkParams{
			OAuthAccessToken:  link.OAuthAccessToken,
			OAuthRefreshToken: link.OAuthRefreshToken,
			OAuthExpiry:       link.OAuthExpiry,
			UserID:            link.UserID,
			LoginType:         link.LoginType,
			Claims:            database.UserLinkClaims{},
		}).Asserts(rbac.ResourceUserObject(link.UserID), policy.ActionUpdatePersonal).Returns(link)
	}))
	s.Run("UpdateUserRoles", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{RBACRoles: []string{codersdk.RoleTemplateAdmin}})
		o := u
		o.RBACRoles = []string{codersdk.RoleUserAdmin}
		check.Args(database.UpdateUserRolesParams{
			GrantedRoles: []string{codersdk.RoleUserAdmin},
			ID:           u.ID,
		}).Asserts(
			u, policy.ActionRead,
			rbac.ResourceAssignRole, policy.ActionAssign,
			rbac.ResourceAssignRole, policy.ActionDelete,
		).Returns(o)
	}))
	s.Run("AllUserIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{})
		b := dbgen.User(s.T(), db, database.User{})
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(slice.New(a.ID, b.ID))
	}))
	s.Run("CustomRoles", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.CustomRolesParams{}).Asserts(rbac.ResourceAssignRole, policy.ActionRead).Returns([]database.CustomRole{})
	}))
	s.Run("Organization/DeleteCustomRole", s.Subtest(func(db database.Store, check *expects) {
		customRole := dbgen.CustomRole(s.T(), db, database.CustomRole{
			OrganizationID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		})
		check.Args(database.DeleteCustomRoleParams{
			Name:           customRole.Name,
			OrganizationID: customRole.OrganizationID,
		}).Asserts(
			rbac.ResourceAssignOrgRole.InOrg(customRole.OrganizationID.UUID), policy.ActionDelete)
	}))
	s.Run("Site/DeleteCustomRole", s.Subtest(func(db database.Store, check *expects) {
		customRole := dbgen.CustomRole(s.T(), db, database.CustomRole{
			OrganizationID: uuid.NullUUID{
				UUID:  uuid.Nil,
				Valid: false,
			},
		})
		check.Args(database.DeleteCustomRoleParams{
			Name: customRole.Name,
		}).Asserts(
			rbac.ResourceAssignRole, policy.ActionDelete)
	}))
	s.Run("Blank/UpdateCustomRole", s.Subtest(func(db database.Store, check *expects) {
		customRole := dbgen.CustomRole(s.T(), db, database.CustomRole{})
		// Blank is no perms in the role
		check.Args(database.UpdateCustomRoleParams{
			Name:            customRole.Name,
			DisplayName:     "Test Name",
			SitePermissions: nil,
			OrgPermissions:  nil,
			UserPermissions: nil,
		}).Asserts(rbac.ResourceAssignRole, policy.ActionUpdate)
	}))
	s.Run("SitePermissions/UpdateCustomRole", s.Subtest(func(db database.Store, check *expects) {
		customRole := dbgen.CustomRole(s.T(), db, database.CustomRole{
			OrganizationID: uuid.NullUUID{
				UUID:  uuid.Nil,
				Valid: false,
			},
		})
		check.Args(database.UpdateCustomRoleParams{
			Name:           customRole.Name,
			OrganizationID: customRole.OrganizationID,
			DisplayName:    "Test Name",
			SitePermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionDelete, codersdk.ActionViewInsights},
			}), convertSDKPerm),
			OrgPermissions: nil,
			UserPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}), convertSDKPerm),
		}).Asserts(
			// First check
			rbac.ResourceAssignRole, policy.ActionUpdate,
			// Escalation checks
			rbac.ResourceTemplate, policy.ActionCreate,
			rbac.ResourceTemplate, policy.ActionRead,
			rbac.ResourceTemplate, policy.ActionUpdate,
			rbac.ResourceTemplate, policy.ActionDelete,
			rbac.ResourceTemplate, policy.ActionViewInsights,

			rbac.ResourceWorkspace.WithOwner(testActorID.String()), policy.ActionRead,
		)
	}))
	s.Run("OrgPermissions/UpdateCustomRole", s.Subtest(func(db database.Store, check *expects) {
		orgID := uuid.New()
		customRole := dbgen.CustomRole(s.T(), db, database.CustomRole{
			OrganizationID: uuid.NullUUID{
				UUID:  orgID,
				Valid: true,
			},
		})

		check.Args(database.UpdateCustomRoleParams{
			Name:            customRole.Name,
			DisplayName:     "Test Name",
			OrganizationID:  customRole.OrganizationID,
			SitePermissions: nil,
			OrgPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead},
			}), convertSDKPerm),
			UserPermissions: nil,
		}).Asserts(
			// First check
			rbac.ResourceAssignOrgRole.InOrg(orgID), policy.ActionUpdate,
			// Escalation checks
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionCreate,
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionRead,
		)
	}))
	s.Run("Blank/InsertCustomRole", s.Subtest(func(db database.Store, check *expects) {
		// Blank is no perms in the role
		check.Args(database.InsertCustomRoleParams{
			Name:            "test",
			DisplayName:     "Test Name",
			SitePermissions: nil,
			OrgPermissions:  nil,
			UserPermissions: nil,
		}).Asserts(rbac.ResourceAssignRole, policy.ActionCreate)
	}))
	s.Run("SitePermissions/InsertCustomRole", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertCustomRoleParams{
			Name:        "test",
			DisplayName: "Test Name",
			SitePermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionDelete, codersdk.ActionViewInsights},
			}), convertSDKPerm),
			OrgPermissions: nil,
			UserPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}), convertSDKPerm),
		}).Asserts(
			// First check
			rbac.ResourceAssignRole, policy.ActionCreate,
			// Escalation checks
			rbac.ResourceTemplate, policy.ActionCreate,
			rbac.ResourceTemplate, policy.ActionRead,
			rbac.ResourceTemplate, policy.ActionUpdate,
			rbac.ResourceTemplate, policy.ActionDelete,
			rbac.ResourceTemplate, policy.ActionViewInsights,

			rbac.ResourceWorkspace.WithOwner(testActorID.String()), policy.ActionRead,
		)
	}))
	s.Run("OrgPermissions/InsertCustomRole", s.Subtest(func(db database.Store, check *expects) {
		orgID := uuid.New()
		check.Args(database.InsertCustomRoleParams{
			Name:        "test",
			DisplayName: "Test Name",
			OrganizationID: uuid.NullUUID{
				UUID:  orgID,
				Valid: true,
			},
			SitePermissions: nil,
			OrgPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead},
			}), convertSDKPerm),
			UserPermissions: nil,
		}).Asserts(
			// First check
			rbac.ResourceAssignOrgRole.InOrg(orgID), policy.ActionCreate,
			// Escalation checks
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionCreate,
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionRead,
		)
	}))
}

func (s *MethodTestSuite) TestWorkspace() {
	s.Run("GetWorkspaceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(ws.ID).Asserts(ws, policy.ActionRead)
	}))
	s.Run("GetWorkspaces", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		_ = dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		// No asserts here because SQLFilter.
		check.Args(database.GetWorkspacesParams{}).Asserts()
	}))
	s.Run("GetAuthorizedWorkspaces", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		_ = dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		// No asserts here because SQLFilter.
		check.Args(database.GetWorkspacesParams{}, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("GetWorkspacesAndAgentsByOwnerID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		_ = dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		// No asserts here because SQLFilter.
		check.Args(ws.OwnerID).Asserts()
	}))
	s.Run("GetAuthorizedWorkspacesAndAgentsByOwnerID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		_ = dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		// No asserts here because SQLFilter.
		check.Args(ws.OwnerID, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("GetLatestWorkspaceBuildByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(ws.ID).Asserts(ws, policy.ActionRead).Returns(b)
	}))
	s.Run("GetWorkspaceAgentByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, policy.ActionRead).Returns(agt)
	}))
	s.Run("GetWorkspaceAgentLifecycleStateByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentMetadata", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		_ = db.InsertWorkspaceAgentMetadata(context.Background(), database.InsertWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agt.ID,
			DisplayName:      "test",
			Key:              "test",
		})
		check.Args(database.GetWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agt.ID,
			Keys:             []string{"test"},
		}).Asserts(ws, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentByInstanceID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.AuthInstanceID.String).Asserts(ws, policy.ActionRead).Returns(agt)
	}))
	s.Run("UpdateWorkspaceAgentLifecycleStateByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agt.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentMetadata", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agt.ID,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentLogOverflowByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentLogOverflowByIDParams{
			ID:             agt.ID,
			LogsOverflowed: true,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentStartupByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentStartupByIDParams{
			ID: agt.ID,
			Subsystems: []database.WorkspaceAgentSubsystem{
				database.WorkspaceAgentSubsystemEnvbox,
			},
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceAgentLogsAfter", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.GetWorkspaceAgentLogsAfterParams{
			AgentID: agt.ID,
		}).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceAgentLog{})
	}))
	s.Run("GetWorkspaceAppByAgentIDAndSlug", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})

		check.Args(database.GetWorkspaceAppByAgentIDAndSlugParams{
			AgentID: agt.ID,
			Slug:    app.Slug,
		}).Asserts(ws, policy.ActionRead).Returns(app)
	}))
	s.Run("GetWorkspaceAppsByAgentID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		a := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		b := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})

		check.Args(agt.ID).Asserts(ws, policy.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetWorkspaceBuildByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.ID).Asserts(ws, policy.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByJobID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.JobID).Asserts(ws, policy.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByWorkspaceIDAndBuildNumber", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 10})
		check.Args(database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
			WorkspaceID: ws.ID,
			BuildNumber: build.BuildNumber,
		}).Asserts(ws, policy.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args(build.ID).Asserts(ws, policy.ActionRead).
			Returns([]database.WorkspaceBuildParameter{})
	}))
	s.Run("GetWorkspaceBuildsByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 1})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 2})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, BuildNumber: 3})
		check.Args(database.GetWorkspaceBuildsByWorkspaceIDParams{WorkspaceID: ws.ID}).Asserts(ws, policy.ActionRead) // ordering
	}))
	s.Run("GetWorkspaceByAgentID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentsInLatestBuildByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(ws.ID).Asserts(ws, policy.ActionRead)
	}))
	s.Run("GetWorkspaceByOwnerIDAndName", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: ws.OwnerID,
			Deleted: ws.Deleted,
			Name:    ws.Name,
		}).Asserts(ws, policy.ActionRead)
	}))
	s.Run("GetWorkspaceResourceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args(res.ID).Asserts(ws, policy.ActionRead).Returns(res)
	}))
	s.Run("Build/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args(job.ID).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceResource{})
	}))
	s.Run("Template/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})
		check.Args(job.ID).Asserts(v.RBACObject(tpl), []policy.Action{policy.ActionRead, policy.ActionRead}).Returns([]database.WorkspaceResource{})
	}))
	s.Run("InsertWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(database.InsertWorkspaceParams{
			ID:               uuid.New(),
			OwnerID:          u.ID,
			OrganizationID:   o.ID,
			AutomaticUpdates: database.AutomaticUpdatesNever,
		}).Asserts(rbac.ResourceWorkspace.WithOwner(u.ID.String()).InOrg(o.ID), policy.ActionCreate)
	}))
	s.Run("Start/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		t := dbgen.Template(s.T(), db, database.Template{})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: t.ID,
		})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, policy.ActionWorkspaceStart)
	}))
	s.Run("Stop/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		t := dbgen.Template(s.T(), db, database.Template{})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: t.ID,
		})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionStop,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, policy.ActionWorkspaceStop)
	}))
	s.Run("Start/RequireActiveVersion/VersionMismatch/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		t := dbgen.Template(s.T(), db, database.Template{})
		ctx := testutil.Context(s.T(), testutil.WaitShort)
		err := db.UpdateTemplateAccessControlByID(ctx, database.UpdateTemplateAccessControlByIDParams{
			ID:                   t.ID,
			RequireActiveVersion: true,
		})
		require.NoError(s.T(), err)
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t.ID},
		})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: t.ID,
		})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: v.ID,
		}).Asserts(
			w, policy.ActionWorkspaceStart,
			t, policy.ActionUpdate,
		)
	}))
	s.Run("Start/RequireActiveVersion/VersionsMatch/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{})
		t := dbgen.Template(s.T(), db, database.Template{
			ActiveVersionID: v.ID,
		})

		ctx := testutil.Context(s.T(), testutil.WaitShort)
		err := db.UpdateTemplateAccessControlByID(ctx, database.UpdateTemplateAccessControlByIDParams{
			ID:                   t.ID,
			RequireActiveVersion: true,
		})
		require.NoError(s.T(), err)

		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: t.ID,
		})
		// Assert that we do not check for template update permissions
		// if versions match.
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: v.ID,
		}).Asserts(
			w, policy.ActionWorkspaceStart,
		)
	}))
	s.Run("Delete/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionDelete,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w, policy.ActionDelete)
	}))
	s.Run("InsertWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: w.ID})
		check.Args(database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: b.ID,
			Name:             []string{"foo", "bar"},
			Value:            []string{"baz", "qux"},
		}).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspace", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		expected := w
		expected.Name = ""
		check.Args(database.UpdateWorkspaceParams{
			ID: w.ID,
		}).Asserts(w, policy.ActionUpdate).Returns(expected)
	}))
	s.Run("UpdateWorkspaceDormantDeletingAt", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.UpdateWorkspaceDormantDeletingAtParams{
			ID: w.ID,
		}).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceAutomaticUpdates", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.UpdateWorkspaceAutomaticUpdatesParams{
			ID:               w.ID,
			AutomaticUpdates: database.AutomaticUpdatesAlways,
		}).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceAppHealthByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		check.Args(database.UpdateWorkspaceAppHealthByIDParams{
			ID:     app.ID,
			Health: database.WorkspaceAppHealthDisabled,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAutostart", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.UpdateWorkspaceAutostartParams{
			ID: ws.ID,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceBuildDeadlineByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		check.Args(database.UpdateWorkspaceBuildDeadlineByIDParams{
			ID:        build.ID,
			UpdatedAt: build.UpdatedAt,
			Deadline:  build.Deadline,
		}).Asserts(ws, policy.ActionUpdate)
	}))
	s.Run("SoftDeleteWorkspaceByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		ws.Deleted = true
		check.Args(ws.ID).Asserts(ws, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceDeletedByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{Deleted: true})
		check.Args(database.UpdateWorkspaceDeletedByIDParams{
			ID:      ws.ID,
			Deleted: true,
		}).Asserts(ws, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceLastUsedAt", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.UpdateWorkspaceLastUsedAtParams{
			ID: ws.ID,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("BatchUpdateWorkspaceLastUsedAt", s.Subtest(func(db database.Store, check *expects) {
		ws1 := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		ws2 := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.BatchUpdateWorkspaceLastUsedAtParams{
			IDs: []uuid.UUID{ws1.ID, ws2.ID},
		}).Asserts(rbac.ResourceWorkspace.All(), policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceTTL", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		check.Args(database.UpdateWorkspaceTTLParams{
			ID: ws.ID,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceByWorkspaceAppID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		app := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: agt.ID})
		check.Args(app.ID).Asserts(ws, policy.ActionRead)
	}))
	s.Run("ActivityBumpWorkspace", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args(database.ActivityBumpWorkspaceParams{
			WorkspaceID: ws.ID,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("FavoriteWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID})
		check.Args(ws.ID).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("UnfavoriteWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID})
		check.Args(ws.ID).Asserts(ws, policy.ActionUpdate).Returns()
	}))
}

func (s *MethodTestSuite) TestWorkspacePortSharing() {
	s.Run("UpsertWorkspaceAgentPortShare", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID})
		ps := dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		//nolint:gosimple // casting is not a simplification
		check.Args(database.UpsertWorkspaceAgentPortShareParams{
			WorkspaceID: ps.WorkspaceID,
			AgentName:   ps.AgentName,
			Port:        ps.Port,
			ShareLevel:  ps.ShareLevel,
			Protocol:    ps.Protocol,
		}).Asserts(ws, policy.ActionUpdate).Returns(ps)
	}))
	s.Run("GetWorkspaceAgentPortShare", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID})
		ps := dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(database.GetWorkspaceAgentPortShareParams{
			WorkspaceID: ps.WorkspaceID,
			AgentName:   ps.AgentName,
			Port:        ps.Port,
		}).Asserts(ws, policy.ActionRead).Returns(ps)
	}))
	s.Run("ListWorkspaceAgentPortShares", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID})
		ps := dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(ws.ID).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceAgentPortShare{ps})
	}))
	s.Run("DeleteWorkspaceAgentPortShare", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID})
		ps := dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(database.DeleteWorkspaceAgentPortShareParams{
			WorkspaceID: ps.WorkspaceID,
			AgentName:   ps.AgentName,
			Port:        ps.Port,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("DeleteWorkspaceAgentPortSharesByTemplate", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		t := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID, TemplateID: t.ID})
		_ = dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(t.ID).Asserts(t, policy.ActionUpdate).Returns()
	}))
	s.Run("ReduceWorkspaceAgentShareLevelToAuthenticatedByTemplate", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		t := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OwnerID: u.ID, TemplateID: t.ID})
		_ = dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(t.ID).Asserts(t, policy.ActionUpdate).Returns()
	}))
}

func (s *MethodTestSuite) TestProvisionerKeys() {
	s.Run("InsertProvisionerKey", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pk := database.ProvisionerKey{
			ID:             uuid.New(),
			CreatedAt:      time.Now(),
			OrganizationID: org.ID,
			Name:           strings.ToLower(coderdtest.RandomName(s.T())),
			HashedSecret:   []byte(coderdtest.RandomName(s.T())),
		}
		//nolint:gosimple // casting is not a simplification
		check.Args(database.InsertProvisionerKeyParams{
			ID:             pk.ID,
			CreatedAt:      pk.CreatedAt,
			OrganizationID: pk.OrganizationID,
			Name:           pk.Name,
			HashedSecret:   pk.HashedSecret,
		}).Asserts(pk, policy.ActionCreate).Returns(pk)
	}))
	s.Run("GetProvisionerKeyByID", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pk := dbgen.ProvisionerKey(s.T(), db, database.ProvisionerKey{OrganizationID: org.ID})
		check.Args(pk.ID).Asserts(pk, policy.ActionRead).Returns(pk)
	}))
	s.Run("GetProvisionerKeyByHashedSecret", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pk := dbgen.ProvisionerKey(s.T(), db, database.ProvisionerKey{OrganizationID: org.ID, HashedSecret: []byte("foo")})
		check.Args([]byte("foo")).Asserts(pk, policy.ActionRead).Returns(pk)
	}))
	s.Run("GetProvisionerKeyByName", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pk := dbgen.ProvisionerKey(s.T(), db, database.ProvisionerKey{OrganizationID: org.ID})
		check.Args(database.GetProvisionerKeyByNameParams{
			OrganizationID: org.ID,
			Name:           pk.Name,
		}).Asserts(pk, policy.ActionRead).Returns(pk)
	}))
	s.Run("ListProvisionerKeysByOrganization", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pk := dbgen.ProvisionerKey(s.T(), db, database.ProvisionerKey{OrganizationID: org.ID})
		pks := []database.ProvisionerKey{
			{
				ID:             pk.ID,
				CreatedAt:      pk.CreatedAt,
				OrganizationID: pk.OrganizationID,
				Name:           pk.Name,
			},
		}
		check.Args(org.ID).Asserts(pk, policy.ActionRead).Returns(pks)
	}))
	s.Run("ListProvisionerKeysByOrganizationExcludeReserved", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pk := dbgen.ProvisionerKey(s.T(), db, database.ProvisionerKey{OrganizationID: org.ID})
		pks := []database.ProvisionerKey{
			{
				ID:             pk.ID,
				CreatedAt:      pk.CreatedAt,
				OrganizationID: pk.OrganizationID,
				Name:           pk.Name,
			},
		}
		check.Args(org.ID).Asserts(pk, policy.ActionRead).Returns(pks)
	}))
	s.Run("DeleteProvisionerKey", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pk := dbgen.ProvisionerKey(s.T(), db, database.ProvisionerKey{OrganizationID: org.ID})
		check.Args(pk.ID).Asserts(pk, policy.ActionDelete).Returns()
	}))
}

func (s *MethodTestSuite) TestExtraMethods() {
	s.Run("GetProvisionerDaemons", s.Subtest(func(db database.Store, check *expects) {
		d, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		check.Args().Asserts(d, policy.ActionRead)
	}))
	s.Run("GetProvisionerDaemonsByOrganization", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		d, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			OrganizationID: org.ID,
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		ds, err := db.GetProvisionerDaemonsByOrganization(context.Background(), database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: org.ID})
		s.NoError(err, "get provisioner daemon by org")
		check.Args(database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: org.ID}).Asserts(d, policy.ActionRead).Returns(ds)
	}))
	s.Run("DeleteOldProvisionerDaemons", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("UpdateProvisionerDaemonLastSeenAt", s.Subtest(func(db database.Store, check *expects) {
		d, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		check.Args(database.UpdateProvisionerDaemonLastSeenAtParams{
			ID:         d.ID,
			LastSeenAt: sql.NullTime{Time: dbtime.Now(), Valid: true},
		}).Asserts(rbac.ResourceProvisionerDaemon, policy.ActionUpdate)
	}))
}

// All functions in this method test suite are not implemented in dbmem, but
// we still want to assert RBAC checks.
func (s *MethodTestSuite) TestTailnetFunctions() {
	s.Run("CleanTailnetCoordinators", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("CleanTailnetLostPeers", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("CleanTailnetTunnels", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteAllTailnetClientSubscriptions", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteAllTailnetClientSubscriptionsParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteAllTailnetTunnels", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteAllTailnetTunnelsParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteCoordinator", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetAgentParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetClient", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetClientParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetClientSubscription", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetClientSubscriptionParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetPeer", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetPeerParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetTunnel", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetTunnelParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetAgents", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetAgents", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetClientsForAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetPeers", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetTunnelPeerBindings", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetTunnelPeerIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetCoordinators", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetPeers", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetTunnels", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetAgentParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetClient", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetClientParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetClientSubscription", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetClientSubscriptionParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetCoordinator", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetPeer", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetPeerParams{
			Status: database.TailnetStatusOk,
		}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionCreate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetTunnel", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetTunnelParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionCreate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpdateTailnetPeerStatusByCoordinator", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.UpdateTailnetPeerStatusByCoordinatorParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
}

func (s *MethodTestSuite) TestDBCrypt() {
	s.Run("GetDBCryptKeys", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.DBCryptKey{})
	}))
	s.Run("InsertDBCryptKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertDBCryptKeyParams{}).
			Asserts(rbac.ResourceSystem, policy.ActionCreate).
			Returns()
	}))
	s.Run("RevokeDBCryptKey", s.Subtest(func(db database.Store, check *expects) {
		err := db.InsertDBCryptKey(context.Background(), database.InsertDBCryptKeyParams{
			ActiveKeyDigest: "revoke me",
		})
		s.NoError(err)
		check.Args("revoke me").
			Asserts(rbac.ResourceSystem, policy.ActionUpdate).
			Returns()
	}))
}

func (s *MethodTestSuite) TestCryptoKeys() {
	s.Run("GetCryptoKeys", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceCryptoKey, policy.ActionRead)
	}))
	s.Run("InsertCryptoKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertCryptoKeyParams{
			Feature: database.CryptoKeyFeatureWorkspaceAppsAPIKey,
		}).
			Asserts(rbac.ResourceCryptoKey, policy.ActionCreate)
	}))
	s.Run("DeleteCryptoKey", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.CryptoKey(s.T(), db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: 4,
		})
		check.Args(database.DeleteCryptoKeyParams{
			Feature:  key.Feature,
			Sequence: key.Sequence,
		}).Asserts(rbac.ResourceCryptoKey, policy.ActionDelete)
	}))
	s.Run("GetCryptoKeyByFeatureAndSequence", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.CryptoKey(s.T(), db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: 4,
		})
		check.Args(database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  key.Feature,
			Sequence: key.Sequence,
		}).Asserts(rbac.ResourceCryptoKey, policy.ActionRead).Returns(key)
	}))
	s.Run("GetLatestCryptoKeyByFeature", s.Subtest(func(db database.Store, check *expects) {
		dbgen.CryptoKey(s.T(), db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: 4,
		})
		check.Args(database.CryptoKeyFeatureWorkspaceAppsAPIKey).Asserts(rbac.ResourceCryptoKey, policy.ActionRead)
	}))
	s.Run("UpdateCryptoKeyDeletesAt", s.Subtest(func(db database.Store, check *expects) {
		key := dbgen.CryptoKey(s.T(), db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: 4,
		})
		check.Args(database.UpdateCryptoKeyDeletesAtParams{
			Feature:   key.Feature,
			Sequence:  key.Sequence,
			DeletesAt: sql.NullTime{Time: time.Now(), Valid: true},
		}).Asserts(rbac.ResourceCryptoKey, policy.ActionUpdate)
	}))
	s.Run("GetCryptoKeysByFeature", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.CryptoKeyFeatureWorkspaceAppsAPIKey).
			Asserts(rbac.ResourceCryptoKey, policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestSystemFunctions() {
	s.Run("UpdateUserLinkedID", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		l := dbgen.UserLink(s.T(), db, database.UserLink{UserID: u.ID})
		check.Args(database.UpdateUserLinkedIDParams{
			UserID:    u.ID,
			LinkedID:  l.LinkedID,
			LoginType: database.LoginTypeGithub,
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns(l)
	}))
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args([]uuid.UUID{ws.ID}).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(slice.New(b))
	}))
	s.Run("UpsertDefaultProxy", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertDefaultProxyParams{}).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))
	s.Run("GetUserLinkByLinkedID", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(l.LinkedID).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(l)
	}))
	s.Run("GetUserLinkByUserIDLoginType", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    l.UserID,
			LoginType: l.LoginType,
		}).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(l)
	}))
	s.Run("GetLatestWorkspaceBuilds", s.Subtest(func(db database.Store, check *expects) {
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetActiveUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetUnexpiredLicenses", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetAuthorizationUserRoles", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		db.InsertDERPMeshKey(context.Background(), "testing")
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("InsertDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceSystem, policy.ActionCreate).Returns()
	}))
	s.Run("InsertDeploymentID", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceSystem, policy.ActionCreate).Returns()
	}))
	s.Run("InsertReplica", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertReplicaParams{
			ID: uuid.New(),
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpdateReplica", s.Subtest(func(db database.Store, check *expects) {
		replica, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New()})
		require.NoError(s.T(), err)
		check.Args(database.UpdateReplicaParams{
			ID:              replica.ID,
			DatabaseLatency: 100,
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("DeleteReplicasUpdatedBefore", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour)).Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("GetReplicasUpdatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour*-1)).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetTemplates", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Template(s.T(), db, database.Template{})
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("UpdateWorkspaceBuildCostByID", s.Subtest(func(db database.Store, check *expects) {
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		o := b
		o.DailyCost = 10
		check.Args(database.UpdateWorkspaceBuildCostByIDParams{
			ID:        b.ID,
			DailyCost: 10,
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceBuildProvisionerStateByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		check.Args(database.UpdateWorkspaceBuildProvisionerStateByIDParams{
			ID:               build.ID,
			ProvisionerState: []byte("testing"),
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("UpsertLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		err := db.UpsertLastUpdateCheck(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceBuildsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAppsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceResourcesCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceResourceMetadataCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResourceMetadatums(s.T(), db, database.WorkspaceResourceMetadatum{})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("DeleteOldWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("GetProvisionerJobsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		// TODO: add provisioner job resource type
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts( /*rbac.ResourceSystem, policy.ActionRead*/ )
	}))
	s.Run("GetTemplateVersionsByIDs", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		t2 := dbgen.Template(s.T(), db, database.Template{})
		tv1 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
		})
		tv2 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
		})
		tv3 := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t2.ID, Valid: true},
		})
		check.Args([]uuid.UUID{tv1.ID, tv2.ID, tv3.ID}).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns(slice.New(tv1, tv2, tv3))
	}))
	s.Run("GetParameterSchemasByJobID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		job := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: tv.JobID})
		check.Args(job.ID).
			Asserts(tpl, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetWorkspaceAppsByAgentIDs", s.Subtest(func(db database.Store, check *expects) {
		aWs := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		aBuild := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: aWs.ID, JobID: uuid.New()})
		aRes := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: aBuild.JobID})
		aAgt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: aRes.ID})
		a := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: aAgt.ID})

		bWs := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		bBuild := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: bWs.ID, JobID: uuid.New()})
		bRes := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: bBuild.JobID})
		bAgt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: bRes.ID})
		b := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: bAgt.ID})

		check.Args([]uuid.UUID{a.AgentID, b.AgentID}).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.WorkspaceApp{a, b})
	}))
	s.Run("GetWorkspaceResourcesByJobIDs", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		tJob := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})

		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		wJob := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args([]uuid.UUID{tJob.ID, wJob.ID}).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.WorkspaceResource{})
	}))
	s.Run("GetWorkspaceResourceMetadataByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		a := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		b := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentsByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args([]uuid.UUID{res.ID}).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.WorkspaceAgent{agt})
	}))
	s.Run("GetProvisionerJobsByIDs", s.Subtest(func(db database.Store, check *expects) {
		// TODO: add a ProvisionerJob resource type
		a := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		b := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts( /*rbac.ResourceSystem, policy.ActionRead*/ ).
			Returns(slice.New(a, b))
	}))
	s.Run("InsertWorkspaceAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentParams{
			ID: uuid.New(),
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceApp", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAppParams{
			ID:           uuid.New(),
			Health:       database.WorkspaceAppHealthDisabled,
			SharingLevel: database.AppSharingLevelOwner,
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceResourceMetadata", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceResourceMetadataParams{
			WorkspaceResourceID: uuid.New(),
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpdateWorkspaceAgentConnectionByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentConnectionByIDParams{
			ID: agt.ID,
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))
	s.Run("AcquireProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			StartedAt: sql.NullTime{Valid: false},
		})
		check.Args(database.AcquireProvisionerJobParams{OrganizationID: j.OrganizationID, Types: []database.ProvisionerType{j.Provisioner}, ProvisionerTags: must(json.Marshal(j.Tags))}).
			Asserts( /*rbac.ResourceSystem, policy.ActionUpdate*/ )
	}))
	s.Run("UpdateProvisionerJobWithCompleteByID", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: j.ID,
		}).Asserts( /*rbac.ResourceSystem, policy.ActionUpdate*/ )
	}))
	s.Run("UpdateProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobByIDParams{
			ID:        j.ID,
			UpdatedAt: time.Now(),
		}).Asserts( /*rbac.ResourceSystem, policy.ActionUpdate*/ )
	}))
	s.Run("InsertProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		check.Args(database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		}).Asserts( /*rbac.ResourceSystem, policy.ActionCreate*/ )
	}))
	s.Run("InsertProvisionerJobLogs", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args(database.InsertProvisionerJobLogsParams{
			JobID: j.ID,
		}).Asserts( /*rbac.ResourceSystem, policy.ActionCreate*/ )
	}))
	s.Run("InsertProvisionerJobTimings", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args(database.InsertProvisionerJobTimingsParams{
			JobID: j.ID,
		}).Asserts( /*rbac.ResourceSystem, policy.ActionCreate*/ )
	}))
	s.Run("UpsertProvisionerDaemon", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		pd := rbac.ResourceProvisionerDaemon.InOrg(org.ID)
		check.Args(database.UpsertProvisionerDaemonParams{
			OrganizationID: org.ID,
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		}).Asserts(pd, policy.ActionCreate)
		check.Args(database.UpsertProvisionerDaemonParams{
			OrganizationID: org.ID,
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeUser,
				provisionersdk.TagOwner: "11111111-1111-1111-1111-111111111111",
			}),
		}).Asserts(pd.WithOwner("11111111-1111-1111-1111-111111111111"), policy.ActionCreate)
	}))
	s.Run("InsertTemplateVersionParameter", s.Subtest(func(db database.Store, check *expects) {
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{})
		check.Args(database.InsertTemplateVersionParameterParams{
			TemplateVersionID: v.ID,
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceResource", s.Subtest(func(db database.Store, check *expects) {
		r := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{})
		check.Args(database.InsertWorkspaceResourceParams{
			ID:         r.ID,
			Transition: database.WorkspaceTransitionStart,
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("DeleteOldWorkspaceAgentLogs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("InsertWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentStatsParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate).Errors(errMatchAny)
	}))
	s.Run("InsertWorkspaceAppStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAppStatsParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentScriptTimings", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentScriptTimingsParams{
			ScriptID: uuid.New(),
			Stage:    database.WorkspaceAgentScriptTimingStageStart,
			Status:   database.WorkspaceAgentScriptTimingStatusOk,
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentScripts", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentScriptsParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentMetadata", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentMetadataParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentLogs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentLogsParams{}).Asserts()
	}))
	s.Run("InsertWorkspaceAgentLogSources", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentLogSourcesParams{}).Asserts()
	}))
	s.Run("GetTemplateDAUs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateDAUsParams{}).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetActiveWorkspaceBuildsByTemplateID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetDeploymentDAUs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(int32(0)).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetAppSecurityKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("UpsertAppSecurityKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("foo").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetApplicationName", s.Subtest(func(db database.Store, check *expects) {
		db.UpsertApplicationName(context.Background(), "foo")
		check.Args().Asserts()
	}))
	s.Run("UpsertApplicationName", s.Subtest(func(db database.Store, check *expects) {
		check.Args("").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetHealthSettings", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("UpsertHealthSettings", s.Subtest(func(db database.Store, check *expects) {
		check.Args("foo").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetNotificationsSettings", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("UpsertNotificationsSettings", s.Subtest(func(db database.Store, check *expects) {
		check.Args("foo").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetDeploymentWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetDeploymentWorkspaceAgentUsageStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetDeploymentWorkspaceStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("GetFileTemplates", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetHungProvisionerJobs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("UpsertOAuthSigningKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("foo").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetOAuthSigningKey", s.Subtest(func(db database.Store, check *expects) {
		db.UpsertOAuthSigningKey(context.Background(), "foo")
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("UpsertCoordinatorResumeTokenSigningKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("foo").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetCoordinatorResumeTokenSigningKey", s.Subtest(func(db database.Store, check *expects) {
		db.UpsertCoordinatorResumeTokenSigningKey(context.Background(), "foo")
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("InsertMissingGroups", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertMissingGroupsParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate).Errors(errMatchAny)
	}))
	s.Run("UpdateUserLoginType", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserLoginTypeParams{
			NewLoginType: database.LoginTypePassword,
			UserID:       u.ID,
		}).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetWorkspaceAgentStatsAndLabels", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetWorkspaceAgentUsageStatsAndLabels", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetWorkspaceAgentUsageStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetWorkspaceProxyByHostname", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{
			WildcardHostname: "*.example.com",
		})
		check.Args(database.GetWorkspaceProxyByHostnameParams{
			Hostname:              "foo.example.com",
			AllowWildcardHostname: true,
		}).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(p)
	}))
	s.Run("GetTemplateAverageBuildTime", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateAverageBuildTimeParams{}).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspacesEligibleForTransition", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("InsertTemplateVersionVariable", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertTemplateVersionVariableParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertTemplateVersionWorkspaceTag", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertTemplateVersionWorkspaceTagParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpdateInactiveUsersToDormant", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpdateInactiveUsersToDormantParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate).Errors(sql.ErrNoRows)
	}))
	s.Run("GetWorkspaceUniqueOwnerCountByTemplateIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{uuid.New()}).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentScriptsByAgentIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{uuid.New()}).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentLogSourcesByAgentIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{uuid.New()}).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetProvisionerJobsByIDsWithQueuePosition", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{}).Asserts()
	}))
	s.Run("GetReplicaByID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetWorkspaceAgentAndLatestBuildByAuthToken", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetUserLinksByUserID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetJFrogXrayScanByWorkspaceAndAgentID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		agent := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{})

		err := db.UpsertJFrogXrayScanByWorkspaceAndAgentID(context.Background(), database.UpsertJFrogXrayScanByWorkspaceAndAgentIDParams{
			AgentID:     agent.ID,
			WorkspaceID: ws.ID,
			Critical:    1,
			High:        12,
			Medium:      14,
			ResultsUrl:  "http://hello",
		})
		require.NoError(s.T(), err)

		expect := database.JfrogXrayScan{
			WorkspaceID: ws.ID,
			AgentID:     agent.ID,
			Critical:    1,
			High:        12,
			Medium:      14,
			ResultsUrl:  "http://hello",
		}

		check.Args(database.GetJFrogXrayScanByWorkspaceAndAgentIDParams{
			WorkspaceID: ws.ID,
			AgentID:     agent.ID,
		}).Asserts(ws, policy.ActionRead).Returns(expect)
	}))
	s.Run("UpsertJFrogXrayScanByWorkspaceAndAgentID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID: tpl.ID,
		})
		check.Args(database.UpsertJFrogXrayScanByWorkspaceAndAgentIDParams{
			WorkspaceID: ws.ID,
			AgentID:     uuid.New(),
		}).Asserts(tpl, policy.ActionCreate)
	}))
	s.Run("DeleteRuntimeConfig", s.Subtest(func(db database.Store, check *expects) {
		check.Args("test").Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("GetRuntimeConfig", s.Subtest(func(db database.Store, check *expects) {
		_ = db.UpsertRuntimeConfig(context.Background(), database.UpsertRuntimeConfigParams{
			Key:   "test",
			Value: "value",
		})
		check.Args("test").Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("UpsertRuntimeConfig", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertRuntimeConfigParams{
			Key:   "test",
			Value: "value",
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("GetFailedWorkspaceBuildsByTemplateID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetFailedWorkspaceBuildsByTemplateIDParams{
			TemplateID: uuid.New(),
			Since:      dbtime.Now(),
		}).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetNotificationReportGeneratorLogByTemplate", s.Subtest(func(db database.Store, check *expects) {
		_ = db.UpsertNotificationReportGeneratorLog(context.Background(), database.UpsertNotificationReportGeneratorLogParams{
			NotificationTemplateID: notifications.TemplateWorkspaceBuildsFailedReport,
			LastGeneratedAt:        dbtime.Now(),
		})
		check.Args(notifications.TemplateWorkspaceBuildsFailedReport).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceBuildStatsByTemplates", s.Subtest(func(db database.Store, check *expects) {
		check.Args(dbtime.Now()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("UpsertNotificationReportGeneratorLog", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertNotificationReportGeneratorLogParams{
			NotificationTemplateID: uuid.New(),
			LastGeneratedAt:        dbtime.Now(),
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("GetProvisionerJobTimingsByJobID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		t := dbgen.ProvisionerJobTimings(s.T(), db, b, 2)
		check.Args(j.ID).Asserts(w, policy.ActionRead).Returns(t)
	}))
	s.Run("GetWorkspaceAgentScriptTimingsByBuildID", s.Subtest(func(db database.Store, check *expects) {
		workspace := dbgen.Workspace(s.T(), db, database.WorkspaceTable{})
		job := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: job.ID, WorkspaceID: workspace.ID})
		resource := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{
			JobID: build.JobID,
		})
		agent := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{
			ResourceID: resource.ID,
		})
		script := dbgen.WorkspaceAgentScript(s.T(), db, database.WorkspaceAgentScript{
			WorkspaceAgentID: agent.ID,
		})
		timing := dbgen.WorkspaceAgentScriptTiming(s.T(), db, database.WorkspaceAgentScriptTiming{
			ScriptID: script.ID,
		})
		rows := []database.GetWorkspaceAgentScriptTimingsByBuildIDRow{
			{
				StartedAt:          timing.StartedAt,
				EndedAt:            timing.EndedAt,
				Stage:              timing.Stage,
				ScriptID:           timing.ScriptID,
				ExitCode:           timing.ExitCode,
				Status:             timing.Status,
				DisplayName:        script.DisplayName,
				WorkspaceAgentID:   agent.ID,
				WorkspaceAgentName: agent.Name,
			},
		}
		check.Args(build.ID).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(rows)
	}))
	s.Run("InsertWorkspaceModule", s.Subtest(func(db database.Store, check *expects) {
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		check.Args(database.InsertWorkspaceModuleParams{
			JobID:      j.ID,
			Transition: database.WorkspaceTransitionStart,
		}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("GetWorkspaceModulesByJobID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceModulesCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		check.Args(dbtime.Now()).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestNotifications() {
	// System functions
	s.Run("AcquireNotificationMessages", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.AcquireNotificationMessagesParams{}).Asserts(rbac.ResourceNotificationMessage, policy.ActionUpdate)
	}))
	s.Run("BulkMarkNotificationMessagesFailed", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.BulkMarkNotificationMessagesFailedParams{}).Asserts(rbac.ResourceNotificationMessage, policy.ActionUpdate)
	}))
	s.Run("BulkMarkNotificationMessagesSent", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.BulkMarkNotificationMessagesSentParams{}).Asserts(rbac.ResourceNotificationMessage, policy.ActionUpdate)
	}))
	s.Run("DeleteOldNotificationMessages", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceNotificationMessage, policy.ActionDelete)
	}))
	s.Run("EnqueueNotificationMessage", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.EnqueueNotificationMessageParams{
			Method:  database.NotificationMethodWebhook,
			Payload: []byte("{}"),
		}).Asserts(rbac.ResourceNotificationMessage, policy.ActionCreate)
	}))
	s.Run("FetchNewMessageMetadata", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.FetchNewMessageMetadataParams{UserID: u.ID}).Asserts(rbac.ResourceNotificationMessage, policy.ActionRead)
	}))
	s.Run("GetNotificationMessagesByStatus", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.GetNotificationMessagesByStatusParams{
			Status: database.NotificationMessageStatusLeased,
			Limit:  10,
		}).Asserts(rbac.ResourceNotificationMessage, policy.ActionRead)
	}))

	// Notification templates
	s.Run("GetNotificationTemplateByID", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		check.Args(user.ID).Asserts(rbac.ResourceNotificationTemplate, policy.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetNotificationTemplatesByKind", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.NotificationTemplateKindSystem).
			Asserts().
			Errors(dbmem.ErrUnimplemented)

		// TODO(dannyk): add support for other database.NotificationTemplateKind types once implemented.
	}))
	s.Run("UpdateNotificationTemplateMethodByID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpdateNotificationTemplateMethodByIDParams{
			Method: database.NullNotificationMethod{NotificationMethod: database.NotificationMethodWebhook, Valid: true},
			ID:     notifications.TemplateWorkspaceDormant,
		}).Asserts(rbac.ResourceNotificationTemplate, policy.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))

	// Notification preferences
	s.Run("GetUserNotificationPreferences", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		check.Args(user.ID).
			Asserts(rbac.ResourceNotificationPreference.WithOwner(user.ID.String()), policy.ActionRead)
	}))
	s.Run("UpdateUserNotificationPreferences", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserNotificationPreferencesParams{
			UserID:                  user.ID,
			NotificationTemplateIds: []uuid.UUID{notifications.TemplateWorkspaceAutoUpdated, notifications.TemplateWorkspaceDeleted},
			Disableds:               []bool{true, false},
		}).Asserts(rbac.ResourceNotificationPreference.WithOwner(user.ID.String()), policy.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestOAuth2ProviderApps() {
	s.Run("GetOAuth2ProviderApps", s.Subtest(func(db database.Store, check *expects) {
		apps := []database.OAuth2ProviderApp{
			dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{Name: "first"}),
			dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{Name: "last"}),
		}
		check.Args().Asserts(rbac.ResourceOauth2App, policy.ActionRead).Returns(apps)
	}))
	s.Run("GetOAuth2ProviderAppByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(app.ID).Asserts(rbac.ResourceOauth2App, policy.ActionRead).Returns(app)
	}))
	s.Run("GetOAuth2ProviderAppsByUserID", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			UserID: user.ID,
		})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		_ = dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		for i := 0; i < 5; i++ {
			_ = dbgen.OAuth2ProviderAppToken(s.T(), db, database.OAuth2ProviderAppToken{
				AppSecretID: secret.ID,
				APIKeyID:    key.ID,
			})
		}
		check.Args(user.ID).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionRead).Returns([]database.GetOAuth2ProviderAppsByUserIDRow{
			{
				OAuth2ProviderApp: database.OAuth2ProviderApp{
					ID:          app.ID,
					CallbackURL: app.CallbackURL,
					Icon:        app.Icon,
					Name:        app.Name,
				},
				TokenCount: 5,
			},
		})
	}))
	s.Run("InsertOAuth2ProviderApp", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertOAuth2ProviderAppParams{}).Asserts(rbac.ResourceOauth2App, policy.ActionCreate)
	}))
	s.Run("UpdateOAuth2ProviderAppByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		app.Name = "my-new-name"
		app.UpdatedAt = time.Now()
		check.Args(database.UpdateOAuth2ProviderAppByIDParams{
			ID:          app.ID,
			Name:        app.Name,
			CallbackURL: app.CallbackURL,
			UpdatedAt:   app.UpdatedAt,
		}).Asserts(rbac.ResourceOauth2App, policy.ActionUpdate).Returns(app)
	}))
	s.Run("DeleteOAuth2ProviderAppByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(app.ID).Asserts(rbac.ResourceOauth2App, policy.ActionDelete)
	}))
}

func (s *MethodTestSuite) TestOAuth2ProviderAppSecrets() {
	s.Run("GetOAuth2ProviderAppSecretsByAppID", s.Subtest(func(db database.Store, check *expects) {
		app1 := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		app2 := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secrets := []database.OAuth2ProviderAppSecret{
			dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
				AppID:     app1.ID,
				CreatedAt: time.Now().Add(-time.Hour), // For ordering.
			}),
			dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
				AppID: app1.ID,
			}),
		}
		_ = dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app2.ID,
		})
		check.Args(app1.ID).Asserts(rbac.ResourceOauth2AppSecret, policy.ActionRead).Returns(secrets)
	}))
	s.Run("GetOAuth2ProviderAppSecretByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		check.Args(secret.ID).Asserts(rbac.ResourceOauth2AppSecret, policy.ActionRead).Returns(secret)
	}))
	s.Run("GetOAuth2ProviderAppSecretByPrefix", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		check.Args(secret.SecretPrefix).Asserts(rbac.ResourceOauth2AppSecret, policy.ActionRead).Returns(secret)
	}))
	s.Run("InsertOAuth2ProviderAppSecret", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(database.InsertOAuth2ProviderAppSecretParams{
			AppID: app.ID,
		}).Asserts(rbac.ResourceOauth2AppSecret, policy.ActionCreate)
	}))
	s.Run("UpdateOAuth2ProviderAppSecretByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		secret.LastUsedAt = sql.NullTime{Time: time.Now(), Valid: true}
		check.Args(database.UpdateOAuth2ProviderAppSecretByIDParams{
			ID:         secret.ID,
			LastUsedAt: secret.LastUsedAt,
		}).Asserts(rbac.ResourceOauth2AppSecret, policy.ActionUpdate).Returns(secret)
	}))
	s.Run("DeleteOAuth2ProviderAppSecretByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		check.Args(secret.ID).Asserts(rbac.ResourceOauth2AppSecret, policy.ActionDelete)
	}))
}

func (s *MethodTestSuite) TestOAuth2ProviderAppCodes() {
	s.Run("GetOAuth2ProviderAppCodeByID", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		code := dbgen.OAuth2ProviderAppCode(s.T(), db, database.OAuth2ProviderAppCode{
			AppID:  app.ID,
			UserID: user.ID,
		})
		check.Args(code.ID).Asserts(code, policy.ActionRead).Returns(code)
	}))
	s.Run("GetOAuth2ProviderAppCodeByPrefix", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		code := dbgen.OAuth2ProviderAppCode(s.T(), db, database.OAuth2ProviderAppCode{
			AppID:  app.ID,
			UserID: user.ID,
		})
		check.Args(code.SecretPrefix).Asserts(code, policy.ActionRead).Returns(code)
	}))
	s.Run("InsertOAuth2ProviderAppCode", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(database.InsertOAuth2ProviderAppCodeParams{
			AppID:  app.ID,
			UserID: user.ID,
		}).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionCreate)
	}))
	s.Run("DeleteOAuth2ProviderAppCodeByID", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		code := dbgen.OAuth2ProviderAppCode(s.T(), db, database.OAuth2ProviderAppCode{
			AppID:  app.ID,
			UserID: user.ID,
		})
		check.Args(code.ID).Asserts(code, policy.ActionDelete)
	}))
	s.Run("DeleteOAuth2ProviderAppCodesByAppAndUserID", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		for i := 0; i < 5; i++ {
			_ = dbgen.OAuth2ProviderAppCode(s.T(), db, database.OAuth2ProviderAppCode{
				AppID:  app.ID,
				UserID: user.ID,
			})
		}
		check.Args(database.DeleteOAuth2ProviderAppCodesByAppAndUserIDParams{
			AppID:  app.ID,
			UserID: user.ID,
		}).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionDelete)
	}))
}

func (s *MethodTestSuite) TestOAuth2ProviderAppTokens() {
	s.Run("InsertOAuth2ProviderAppToken", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			UserID: user.ID,
		})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		check.Args(database.InsertOAuth2ProviderAppTokenParams{
			AppSecretID: secret.ID,
			APIKeyID:    key.ID,
		}).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionCreate)
	}))
	s.Run("GetOAuth2ProviderAppTokenByPrefix", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			UserID: user.ID,
		})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		token := dbgen.OAuth2ProviderAppToken(s.T(), db, database.OAuth2ProviderAppToken{
			AppSecretID: secret.ID,
			APIKeyID:    key.ID,
		})
		check.Args(token.HashPrefix).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionRead)
	}))
	s.Run("DeleteOAuth2ProviderAppTokensByAppAndUserID", s.Subtest(func(db database.Store, check *expects) {
		user := dbgen.User(s.T(), db, database.User{})
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			UserID: user.ID,
		})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		for i := 0; i < 5; i++ {
			_ = dbgen.OAuth2ProviderAppToken(s.T(), db, database.OAuth2ProviderAppToken{
				AppSecretID: secret.ID,
				APIKeyID:    key.ID,
			})
		}
		check.Args(database.DeleteOAuth2ProviderAppTokensByAppAndUserIDParams{
			AppID:  app.ID,
			UserID: user.ID,
		}).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionDelete)
	}))
}
