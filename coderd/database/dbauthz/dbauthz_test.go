package dbauthz_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

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
		Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: xerrors.New("custom error")},
	}, slog.Make(), coderdtest.AccessControlStorePointer())
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}

	w := dbgen.Workspace(t, db, database.Workspace{})
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
		exp = dbgen.Workspace(t, db, database.Workspace{})
		rec = &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
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
	require.Equal(t, exp, w, "must be equal")

	rec.AssertActor(t, subj, rec.Pair(rbac.ActionRead, exp))
	require.NoError(t, rec.AllAsserted(), "should only be 1 rbac call")
}

// TestDBAuthzRecursive is a simple test to search for infinite recursion
// bugs. It isn't perfect, and only catches a subset of the possible bugs
// as only the first db call will be made. But it is better than nothing.
func TestDBAuthzRecursive(t *testing.T) {
	t.Parallel()
	q := dbauthz.New(dbmem.New(), &coderdtest.RecordingAuthorizer{
		Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
	}, slog.Make(), coderdtest.AccessControlStorePointer())
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{rbac.RoleOwner()},
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
		if method.Name == "InTx" || method.Name == "Ping" || method.Name == "Wrappers" {
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
	s.Run("DeleteApplicationConnectAPIKeysByUserID", s.Subtest(func(db database.Store, check *expects) {
		a, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			Scope: database.APIKeyScopeApplicationConnect,
		})
		check.Args(a.UserID).Asserts(rbac.ResourceAPIKey.WithOwner(a.UserID.String()), rbac.ActionDelete).Returns()
	}))
	s.Run("DeleteExternalAuthLink", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		check.Args(database.DeleteExternalAuthLinkParams{
			ProviderID: a.ProviderID,
			UserID:     a.UserID,
		}).Asserts(a, rbac.ActionDelete).Returns()
	}))
	s.Run("GetExternalAuthLinksByUserID", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		b := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{
			UserID: a.UserID,
		})
		check.Args(a.UserID).Asserts(a, rbac.ActionRead, b, rbac.ActionRead)
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
		}).Asserts(v.RBACObject(tpl), rbac.ActionUpdate)
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
		}).Asserts(v.RBACObject(tpl), rbac.ActionUpdate)
	}))
	s.Run("Build/GetProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(j.ID).Asserts(w, rbac.ActionRead).Returns(j)
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
		check.Args(j.ID).Asserts(v.RBACObject(tpl), rbac.ActionRead).Returns(j)
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
		check.Args(j.ID).Asserts(v.RBACObject(tpl), rbac.ActionRead).Returns(j)
	}))
	s.Run("Build/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{AllowUserCancelWorkspaceJobs: true})
		w := dbgen.Workspace(s.T(), db, database.Workspace{TemplateID: tpl.ID})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).Asserts(w, rbac.ActionUpdate).Returns()
	}))
	s.Run("BuildFalseCancel/UpdateProvisionerJobWithCancelByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{AllowUserCancelWorkspaceJobs: false})
		w := dbgen.Workspace(s.T(), db, database.Workspace{TemplateID: tpl.ID})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: w.ID})
		check.Args(database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}).Asserts(w, rbac.ActionUpdate).Returns()
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
			Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
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
			Asserts(v.RBACObjectNoTemplate(), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
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
			Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionUpdate}).Returns()
	}))
	s.Run("GetProvisionerJobsByIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		b := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args([]uuid.UUID{a.ID, b.ID}).Asserts().Returns(slice.New(a, b))
	}))
	s.Run("GetProvisionerLogsAfterID", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
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
			Asserts(rbac.ResourceUserObject(ma.UserID), rbac.ActionRead, rbac.ResourceUserObject(mb.UserID), rbac.ActionRead)
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
	s.Run("GetWorkspaceProxyByName", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(p.Name).Asserts(p, rbac.ActionRead).Returns(p)
	}))
	s.Run("UpdateWorkspaceProxyDeleted", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(database.UpdateWorkspaceProxyDeletedParams{
			ID:      p.ID,
			Deleted: true,
		}).Asserts(p, rbac.ActionDelete)
	}))
	s.Run("UpdateWorkspaceProxy", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{})
		check.Args(database.UpdateWorkspaceProxyParams{
			ID: p.ID,
		}).Asserts(p, rbac.ActionUpdate)
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
		check.Args(t1.ID).Asserts(t1, rbac.ActionUpdate)
	}))
	s.Run("GetTemplateUserRoles", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(t1.ID).Asserts(t1, rbac.ActionUpdate)
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
		}).Asserts(t1, rbac.ActionCreate)
	}))
	s.Run("UpdateTemplateAccessControlByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateAccessControlByIDParams{
			ID: t1.ID,
		}).Asserts(t1, rbac.ActionUpdate)
	}))
	s.Run("UpdateTemplateScheduleByID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateScheduleByIDParams{
			ID: t1.ID,
		}).Asserts(t1, rbac.ActionUpdate)
	}))
	s.Run("UpdateTemplateWorkspacesLastUsedAt", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateTemplateWorkspacesLastUsedAtParams{
			TemplateID: t1.ID,
		}).Asserts(t1, rbac.ActionUpdate)
	}))
	s.Run("UpdateWorkspacesDormantDeletingAtByTemplateID", s.Subtest(func(db database.Store, check *expects) {
		t1 := dbgen.Template(s.T(), db, database.Template{})
		check.Args(database.UpdateWorkspacesDormantDeletingAtByTemplateIDParams{
			TemplateID: t1.ID,
		}).Asserts(t1, rbac.ActionUpdate)
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
		}).Asserts(t1, rbac.ActionUpdate)
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
	s.Run("UpdateTemplateVersionExternalAuthProvidersByJobID", s.Subtest(func(db database.Store, check *expects) {
		jobID := uuid.New()
		t1 := dbgen.Template(s.T(), db, database.Template{})
		_ = dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true},
			JobID:      jobID,
		})
		check.Args(database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams{
			JobID:                 jobID,
			ExternalAuthProviders: []string{},
		}).Asserts(t1, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetTemplateInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateInsightsParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
	}))
	s.Run("GetUserLatencyInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetUserLatencyInsightsParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
	}))
	s.Run("GetUserActivityInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetUserActivityInsightsParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
	}))
	s.Run("GetTemplateParameterInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateParameterInsightsParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
	}))
	s.Run("GetTemplateInsightsByInterval", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateInsightsByIntervalParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
	}))
	s.Run("GetTemplateInsightsByTemplate", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateInsightsByTemplateParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
	}))
	s.Run("GetTemplateAppInsights", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateAppInsightsParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
	}))
	s.Run("GetTemplateAppInsightsByTemplate", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateAppInsightsByTemplateParams{}).Asserts(rbac.ResourceTemplateInsights, rbac.ActionRead)
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
		a := dbgen.User(s.T(), db, database.User{CreatedAt: dbtime.Now().Add(-time.Hour)})
		b := dbgen.User(s.T(), db, database.User{CreatedAt: dbtime.Now()})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(a, rbac.ActionRead, b, rbac.ActionRead).
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
	s.Run("UpdateUserQuietHoursSchedule", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserQuietHoursScheduleParams{
			ID: u.ID,
		}).Asserts(u.UserDataRBACObject(), rbac.ActionUpdate)
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
	s.Run("UpdateUserAppearanceSettings", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserAppearanceSettingsParams{
			ID:              u.ID,
			ThemePreference: u.ThemePreference,
			UpdatedAt:       u.UpdatedAt,
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
	s.Run("GetExternalAuthLink", s.Subtest(func(db database.Store, check *expects) {
		link := dbgen.ExternalAuthLink(s.T(), db, database.ExternalAuthLink{})
		check.Args(database.GetExternalAuthLinkParams{
			ProviderID: link.ProviderID,
			UserID:     link.UserID,
		}).Asserts(link, rbac.ActionRead).Returns(link)
	}))
	s.Run("InsertExternalAuthLink", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.InsertExternalAuthLinkParams{
			ProviderID: uuid.NewString(),
			UserID:     u.ID,
		}).Asserts(rbac.ResourceUserData.WithOwner(u.ID.String()).WithID(u.ID), rbac.ActionCreate)
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
			DebugContext:      json.RawMessage("{}"),
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
	s.Run("AllUserIDs", s.Subtest(func(db database.Store, check *expects) {
		a := dbgen.User(s.T(), db, database.User{})
		b := dbgen.User(s.T(), db, database.User{})
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead).Returns(slice.New(a.ID, b.ID))
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
	s.Run("GetWorkspaceAgentByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(agt)
	}))
	s.Run("GetWorkspaceAgentLifecycleStateByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceAgentMetadata", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
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
		}).Asserts(ws, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceAgentByInstanceID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.AuthInstanceID.String).Asserts(ws, rbac.ActionRead).Returns(agt)
	}))
	s.Run("UpdateWorkspaceAgentLifecycleStateByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agt.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateCreated,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentMetadata", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agt.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentLogOverflowByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentLogOverflowByIDParams{
			ID:             agt.ID,
			LogsOverflowed: true,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentStartupByID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
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
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceAgentLogsAfter", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.GetWorkspaceAgentLogsAfterParams{
			AgentID: agt.ID,
		}).Asserts(ws, rbac.ActionRead).Returns([]database.WorkspaceAgentLog{})
	}))
	s.Run("GetWorkspaceAppByAgentIDAndSlug", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
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
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
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
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(agt.ID).Asserts(ws, rbac.ActionRead).Returns(database.GetWorkspaceByAgentIDRow{
			Workspace:    ws,
			TemplateName: tpl.Name,
		})
	}))
	s.Run("GetWorkspaceAgentsInLatestBuildByWorkspaceID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: tpl.ID,
		})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(ws.ID).Asserts(ws, rbac.ActionRead)
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
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args(res.ID).Asserts(ws, rbac.ActionRead).Returns(res)
	}))
	s.Run("Build/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args(job.ID).Asserts(ws, rbac.ActionRead).Returns([]database.WorkspaceResource{})
	}))
	s.Run("Template/GetWorkspaceResourcesByJobID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		job := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})
		check.Args(job.ID).Asserts(v.RBACObject(tpl), []rbac.Action{rbac.ActionRead, rbac.ActionRead}).Returns([]database.WorkspaceResource{})
	}))
	s.Run("InsertWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		o := dbgen.Organization(s.T(), db, database.Organization{})
		check.Args(database.InsertWorkspaceParams{
			ID:               uuid.New(),
			OwnerID:          u.ID,
			OrganizationID:   o.ID,
			AutomaticUpdates: database.AutomaticUpdatesNever,
		}).Asserts(rbac.ResourceWorkspace.WithOwner(u.ID.String()).InOrg(o.ID), rbac.ActionCreate)
	}))
	s.Run("Start/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		t := dbgen.Template(s.T(), db, database.Template{})
		w := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: t.ID,
		})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionStart,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w.WorkspaceBuildRBAC(database.WorkspaceTransitionStart), rbac.ActionUpdate)
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
		w := dbgen.Workspace(s.T(), db, database.Workspace{
			TemplateID: t.ID,
		})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: v.ID,
		}).Asserts(
			w.WorkspaceBuildRBAC(database.WorkspaceTransitionStart), rbac.ActionUpdate,
			t, rbac.ActionUpdate,
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

		w := dbgen.Workspace(s.T(), db, database.Workspace{
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
			w.WorkspaceBuildRBAC(database.WorkspaceTransitionStart), rbac.ActionUpdate,
		)
	}))
	s.Run("Delete/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID: w.ID,
			Transition:  database.WorkspaceTransitionDelete,
			Reason:      database.BuildReasonInitiator,
		}).Asserts(w.WorkspaceBuildRBAC(database.WorkspaceTransitionDelete), rbac.ActionDelete)
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
	s.Run("UpdateWorkspaceDormantDeletingAt", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceDormantDeletingAtParams{
			ID: w.ID,
		}).Asserts(w, rbac.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceAutomaticUpdates", s.Subtest(func(db database.Store, check *expects) {
		w := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.UpdateWorkspaceAutomaticUpdatesParams{
			ID:               w.ID,
			AutomaticUpdates: database.AutomaticUpdatesAlways,
		}).Asserts(w, rbac.ActionUpdate)
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
	s.Run("UpdateWorkspaceBuildDeadlineByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		check.Args(database.UpdateWorkspaceBuildDeadlineByIDParams{
			ID:        build.ID,
			UpdatedAt: build.UpdatedAt,
			Deadline:  build.Deadline,
		}).Asserts(ws, rbac.ActionUpdate)
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
	s.Run("BatchUpdateWorkspaceLastUsedAt", s.Subtest(func(db database.Store, check *expects) {
		ws1 := dbgen.Workspace(s.T(), db, database.Workspace{})
		ws2 := dbgen.Workspace(s.T(), db, database.Workspace{})
		check.Args(database.BatchUpdateWorkspaceLastUsedAtParams{
			IDs: []uuid.UUID{ws1.ID, ws2.ID},
		}).Asserts(rbac.ResourceWorkspace.All(), rbac.ActionUpdate).Returns()
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
	s.Run("ActivityBumpWorkspace", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args(database.ActivityBumpWorkspaceParams{
			WorkspaceID: ws.ID,
		}).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("FavoriteWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{OwnerID: u.ID})
		check.Args(ws.ID).Asserts(ws, rbac.ActionUpdate).Returns()
	}))
	s.Run("UnfavoriteWorkspace", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		ws := dbgen.Workspace(s.T(), db, database.Workspace{OwnerID: u.ID})
		check.Args(ws.ID).Asserts(ws, rbac.ActionUpdate).Returns()
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
		check.Args().Asserts(d, rbac.ActionRead)
	}))
	s.Run("DeleteOldProvisionerDaemons", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionDelete)
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
		}).Asserts(rbac.ResourceProvisionerDaemon, rbac.ActionUpdate)
	}))
}

// All functions in this method test suite are not implemented in dbmem, but
// we still want to assert RBAC checks.
func (s *MethodTestSuite) TestTailnetFunctions() {
	s.Run("CleanTailnetCoordinators", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("CleanTailnetLostPeers", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("CleanTailnetTunnels", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteAllTailnetClientSubscriptions", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteAllTailnetClientSubscriptionsParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteAllTailnetTunnels", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteAllTailnetTunnelsParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteCoordinator", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetAgentParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetClient", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetClientParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetClientSubscription", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetClientSubscriptionParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetPeer", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetPeerParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("DeleteTailnetTunnel", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.DeleteTailnetTunnelParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionDelete).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetAgents", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetAgents", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetClientsForAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetPeers", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetTunnelPeerBindings", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetTailnetTunnelPeerIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetCoordinators", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetPeers", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("GetAllTailnetTunnels", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionRead).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetAgentParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetClient", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetClientParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetClientSubscription", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetClientSubscriptionParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetCoordinator", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionUpdate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetPeer", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetPeerParams{
			Status: database.TailnetStatusOk,
		}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionCreate).
			Errors(dbmem.ErrUnimplemented)
	}))
	s.Run("UpsertTailnetTunnel", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertTailnetTunnelParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, rbac.ActionCreate).
			Errors(dbmem.ErrUnimplemented)
	}))
}

func (s *MethodTestSuite) TestDBCrypt() {
	s.Run("GetDBCryptKeys", s.Subtest(func(db database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceSystem, rbac.ActionRead).
			Returns([]database.DBCryptKey{})
	}))
	s.Run("InsertDBCryptKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertDBCryptKeyParams{}).
			Asserts(rbac.ResourceSystem, rbac.ActionCreate).
			Returns()
	}))
	s.Run("RevokeDBCryptKey", s.Subtest(func(db database.Store, check *expects) {
		err := db.InsertDBCryptKey(context.Background(), database.InsertDBCryptKeyParams{
			ActiveKeyDigest: "revoke me",
		})
		s.NoError(err)
		check.Args("revoke me").
			Asserts(rbac.ResourceSystem, rbac.ActionUpdate).
			Returns()
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
		}).Asserts(rbac.ResourceSystem, rbac.ActionUpdate).Returns(l)
	}))
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID})
		check.Args([]uuid.UUID{ws.ID}).Asserts(rbac.ResourceSystem, rbac.ActionRead).Returns(slice.New(b))
	}))
	s.Run("UpsertDefaultProxy", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpsertDefaultProxyParams{}).Asserts(rbac.ResourceSystem, rbac.ActionUpdate).Returns()
	}))
	s.Run("GetUserLinkByLinkedID", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(l.LinkedID).Asserts(rbac.ResourceSystem, rbac.ActionRead).Returns(l)
	}))
	s.Run("GetUserLinkByUserIDLoginType", s.Subtest(func(db database.Store, check *expects) {
		l := dbgen.UserLink(s.T(), db, database.UserLink{})
		check.Args(database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    l.UserID,
			LoginType: l.LoginType,
		}).Asserts(rbac.ResourceSystem, rbac.ActionRead).Returns(l)
	}))
	s.Run("GetLatestWorkspaceBuilds", s.Subtest(func(db database.Store, check *expects) {
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetActiveUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead).Returns(int64(0))
	}))
	s.Run("GetUnexpiredLicenses", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetAuthorizationUserRoles", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(u.ID).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("InsertDERPMeshKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceSystem, rbac.ActionCreate).Returns()
	}))
	s.Run("InsertDeploymentID", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceSystem, rbac.ActionCreate).Returns()
	}))
	s.Run("InsertReplica", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertReplicaParams{
			ID: uuid.New(),
		}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("UpdateReplica", s.Subtest(func(db database.Store, check *expects) {
		replica, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New()})
		require.NoError(s.T(), err)
		check.Args(database.UpdateReplicaParams{
			ID:              replica.ID,
			DatabaseLatency: 100,
		}).Asserts(rbac.ResourceSystem, rbac.ActionUpdate)
	}))
	s.Run("DeleteReplicasUpdatedBefore", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour)).Asserts(rbac.ResourceSystem, rbac.ActionDelete)
	}))
	s.Run("GetReplicasUpdatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_, err := db.InsertReplica(context.Background(), database.InsertReplicaParams{ID: uuid.New(), UpdatedAt: time.Now()})
		require.NoError(s.T(), err)
		check.Args(time.Now().Add(time.Hour*-1)).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetUserCount", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead).Returns(int64(0))
	}))
	s.Run("GetTemplates", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.Template(s.T(), db, database.Template{})
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("UpdateWorkspaceBuildCostByID", s.Subtest(func(db database.Store, check *expects) {
		b := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{})
		o := b
		o.DailyCost = 10
		check.Args(database.UpdateWorkspaceBuildCostByIDParams{
			ID:        b.ID,
			DailyCost: 10,
		}).Asserts(rbac.ResourceSystem, rbac.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceBuildProvisionerStateByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		check.Args(database.UpdateWorkspaceBuildProvisionerStateByIDParams{
			ID:               build.ID,
			ProvisionerState: []byte("testing"),
		}).Asserts(rbac.ResourceSystem, rbac.ActionUpdate)
	}))
	s.Run("UpsertLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		check.Args("value").Asserts(rbac.ResourceSystem, rbac.ActionUpdate)
	}))
	s.Run("GetLastUpdateCheck", s.Subtest(func(db database.Store, check *expects) {
		err := db.UpsertLastUpdateCheck(context.Background(), "value")
		require.NoError(s.T(), err)
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceBuildsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceAgentsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceAppsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceResourcesCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceResourceMetadataCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		_ = dbgen.WorkspaceResourceMetadatums(s.T(), db, database.WorkspaceResourceMetadatum{})
		check.Args(time.Now()).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("DeleteOldWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionDelete)
	}))
	s.Run("GetProvisionerJobsCreatedAfter", s.Subtest(func(db database.Store, check *expects) {
		// TODO: add provisioner job resource type
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{CreatedAt: time.Now().Add(-time.Hour)})
		check.Args(time.Now()).Asserts( /*rbac.ResourceSystem, rbac.ActionRead*/ )
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
			Asserts(rbac.ResourceSystem, rbac.ActionRead).
			Returns(slice.New(tv1, tv2, tv3))
	}))
	s.Run("GetParameterSchemasByJobID", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true},
		})
		job := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: tv.JobID})
		check.Args(job.ID).
			Asserts(tpl, rbac.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetWorkspaceAppsByAgentIDs", s.Subtest(func(db database.Store, check *expects) {
		aWs := dbgen.Workspace(s.T(), db, database.Workspace{})
		aBuild := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: aWs.ID, JobID: uuid.New()})
		aRes := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: aBuild.JobID})
		aAgt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: aRes.ID})
		a := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: aAgt.ID})

		bWs := dbgen.Workspace(s.T(), db, database.Workspace{})
		bBuild := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: bWs.ID, JobID: uuid.New()})
		bRes := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: bBuild.JobID})
		bAgt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: bRes.ID})
		b := dbgen.WorkspaceApp(s.T(), db, database.WorkspaceApp{AgentID: bAgt.ID})

		check.Args([]uuid.UUID{a.AgentID, b.AgentID}).
			Asserts(rbac.ResourceSystem, rbac.ActionRead).
			Returns([]database.WorkspaceApp{a, b})
	}))
	s.Run("GetWorkspaceResourcesByJobIDs", s.Subtest(func(db database.Store, check *expects) {
		tpl := dbgen.Template(s.T(), db, database.Template{})
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, JobID: uuid.New()})
		tJob := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})

		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		wJob := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		check.Args([]uuid.UUID{tJob.ID, wJob.ID}).
			Asserts(rbac.ResourceSystem, rbac.ActionRead).
			Returns([]database.WorkspaceResource{})
	}))
	s.Run("GetWorkspaceResourceMetadataByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		_ = dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		a := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		b := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceAgentsByResourceIDs", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args([]uuid.UUID{res.ID}).
			Asserts(rbac.ResourceSystem, rbac.ActionRead).
			Returns([]database.WorkspaceAgent{agt})
	}))
	s.Run("GetProvisionerJobsByIDs", s.Subtest(func(db database.Store, check *expects) {
		// TODO: add a ProvisionerJob resource type
		a := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		b := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args([]uuid.UUID{a.ID, b.ID}).
			Asserts( /*rbac.ResourceSystem, rbac.ActionRead*/ ).
			Returns(slice.New(a, b))
	}))
	s.Run("InsertWorkspaceAgent", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentParams{
			ID: uuid.New(),
		}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("InsertWorkspaceApp", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAppParams{
			ID:           uuid.New(),
			Health:       database.WorkspaceAppHealthDisabled,
			SharingLevel: database.AppSharingLevelOwner,
		}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("InsertWorkspaceResourceMetadata", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceResourceMetadataParams{
			WorkspaceResourceID: uuid.New(),
		}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("UpdateWorkspaceAgentConnectionByID", s.Subtest(func(db database.Store, check *expects) {
		ws := dbgen.Workspace(s.T(), db, database.Workspace{})
		build := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: uuid.New()})
		res := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{JobID: build.JobID})
		agt := dbgen.WorkspaceAgent(s.T(), db, database.WorkspaceAgent{ResourceID: res.ID})
		check.Args(database.UpdateWorkspaceAgentConnectionByIDParams{
			ID: agt.ID,
		}).Asserts(rbac.ResourceSystem, rbac.ActionUpdate).Returns()
	}))
	s.Run("AcquireProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			StartedAt: sql.NullTime{Valid: false},
		})
		check.Args(database.AcquireProvisionerJobParams{Types: []database.ProvisionerType{j.Provisioner}, Tags: must(json.Marshal(j.Tags))}).
			Asserts( /*rbac.ResourceSystem, rbac.ActionUpdate*/ )
	}))
	s.Run("UpdateProvisionerJobWithCompleteByID", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobWithCompleteByIDParams{
			ID: j.ID,
		}).Asserts( /*rbac.ResourceSystem, rbac.ActionUpdate*/ )
	}))
	s.Run("UpdateProvisionerJobByID", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args(database.UpdateProvisionerJobByIDParams{
			ID:        j.ID,
			UpdatedAt: time.Now(),
		}).Asserts( /*rbac.ResourceSystem, rbac.ActionUpdate*/ )
	}))
	s.Run("InsertProvisionerJob", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		check.Args(database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
		}).Asserts( /*rbac.ResourceSystem, rbac.ActionCreate*/ )
	}))
	s.Run("InsertProvisionerJobLogs", s.Subtest(func(db database.Store, check *expects) {
		// TODO: we need to create a ProvisionerJob resource
		j := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{})
		check.Args(database.InsertProvisionerJobLogsParams{
			JobID: j.ID,
		}).Asserts( /*rbac.ResourceSystem, rbac.ActionCreate*/ )
	}))
	s.Run("UpsertProvisionerDaemon", s.Subtest(func(db database.Store, check *expects) {
		pd := rbac.ResourceProvisionerDaemon.All()
		check.Args(database.UpsertProvisionerDaemonParams{
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		}).Asserts(pd, rbac.ActionCreate)
		check.Args(database.UpsertProvisionerDaemonParams{
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeUser,
				provisionersdk.TagOwner: "11111111-1111-1111-1111-111111111111",
			}),
		}).Asserts(pd.WithOwner("11111111-1111-1111-1111-111111111111"), rbac.ActionCreate)
	}))
	s.Run("InsertTemplateVersionParameter", s.Subtest(func(db database.Store, check *expects) {
		v := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{})
		check.Args(database.InsertTemplateVersionParameterParams{
			TemplateVersionID: v.ID,
		}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("InsertWorkspaceResource", s.Subtest(func(db database.Store, check *expects) {
		r := dbgen.WorkspaceResource(s.T(), db, database.WorkspaceResource{})
		check.Args(database.InsertWorkspaceResourceParams{
			ID:         r.ID,
			Transition: database.WorkspaceTransitionStart,
		}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("DeleteOldWorkspaceAgentLogs", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionDelete)
	}))
	s.Run("InsertWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentStatsParams{}).Asserts(rbac.ResourceSystem, rbac.ActionCreate).Errors(errMatchAny)
	}))
	s.Run("InsertWorkspaceAppStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAppStatsParams{}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentScripts", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentScriptsParams{}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentMetadata", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentMetadataParams{}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentLogs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentLogsParams{}).Asserts()
	}))
	s.Run("InsertWorkspaceAgentLogSources", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertWorkspaceAgentLogSourcesParams{}).Asserts()
	}))
	s.Run("GetTemplateDAUs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateDAUsParams{}).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetActiveWorkspaceBuildsByTemplateID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, rbac.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetDeploymentDAUs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(int32(0)).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetAppSecurityKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("UpsertAppSecurityKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("").Asserts()
	}))
	s.Run("GetApplicationName", s.Subtest(func(db database.Store, check *expects) {
		db.UpsertApplicationName(context.Background(), "foo")
		check.Args().Asserts()
	}))
	s.Run("UpsertApplicationName", s.Subtest(func(db database.Store, check *expects) {
		check.Args("").Asserts(rbac.ResourceDeploymentValues, rbac.ActionCreate)
	}))
	s.Run("GetHealthSettings", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("UpsertHealthSettings", s.Subtest(func(db database.Store, check *expects) {
		check.Args("foo").Asserts(rbac.ResourceDeploymentValues, rbac.ActionCreate)
	}))
	s.Run("GetDeploymentWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetDeploymentWorkspaceStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args().Asserts()
	}))
	s.Run("GetFileTemplates", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetHungProvisionerJobs", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("UpsertOAuthSigningKey", s.Subtest(func(db database.Store, check *expects) {
		check.Args("foo").Asserts(rbac.ResourceSystem, rbac.ActionUpdate)
	}))
	s.Run("GetOAuthSigningKey", s.Subtest(func(db database.Store, check *expects) {
		db.UpsertOAuthSigningKey(context.Background(), "foo")
		check.Args().Asserts(rbac.ResourceSystem, rbac.ActionUpdate)
	}))
	s.Run("InsertMissingGroups", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertMissingGroupsParams{}).Asserts(rbac.ResourceSystem, rbac.ActionCreate).Errors(errMatchAny)
	}))
	s.Run("UpdateUserLoginType", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		check.Args(database.UpdateUserLoginTypeParams{
			NewLoginType: database.LoginTypePassword,
			UserID:       u.ID,
		}).Asserts(rbac.ResourceSystem, rbac.ActionUpdate)
	}))
	s.Run("GetWorkspaceAgentStatsAndLabels", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetWorkspaceAgentStats", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("GetWorkspaceProxyByHostname", s.Subtest(func(db database.Store, check *expects) {
		p, _ := dbgen.WorkspaceProxy(s.T(), db, database.WorkspaceProxy{
			WildcardHostname: "*.example.com",
		})
		check.Args(database.GetWorkspaceProxyByHostnameParams{
			Hostname:              "foo.example.com",
			AllowWildcardHostname: true,
		}).Asserts(rbac.ResourceSystem, rbac.ActionRead).Returns(p)
	}))
	s.Run("GetTemplateAverageBuildTime", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.GetTemplateAverageBuildTimeParams{}).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspacesEligibleForTransition", s.Subtest(func(db database.Store, check *expects) {
		check.Args(time.Time{}).Asserts()
	}))
	s.Run("InsertTemplateVersionVariable", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertTemplateVersionVariableParams{}).Asserts(rbac.ResourceSystem, rbac.ActionCreate)
	}))
	s.Run("UpdateInactiveUsersToDormant", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.UpdateInactiveUsersToDormantParams{}).Asserts(rbac.ResourceSystem, rbac.ActionCreate).Errors(sql.ErrNoRows)
	}))
	s.Run("GetWorkspaceUniqueOwnerCountByTemplateIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{uuid.New()}).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceAgentScriptsByAgentIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{uuid.New()}).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetWorkspaceAgentLogSourcesByAgentIDs", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{uuid.New()}).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
	s.Run("GetProvisionerJobsByIDsWithQueuePosition", s.Subtest(func(db database.Store, check *expects) {
		check.Args([]uuid.UUID{}).Asserts()
	}))
	s.Run("GetReplicaByID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, rbac.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetWorkspaceAgentAndOwnerByAuthToken", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, rbac.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetUserLinksByUserID", s.Subtest(func(db database.Store, check *expects) {
		check.Args(uuid.New()).Asserts(rbac.ResourceSystem, rbac.ActionRead)
	}))
}

func (s *MethodTestSuite) TestOAuth2ProviderApps() {
	s.Run("GetOAuth2ProviderApps", s.Subtest(func(db database.Store, check *expects) {
		apps := []database.OAuth2ProviderApp{
			dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{Name: "first"}),
			dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{Name: "last"}),
		}
		check.Args().Asserts(rbac.ResourceOAuth2ProviderApp, rbac.ActionRead).Returns(apps)
	}))
	s.Run("GetOAuth2ProviderAppByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(app.ID).Asserts(rbac.ResourceOAuth2ProviderApp, rbac.ActionRead).Returns(app)
	}))
	s.Run("InsertOAuth2ProviderApp", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertOAuth2ProviderAppParams{}).Asserts(rbac.ResourceOAuth2ProviderApp, rbac.ActionCreate)
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
		}).Asserts(rbac.ResourceOAuth2ProviderApp, rbac.ActionUpdate).Returns(app)
	}))
	s.Run("DeleteOAuth2ProviderAppByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(app.ID).Asserts(rbac.ResourceOAuth2ProviderApp, rbac.ActionDelete)
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
		check.Args(app1.ID).Asserts(rbac.ResourceOAuth2ProviderAppSecret, rbac.ActionRead).Returns(secrets)
	}))
	s.Run("GetOAuth2ProviderAppSecretByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		check.Args(secret.ID).Asserts(rbac.ResourceOAuth2ProviderAppSecret, rbac.ActionRead).Returns(secret)
	}))
	s.Run("InsertOAuth2ProviderAppSecret", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(database.InsertOAuth2ProviderAppSecretParams{
			AppID: app.ID,
		}).Asserts(rbac.ResourceOAuth2ProviderAppSecret, rbac.ActionCreate)
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
		}).Asserts(rbac.ResourceOAuth2ProviderAppSecret, rbac.ActionUpdate).Returns(secret)
	}))
	s.Run("DeleteOAuth2ProviderAppSecretByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		check.Args(secret.ID).Asserts(rbac.ResourceOAuth2ProviderAppSecret, rbac.ActionDelete)
	}))
}
