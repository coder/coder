package dbauthz_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
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

	db := dbmock.NewMockStore(gomock.NewController(t))
	db.EXPECT().Wrappers().Times(1).Return([]string{})
	db.EXPECT().Ping(gomock.Any()).Times(1).Return(time.Second, nil)
	q := dbauthz.New(db, &coderdtest.RecordingAuthorizer{}, slog.Make(), coderdtest.AccessControlStorePointer())
	_, err := q.Ping(context.Background())
	require.NoError(t, err, "must not error")
}

// TestInTX is not perfect, just checks that it properly checks auth.
func TestInTX(t *testing.T) {
	t.Parallel()

	var (
		ctrl  = gomock.NewController(t)
		db    = dbmock.NewMockStore(ctrl)
		mTx   = dbmock.NewMockStore(ctrl) // to record the 'in tx' calls
		faker = gofakeit.New(0)
		w     = testutil.Fake(t, faker, database.Workspace{})
		actor = rbac.Subject{
			ID:     uuid.NewString(),
			Roles:  rbac.RoleIdentifiers{rbac.RoleOwner()},
			Groups: []string{},
			Scope:  rbac.ScopeAll,
		}
		ctx = dbauthz.As(context.Background(), actor)
	)

	db.EXPECT().Wrappers().Times(1).Return([]string{}) // called by dbauthz.New
	q := dbauthz.New(db, &coderdtest.RecordingAuthorizer{
		Wrapped: (&coderdtest.FakeAuthorizer{}).AlwaysReturn(xerrors.New("custom error")),
	}, slog.Make(), coderdtest.AccessControlStorePointer())

	db.EXPECT().InTx(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
		func(f func(database.Store) error, _ *database.TxOptions) error {
			return f(mTx)
		},
	)
	mTx.EXPECT().Wrappers().Times(1).Return([]string{})
	mTx.EXPECT().GetWorkspaceByID(gomock.Any(), gomock.Any()).Times(1).Return(w, nil)
	err := q.InTx(func(tx database.Store) error {
		// The inner tx should use the parent's authz
		_, err := tx.GetWorkspaceByID(ctx, w.ID)
		return err
	}, nil)
	require.ErrorContains(t, err, "custom error", "must be our custom error")
	require.ErrorAs(t, err, &dbauthz.NotAuthorizedError{}, "must be an authorized error")
	require.True(t, dbauthz.IsNotAuthorizedError(err), "must be an authorized error")
}

// TestNew should not double wrap a querier.
func TestNew(t *testing.T) {
	t.Parallel()

	var (
		ctrl  = gomock.NewController(t)
		db    = dbmock.NewMockStore(ctrl)
		faker = gofakeit.New(0)
		rec   = &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		subj = rbac.Subject{}
		ctx  = dbauthz.As(context.Background(), rbac.Subject{})
	)
	db.EXPECT().Wrappers().Times(1).Return([]string{}).Times(2) // two calls to New()
	exp := testutil.Fake(t, faker, database.Workspace{})
	db.EXPECT().GetWorkspaceByID(gomock.Any(), exp.ID).Times(1).Return(exp, nil)
	// Double wrap should not cause an actual double wrap. So only 1 rbac call
	// should be made.
	az := dbauthz.New(db, rec, slog.Make(), coderdtest.AccessControlStorePointer())
	az = dbauthz.New(az, rec, slog.Make(), coderdtest.AccessControlStorePointer())

	w, err := az.GetWorkspaceByID(ctx, exp.ID)
	require.NoError(t, err, "must not error")
	require.Equal(t, exp, w, "must be equal")

	rec.AssertActor(t, subj, rec.Pair(policy.ActionRead, exp))
	require.NoError(t, rec.AllAsserted(), "should only be 1 rbac call")
}

// TestDBAuthzRecursive is a simple test to search for infinite recursion
// bugs. It isn't perfect, and only catches a subset of the possible bugs
// as only the first db call will be made. But it is better than nothing.
// This can be removed when all tests in this package are migrated to
// dbmock as it will immediately detect recursive calls.
func TestDBAuthzRecursive(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	q := dbauthz.New(db, &coderdtest.RecordingAuthorizer{
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

func defaultIPAddress() pqtype.Inet {
	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   net.IPv4(127, 0, 0, 1),
			Mask: net.IPv4Mask(255, 255, 255, 255),
		},
		Valid: true,
	}
}

func (s *MethodTestSuite) TestAPIKey() {
	s.Run("DeleteAPIKeyByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.APIKey{})
		dbm.EXPECT().GetAPIKeyByID(gomock.Any(), key.ID).Return(key, nil).AnyTimes()
		dbm.EXPECT().DeleteAPIKeyByID(gomock.Any(), key.ID).Return(nil).AnyTimes()
		check.Args(key.ID).Asserts(key, policy.ActionDelete).Returns()
	}))
	s.Run("GetAPIKeyByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.APIKey{})
		dbm.EXPECT().GetAPIKeyByID(gomock.Any(), key.ID).Return(key, nil).AnyTimes()
		check.Args(key.ID).Asserts(key, policy.ActionRead).Returns(key)
	}))
	s.Run("GetAPIKeyByName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.APIKey{LoginType: database.LoginTypeToken, TokenName: "marge-cat"})
		dbm.EXPECT().GetAPIKeyByName(gomock.Any(), database.GetAPIKeyByNameParams{TokenName: key.TokenName, UserID: key.UserID}).Return(key, nil).AnyTimes()
		check.Args(database.GetAPIKeyByNameParams{TokenName: key.TokenName, UserID: key.UserID}).Asserts(key, policy.ActionRead).Returns(key)
	}))
	s.Run("GetAPIKeysByLoginType", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.APIKey{LoginType: database.LoginTypePassword})
		b := testutil.Fake(s.T(), faker, database.APIKey{LoginType: database.LoginTypePassword})
		dbm.EXPECT().GetAPIKeysByLoginType(gomock.Any(), database.LoginTypePassword).Return([]database.APIKey{a, b}, nil).AnyTimes()
		check.Args(database.LoginTypePassword).Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetAPIKeysByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u1 := testutil.Fake(s.T(), faker, database.User{})
		keyA := testutil.Fake(s.T(), faker, database.APIKey{UserID: u1.ID, LoginType: database.LoginTypeToken, TokenName: "key-a"})
		keyB := testutil.Fake(s.T(), faker, database.APIKey{UserID: u1.ID, LoginType: database.LoginTypeToken, TokenName: "key-b"})

		dbm.EXPECT().GetAPIKeysByUserID(gomock.Any(), gomock.Any()).Return(slice.New(keyA, keyB), nil).AnyTimes()
		check.Args(database.GetAPIKeysByUserIDParams{LoginType: database.LoginTypeToken, UserID: u1.ID}).
			Asserts(keyA, policy.ActionRead, keyB, policy.ActionRead).
			Returns(slice.New(keyA, keyB))
	}))
	s.Run("GetAPIKeysLastUsedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		now := time.Now()
		a := database.APIKey{LastUsed: now.Add(time.Hour)}
		b := database.APIKey{LastUsed: now.Add(time.Hour)}
		dbm.EXPECT().GetAPIKeysLastUsedAfter(gomock.Any(), gomock.Any()).Return([]database.APIKey{a, b}, nil).AnyTimes()
		check.Args(now).Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("InsertAPIKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.InsertAPIKeyParams{UserID: u.ID, LoginType: database.LoginTypePassword, Scopes: database.APIKeyScopes{database.ApiKeyScopeCoderAll}, IPAddress: defaultIPAddress()}
		ret := testutil.Fake(s.T(), faker, database.APIKey{UserID: u.ID, LoginType: database.LoginTypePassword})
		dbm.EXPECT().InsertAPIKey(gomock.Any(), arg).Return(ret, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceApiKey.WithOwner(u.ID.String()), policy.ActionCreate)
	}))
	s.Run("UpdateAPIKeyByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		a := testutil.Fake(s.T(), faker, database.APIKey{UserID: u.ID, IPAddress: defaultIPAddress()})
		arg := database.UpdateAPIKeyByIDParams{ID: a.ID, IPAddress: defaultIPAddress(), LastUsed: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
		dbm.EXPECT().GetAPIKeyByID(gomock.Any(), a.ID).Return(a, nil).AnyTimes()
		dbm.EXPECT().UpdateAPIKeyByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(a, policy.ActionUpdate).Returns()
	}))
	s.Run("DeleteApplicationConnectAPIKeysByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.APIKey{Scopes: database.APIKeyScopes{database.ApiKeyScopeCoderApplicationConnect}})
		dbm.EXPECT().DeleteApplicationConnectAPIKeysByUserID(gomock.Any(), a.UserID).Return(nil).AnyTimes()
		check.Args(a.UserID).Asserts(rbac.ResourceApiKey.WithOwner(a.UserID.String()), policy.ActionDelete).Returns()
	}))
	s.Run("DeleteExternalAuthLink", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.ExternalAuthLink{})
		dbm.EXPECT().GetExternalAuthLink(gomock.Any(), database.GetExternalAuthLinkParams{ProviderID: a.ProviderID, UserID: a.UserID}).Return(a, nil).AnyTimes()
		dbm.EXPECT().DeleteExternalAuthLink(gomock.Any(), database.DeleteExternalAuthLinkParams{ProviderID: a.ProviderID, UserID: a.UserID}).Return(nil).AnyTimes()
		check.Args(database.DeleteExternalAuthLinkParams{ProviderID: a.ProviderID, UserID: a.UserID}).Asserts(a, policy.ActionUpdatePersonal).Returns()
	}))
	s.Run("GetExternalAuthLinksByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.ExternalAuthLink{})
		b := testutil.Fake(s.T(), faker, database.ExternalAuthLink{UserID: a.UserID})
		dbm.EXPECT().GetExternalAuthLinksByUserID(gomock.Any(), a.UserID).Return([]database.ExternalAuthLink{a, b}, nil).AnyTimes()
		check.Args(a.UserID).Asserts(a, policy.ActionReadPersonal, b, policy.ActionReadPersonal)
	}))
}

func (s *MethodTestSuite) TestAuditLogs() {
	s.Run("InsertAuditLog", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertAuditLogParams{ResourceType: database.ResourceTypeOrganization, Action: database.AuditActionCreate, Diff: json.RawMessage("{}"), AdditionalFields: json.RawMessage("{}")}
		dbm.EXPECT().InsertAuditLog(gomock.Any(), arg).Return(database.AuditLog{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceAuditLog, policy.ActionCreate)
	}))
	s.Run("GetAuditLogsOffset", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetAuditLogsOffsetParams{LimitOpt: 10}
		dbm.EXPECT().GetAuditLogsOffset(gomock.Any(), arg).Return([]database.GetAuditLogsOffsetRow{}, nil).AnyTimes()
		dbm.EXPECT().GetAuthorizedAuditLogsOffset(gomock.Any(), arg, gomock.Any()).Return([]database.GetAuditLogsOffsetRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceAuditLog, policy.ActionRead).WithNotAuthorized("nil")
	}))
	s.Run("GetAuthorizedAuditLogsOffset", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetAuditLogsOffsetParams{LimitOpt: 10}
		dbm.EXPECT().GetAuthorizedAuditLogsOffset(gomock.Any(), arg, gomock.Any()).Return([]database.GetAuditLogsOffsetRow{}, nil).AnyTimes()
		dbm.EXPECT().GetAuditLogsOffset(gomock.Any(), arg).Return([]database.GetAuditLogsOffsetRow{}, nil).AnyTimes()
		check.Args(arg, emptyPreparedAuthorized{}).Asserts(rbac.ResourceAuditLog, policy.ActionRead)
	}))
	s.Run("CountAuditLogs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().CountAuditLogs(gomock.Any(), database.CountAuditLogsParams{}).Return(int64(0), nil).AnyTimes()
		dbm.EXPECT().CountAuthorizedAuditLogs(gomock.Any(), database.CountAuditLogsParams{}, gomock.Any()).Return(int64(0), nil).AnyTimes()
		check.Args(database.CountAuditLogsParams{}).Asserts(rbac.ResourceAuditLog, policy.ActionRead).WithNotAuthorized("nil")
	}))
	s.Run("CountAuthorizedAuditLogs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().CountAuthorizedAuditLogs(gomock.Any(), database.CountAuditLogsParams{}, gomock.Any()).Return(int64(0), nil).AnyTimes()
		dbm.EXPECT().CountAuditLogs(gomock.Any(), database.CountAuditLogsParams{}).Return(int64(0), nil).AnyTimes()
		check.Args(database.CountAuditLogsParams{}, emptyPreparedAuthorized{}).Asserts(rbac.ResourceAuditLog, policy.ActionRead)
	}))
	s.Run("DeleteOldAuditLogConnectionEvents", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().DeleteOldAuditLogConnectionEvents(gomock.Any(), database.DeleteOldAuditLogConnectionEventsParams{}).Return(nil).AnyTimes()
		check.Args(database.DeleteOldAuditLogConnectionEventsParams{}).Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
}

func (s *MethodTestSuite) TestConnectionLogs() {
	s.Run("UpsertConnectionLog", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.WorkspaceTable{})
		arg := database.UpsertConnectionLogParams{Ip: defaultIPAddress(), Type: database.ConnectionTypeSsh, WorkspaceID: ws.ID, OrganizationID: ws.OrganizationID, ConnectionStatus: database.ConnectionStatusConnected, WorkspaceOwnerID: ws.OwnerID}
		dbm.EXPECT().UpsertConnectionLog(gomock.Any(), arg).Return(database.ConnectionLog{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceConnectionLog, policy.ActionUpdate)
	}))
	s.Run("GetConnectionLogsOffset", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetConnectionLogsOffsetParams{LimitOpt: 10}
		dbm.EXPECT().GetConnectionLogsOffset(gomock.Any(), arg).Return([]database.GetConnectionLogsOffsetRow{}, nil).AnyTimes()
		dbm.EXPECT().GetAuthorizedConnectionLogsOffset(gomock.Any(), arg, gomock.Any()).Return([]database.GetConnectionLogsOffsetRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceConnectionLog, policy.ActionRead).WithNotAuthorized("nil")
	}))
	s.Run("GetAuthorizedConnectionLogsOffset", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetConnectionLogsOffsetParams{LimitOpt: 10}
		dbm.EXPECT().GetAuthorizedConnectionLogsOffset(gomock.Any(), arg, gomock.Any()).Return([]database.GetConnectionLogsOffsetRow{}, nil).AnyTimes()
		dbm.EXPECT().GetConnectionLogsOffset(gomock.Any(), arg).Return([]database.GetConnectionLogsOffsetRow{}, nil).AnyTimes()
		check.Args(arg, emptyPreparedAuthorized{}).Asserts(rbac.ResourceConnectionLog, policy.ActionRead)
	}))
	s.Run("CountConnectionLogs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().CountConnectionLogs(gomock.Any(), database.CountConnectionLogsParams{}).Return(int64(0), nil).AnyTimes()
		dbm.EXPECT().CountAuthorizedConnectionLogs(gomock.Any(), database.CountConnectionLogsParams{}, gomock.Any()).Return(int64(0), nil).AnyTimes()
		check.Args(database.CountConnectionLogsParams{}).Asserts(rbac.ResourceConnectionLog, policy.ActionRead).WithNotAuthorized("nil")
	}))
	s.Run("CountAuthorizedConnectionLogs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().CountAuthorizedConnectionLogs(gomock.Any(), database.CountConnectionLogsParams{}, gomock.Any()).Return(int64(0), nil).AnyTimes()
		dbm.EXPECT().CountConnectionLogs(gomock.Any(), database.CountConnectionLogsParams{}).Return(int64(0), nil).AnyTimes()
		check.Args(database.CountConnectionLogsParams{}, emptyPreparedAuthorized{}).Asserts(rbac.ResourceConnectionLog, policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestFile() {
	s.Run("GetFileByHashAndCreator", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		f := testutil.Fake(s.T(), faker, database.File{})
		dbm.EXPECT().GetFileByHashAndCreator(gomock.Any(), gomock.Any()).Return(f, nil).AnyTimes()
		// dbauthz may attempt to check template access on NotAuthorized; ensure mock handles it.
		dbm.EXPECT().GetFileTemplates(gomock.Any(), f.ID).Return([]database.GetFileTemplatesRow{}, nil).AnyTimes()
		check.Args(database.GetFileByHashAndCreatorParams{
			Hash:      f.Hash,
			CreatedBy: f.CreatedBy,
		}).Asserts(f, policy.ActionRead).Returns(f)
	}))
	s.Run("GetFileByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		f := testutil.Fake(s.T(), faker, database.File{})
		dbm.EXPECT().GetFileByID(gomock.Any(), f.ID).Return(f, nil).AnyTimes()
		dbm.EXPECT().GetFileTemplates(gomock.Any(), f.ID).Return([]database.GetFileTemplatesRow{}, nil).AnyTimes()
		check.Args(f.ID).Asserts(f, policy.ActionRead).Returns(f)
	}))
	s.Run("GetFileIDByTemplateVersionID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		tvID := uuid.New()
		fileID := uuid.New()
		dbm.EXPECT().GetFileIDByTemplateVersionID(gomock.Any(), tvID).Return(fileID, nil).AnyTimes()
		check.Args(tvID).Asserts(rbac.ResourceFile.WithID(fileID), policy.ActionRead).Returns(fileID)
	}))
	s.Run("InsertFile", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		ret := testutil.Fake(s.T(), faker, database.File{CreatedBy: u.ID})
		dbm.EXPECT().InsertFile(gomock.Any(), gomock.Any()).Return(ret, nil).AnyTimes()
		check.Args(database.InsertFileParams{
			CreatedBy: u.ID,
		}).Asserts(rbac.ResourceFile.WithOwner(u.ID.String()), policy.ActionCreate)
	}))
}

func (s *MethodTestSuite) TestGroup() {
	s.Run("DeleteGroupByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		dbm.EXPECT().GetGroupByID(gomock.Any(), g.ID).Return(g, nil).AnyTimes()
		dbm.EXPECT().DeleteGroupByID(gomock.Any(), g.ID).Return(nil).AnyTimes()
		check.Args(g.ID).Asserts(g, policy.ActionDelete).Returns()
	}))

	s.Run("DeleteGroupMemberFromGroup", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		u := testutil.Fake(s.T(), faker, database.User{})
		m := testutil.Fake(s.T(), faker, database.GroupMember{GroupID: g.ID, UserID: u.ID})
		dbm.EXPECT().GetGroupByID(gomock.Any(), g.ID).Return(g, nil).AnyTimes()
		dbm.EXPECT().DeleteGroupMemberFromGroup(gomock.Any(), database.DeleteGroupMemberFromGroupParams{UserID: m.UserID, GroupID: g.ID}).Return(nil).AnyTimes()
		check.Args(database.DeleteGroupMemberFromGroupParams{UserID: m.UserID, GroupID: g.ID}).Asserts(g, policy.ActionUpdate).Returns()
	}))

	s.Run("GetGroupByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		dbm.EXPECT().GetGroupByID(gomock.Any(), g.ID).Return(g, nil).AnyTimes()
		check.Args(g.ID).Asserts(g, policy.ActionRead).Returns(g)
	}))

	s.Run("GetGroupByOrgAndName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		dbm.EXPECT().GetGroupByOrgAndName(gomock.Any(), database.GetGroupByOrgAndNameParams{OrganizationID: g.OrganizationID, Name: g.Name}).Return(g, nil).AnyTimes()
		check.Args(database.GetGroupByOrgAndNameParams{OrganizationID: g.OrganizationID, Name: g.Name}).Asserts(g, policy.ActionRead).Returns(g)
	}))

	s.Run("GetGroupMembersByGroupID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		u := testutil.Fake(s.T(), faker, database.User{})
		gm := testutil.Fake(s.T(), faker, database.GroupMember{GroupID: g.ID, UserID: u.ID})
		arg := database.GetGroupMembersByGroupIDParams{GroupID: g.ID, IncludeSystem: false}
		dbm.EXPECT().GetGroupMembersByGroupID(gomock.Any(), arg).Return([]database.GroupMember{gm}, nil).AnyTimes()
		check.Args(arg).Asserts(gm, policy.ActionRead)
	}))

	s.Run("GetGroupMembersCountByGroupID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		arg := database.GetGroupMembersCountByGroupIDParams{GroupID: g.ID, IncludeSystem: false}
		dbm.EXPECT().GetGroupByID(gomock.Any(), g.ID).Return(g, nil).AnyTimes()
		dbm.EXPECT().GetGroupMembersCountByGroupID(gomock.Any(), arg).Return(int64(0), nil).AnyTimes()
		check.Args(arg).Asserts(g, policy.ActionRead)
	}))

	s.Run("GetGroupMembers", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetGroupMembers(gomock.Any(), false).Return([]database.GroupMember{}, nil).AnyTimes()
		check.Args(false).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))

	s.Run("System/GetGroups", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		g := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		row := database.GetGroupsRow{Group: g, OrganizationName: o.Name, OrganizationDisplayName: o.DisplayName}
		dbm.EXPECT().GetGroups(gomock.Any(), database.GetGroupsParams{}).Return([]database.GetGroupsRow{row}, nil).AnyTimes()
		check.Args(database.GetGroupsParams{}).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))

	s.Run("GetGroups", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		g := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		u := testutil.Fake(s.T(), faker, database.User{})
		gm := testutil.Fake(s.T(), faker, database.GroupMember{GroupID: g.ID, UserID: u.ID})
		params := database.GetGroupsParams{OrganizationID: g.OrganizationID, HasMemberID: gm.UserID}
		row := database.GetGroupsRow{Group: g, OrganizationName: o.Name, OrganizationDisplayName: o.DisplayName}
		dbm.EXPECT().GetGroups(gomock.Any(), params).Return([]database.GetGroupsRow{row}, nil).AnyTimes()
		check.Args(params).Asserts(rbac.ResourceSystem, policy.ActionRead, g, policy.ActionRead).FailSystemObjectChecks()
	}))

	s.Run("InsertAllUsersGroup", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		ret := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		dbm.EXPECT().InsertAllUsersGroup(gomock.Any(), o.ID).Return(ret, nil).AnyTimes()
		check.Args(o.ID).Asserts(rbac.ResourceGroup.InOrg(o.ID), policy.ActionCreate)
	}))

	s.Run("InsertGroup", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		arg := database.InsertGroupParams{OrganizationID: o.ID, Name: "test"}
		ret := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID, Name: arg.Name})
		dbm.EXPECT().InsertGroup(gomock.Any(), arg).Return(ret, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGroup.InOrg(o.ID), policy.ActionCreate)
	}))

	s.Run("InsertGroupMember", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		arg := database.InsertGroupMemberParams{UserID: uuid.New(), GroupID: g.ID}
		dbm.EXPECT().GetGroupByID(gomock.Any(), g.ID).Return(g, nil).AnyTimes()
		dbm.EXPECT().InsertGroupMember(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(g, policy.ActionUpdate).Returns()
	}))

	s.Run("InsertUserGroupsByName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u1 := testutil.Fake(s.T(), faker, database.User{})
		g1 := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		g2 := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		arg := database.InsertUserGroupsByNameParams{OrganizationID: o.ID, UserID: u1.ID, GroupNames: slice.New(g1.Name, g2.Name)}
		dbm.EXPECT().InsertUserGroupsByName(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceGroup.InOrg(o.ID), policy.ActionUpdate).Returns()
	}))

	s.Run("InsertUserGroupsByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u1 := testutil.Fake(s.T(), faker, database.User{})
		g1 := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		g2 := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		g3 := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		returns := slice.New(g2.ID, g3.ID)
		arg := database.InsertUserGroupsByIDParams{UserID: u1.ID, GroupIds: slice.New(g1.ID, g2.ID, g3.ID)}
		dbm.EXPECT().InsertUserGroupsByID(gomock.Any(), arg).Return(returns, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns(returns)
	}))

	s.Run("RemoveUserFromAllGroups", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u1 := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().RemoveUserFromAllGroups(gomock.Any(), u1.ID).Return(nil).AnyTimes()
		check.Args(u1.ID).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))

	s.Run("RemoveUserFromGroups", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u1 := testutil.Fake(s.T(), faker, database.User{})
		g1 := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		g2 := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		arg := database.RemoveUserFromGroupsParams{UserID: u1.ID, GroupIds: []uuid.UUID{g1.ID, g2.ID}}
		dbm.EXPECT().RemoveUserFromGroups(gomock.Any(), arg).Return(slice.New(g1.ID, g2.ID), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns(slice.New(g1.ID, g2.ID))
	}))

	s.Run("UpdateGroupByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		g := testutil.Fake(s.T(), faker, database.Group{})
		arg := database.UpdateGroupByIDParams{ID: g.ID}
		dbm.EXPECT().GetGroupByID(gomock.Any(), g.ID).Return(g, nil).AnyTimes()
		dbm.EXPECT().UpdateGroupByID(gomock.Any(), arg).Return(g, nil).AnyTimes()
		check.Args(arg).Asserts(g, policy.ActionUpdate)
	}))

	s.Run("ValidateGroupIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		g := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		ids := []uuid.UUID{g.ID}
		dbm.EXPECT().ValidateGroupIDs(gomock.Any(), ids).Return(database.ValidateGroupIDsRow{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestProvisionerJob() {
	s.Run("ArchiveUnusedTemplateVersions", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		arg := database.ArchiveUnusedTemplateVersionsParams{UpdatedAt: dbtime.Now(), TemplateID: tpl.ID, TemplateVersionID: v.ID, JobStatus: database.NullProvisionerJobStatus{}}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().ArchiveUnusedTemplateVersions(gomock.Any(), arg).Return([]uuid.UUID{}, nil).AnyTimes()
		check.Args(arg).Asserts(tpl.RBACObject(), policy.ActionUpdate)
	}))
	s.Run("UnarchiveTemplateVersion", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, Archived: true})
		arg := database.UnarchiveTemplateVersionParams{UpdatedAt: dbtime.Now(), TemplateVersionID: v.ID}
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), v.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().UnarchiveTemplateVersion(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(tpl.RBACObject(), policy.ActionUpdate)
	}))
	s.Run("Build/GetProvisionerJobByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: j.ID})
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), j.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), build.WorkspaceID).Return(ws, nil).AnyTimes()
		check.Args(j.ID).Asserts(ws, policy.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersion/GetProvisionerJobByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeTemplateVersionImport})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), j.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		check.Args(j.ID).Asserts(v.RBACObject(tpl), policy.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersionDryRun/GetProvisionerJobByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeTemplateVersionDryRun})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		j.Input = must(json.Marshal(struct {
			TemplateVersionID uuid.UUID `json:"template_version_id"`
		}{TemplateVersionID: v.ID}))
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), v.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		check.Args(j.ID).Asserts(v.RBACObject(tpl), policy.ActionRead).Returns(j)
	}))
	s.Run("Build/UpdateProvisionerJobWithCancelByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{AllowUserCancelWorkspaceJobs: true})
		ws := testutil.Fake(s.T(), faker, database.Workspace{TemplateID: tpl.ID})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: ws.ID})
		arg := database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}

		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), j.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().UpdateProvisionerJobWithCancelByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("BuildFalseCancel/UpdateProvisionerJobWithCancelByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{AllowUserCancelWorkspaceJobs: false})
		ws := testutil.Fake(s.T(), faker, database.Workspace{TemplateID: tpl.ID})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: ws.ID})
		arg := database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), j.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().UpdateProvisionerJobWithCancelByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("TemplateVersion/UpdateProvisionerJobWithCancelByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeTemplateVersionImport})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		arg := database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), j.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().UpdateProvisionerJobWithCancelByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(v.RBACObject(tpl), []policy.Action{policy.ActionRead, policy.ActionUpdate}).Returns()
	}))
	s.Run("TemplateVersionNoTemplate/UpdateProvisionerJobWithCancelByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeTemplateVersionImport})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{JobID: j.ID})
		// uuid.NullUUID{Valid: false} is a zero value. faker overwrites zero values
		// with random data, so we need to set TemplateID after faker is done with it.
		v.TemplateID = uuid.NullUUID{UUID: uuid.Nil, Valid: false}
		arg := database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), j.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().UpdateProvisionerJobWithCancelByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(v.RBACObjectNoTemplate(), []policy.Action{policy.ActionRead, policy.ActionUpdate}).Returns()
	}))
	s.Run("TemplateVersionDryRun/UpdateProvisionerJobWithCancelByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeTemplateVersionDryRun})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		j.Input = must(json.Marshal(struct {
			TemplateVersionID uuid.UUID `json:"template_version_id"`
		}{TemplateVersionID: v.ID}))
		arg := database.UpdateProvisionerJobWithCancelByIDParams{ID: j.ID}
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), v.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().UpdateProvisionerJobWithCancelByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(v.RBACObject(tpl), []policy.Action{policy.ActionRead, policy.ActionUpdate}).Returns()
	}))
	s.Run("UpdatePrebuildProvisionerJobWithCancel", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := database.UpdatePrebuildProvisionerJobWithCancelParams{
			PresetID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
			Now:      dbtime.Now(),
		}
		canceledJobs := []database.UpdatePrebuildProvisionerJobWithCancelRow{
			{ID: uuid.New(), WorkspaceID: uuid.New(), TemplateID: uuid.New(), TemplateVersionPresetID: uuid.NullUUID{UUID: uuid.New(), Valid: true}},
			{ID: uuid.New(), WorkspaceID: uuid.New(), TemplateID: uuid.New(), TemplateVersionPresetID: uuid.NullUUID{UUID: uuid.New(), Valid: true}},
		}

		dbm.EXPECT().UpdatePrebuildProvisionerJobWithCancel(gomock.Any(), arg).Return(canceledJobs, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourcePrebuiltWorkspace, policy.ActionUpdate).Returns(canceledJobs)
	}))
	s.Run("GetProvisionerJobsByIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		org2 := testutil.Fake(s.T(), faker, database.Organization{})
		a := testutil.Fake(s.T(), faker, database.ProvisionerJob{OrganizationID: org.ID})
		b := testutil.Fake(s.T(), faker, database.ProvisionerJob{OrganizationID: org2.ID})
		ids := []uuid.UUID{a.ID, b.ID}
		dbm.EXPECT().GetProvisionerJobsByIDs(gomock.Any(), ids).Return([]database.ProvisionerJob{a, b}, nil).AnyTimes()
		check.Args(ids).Asserts(
			rbac.ResourceProvisionerJobs.InOrg(org.ID), policy.ActionRead,
			rbac.ResourceProvisionerJobs.InOrg(org2.ID), policy.ActionRead,
		).OutOfOrder().Returns(slice.New(a, b))
	}))
	s.Run("GetProvisionerLogsAfterID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{JobID: j.ID, WorkspaceID: ws.ID})
		arg := database.GetProvisionerLogsAfterIDParams{JobID: j.ID}
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), j.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetProvisionerLogsAfterID(gomock.Any(), arg).Return([]database.ProvisionerJobLog{}, nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionRead).Returns([]database.ProvisionerJobLog{})
	}))
	s.Run("Build/GetProvisionerJobByIDWithLock", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID, JobID: j.ID})
		dbm.EXPECT().GetProvisionerJobByIDWithLock(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), j.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), build.WorkspaceID).Return(ws, nil).AnyTimes()
		check.Args(j.ID).Asserts(ws, policy.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersion/GetProvisionerJobByIDWithLock", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeTemplateVersionImport})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{JobID: j.ID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		dbm.EXPECT().GetProvisionerJobByIDWithLock(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), j.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		check.Args(j.ID).Asserts(v.RBACObject(tpl), policy.ActionRead).Returns(j)
	}))
}

func (s *MethodTestSuite) TestLicense() {
	s.Run("GetLicenses", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		a := database.License{ID: 1}
		b := database.License{ID: 2}
		dbm.EXPECT().GetLicenses(gomock.Any()).Return([]database.License{a, b}, nil).AnyTimes()
		check.Args().Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns([]database.License{a, b})
	}))
	s.Run("GetUnexpiredLicenses", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		l := database.License{
			ID:   1,
			Exp:  time.Now().Add(time.Hour * 24 * 30),
			UUID: uuid.New(),
		}
		db.EXPECT().GetUnexpiredLicenses(gomock.Any()).
			Return([]database.License{l}, nil).
			AnyTimes()
		check.Args().Asserts(rbac.ResourceLicense, policy.ActionRead).
			Returns([]database.License{l})
	}))
	s.Run("InsertLicense", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().InsertLicense(gomock.Any(), database.InsertLicenseParams{}).Return(database.License{}, nil).AnyTimes()
		check.Args(database.InsertLicenseParams{}).Asserts(rbac.ResourceLicense, policy.ActionCreate)
	}))
	s.Run("UpsertLogoURL", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertLogoURL(gomock.Any(), "value").Return(nil).AnyTimes()
		check.Args("value").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("UpsertAnnouncementBanners", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertAnnouncementBanners(gomock.Any(), "value").Return(nil).AnyTimes()
		check.Args("value").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetLicenseByID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		l := database.License{ID: 1}
		dbm.EXPECT().GetLicenseByID(gomock.Any(), int32(1)).Return(l, nil).AnyTimes()
		check.Args(int32(1)).Asserts(l, policy.ActionRead).Returns(l)
	}))
	s.Run("DeleteLicense", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		l := database.License{ID: 1}
		dbm.EXPECT().GetLicenseByID(gomock.Any(), l.ID).Return(l, nil).AnyTimes()
		dbm.EXPECT().DeleteLicense(gomock.Any(), l.ID).Return(int32(1), nil).AnyTimes()
		check.Args(l.ID).Asserts(l, policy.ActionDelete)
	}))
	s.Run("GetDeploymentID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetDeploymentID(gomock.Any()).Return("value", nil).AnyTimes()
		check.Args().Asserts().Returns("value")
	}))
	s.Run("GetDefaultProxyConfig", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetDefaultProxyConfig(gomock.Any()).Return(database.GetDefaultProxyConfigRow{DisplayName: "Default", IconUrl: "/emojis/1f3e1.png"}, nil).AnyTimes()
		check.Args().Asserts().Returns(database.GetDefaultProxyConfigRow{DisplayName: "Default", IconUrl: "/emojis/1f3e1.png"})
	}))
	s.Run("GetLogoURL", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetLogoURL(gomock.Any()).Return("value", nil).AnyTimes()
		check.Args().Asserts().Returns("value")
	}))
	s.Run("GetAnnouncementBanners", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetAnnouncementBanners(gomock.Any()).Return("value", nil).AnyTimes()
		check.Args().Asserts().Returns("value")
	}))
}

func (s *MethodTestSuite) TestOrganization() {
	s.Run("Deployment/OIDCClaimFields", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().OIDCClaimFields(gomock.Any(), uuid.Nil).Return([]string{}, nil).AnyTimes()
		check.Args(uuid.Nil).Asserts(rbac.ResourceIdpsyncSettings, policy.ActionRead).Returns([]string{})
	}))
	s.Run("Organization/OIDCClaimFields", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		dbm.EXPECT().OIDCClaimFields(gomock.Any(), id).Return([]string{}, nil).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceIdpsyncSettings.InOrg(id), policy.ActionRead).Returns([]string{})
	}))
	s.Run("Deployment/OIDCClaimFieldValues", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.OIDCClaimFieldValuesParams{ClaimField: "claim-field", OrganizationID: uuid.Nil}
		dbm.EXPECT().OIDCClaimFieldValues(gomock.Any(), arg).Return([]string{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceIdpsyncSettings, policy.ActionRead).Returns([]string{})
	}))
	s.Run("Organization/OIDCClaimFieldValues", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		arg := database.OIDCClaimFieldValuesParams{ClaimField: "claim-field", OrganizationID: id}
		dbm.EXPECT().OIDCClaimFieldValues(gomock.Any(), arg).Return([]string{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceIdpsyncSettings.InOrg(id), policy.ActionRead).Returns([]string{})
	}))
	s.Run("ByOrganization/GetGroups", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		a := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		b := testutil.Fake(s.T(), faker, database.Group{OrganizationID: o.ID})
		params := database.GetGroupsParams{OrganizationID: o.ID}
		rows := []database.GetGroupsRow{
			{Group: a, OrganizationName: o.Name, OrganizationDisplayName: o.DisplayName},
			{Group: b, OrganizationName: o.Name, OrganizationDisplayName: o.DisplayName},
		}
		dbm.EXPECT().GetGroups(gomock.Any(), params).Return(rows, nil).AnyTimes()
		check.Args(params).
			Asserts(rbac.ResourceSystem, policy.ActionRead, a, policy.ActionRead, b, policy.ActionRead).
			Returns(rows).
			FailSystemObjectChecks()
	}))
	s.Run("GetOrganizationByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		dbm.EXPECT().GetOrganizationByID(gomock.Any(), o.ID).Return(o, nil).AnyTimes()
		check.Args(o.ID).Asserts(o, policy.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationResourceCountByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		row := database.GetOrganizationResourceCountByIDRow{
			WorkspaceCount:      1,
			GroupCount:          1,
			TemplateCount:       1,
			MemberCount:         1,
			ProvisionerKeyCount: 0,
		}
		dbm.EXPECT().GetOrganizationResourceCountByID(gomock.Any(), o.ID).Return(row, nil).AnyTimes()
		check.Args(o.ID).Asserts(
			rbac.ResourceOrganizationMember.InOrg(o.ID), policy.ActionRead,
			rbac.ResourceWorkspace.InOrg(o.ID), policy.ActionRead,
			rbac.ResourceGroup.InOrg(o.ID), policy.ActionRead,
			rbac.ResourceTemplate.InOrg(o.ID), policy.ActionRead,
			rbac.ResourceProvisionerDaemon.InOrg(o.ID), policy.ActionRead,
		).Returns(row)
	}))
	s.Run("GetDefaultOrganization", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		dbm.EXPECT().GetDefaultOrganization(gomock.Any()).Return(o, nil).AnyTimes()
		check.Args().Asserts(o, policy.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationByName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		arg := database.GetOrganizationByNameParams{Name: o.Name, Deleted: o.Deleted}
		dbm.EXPECT().GetOrganizationByName(gomock.Any(), arg).Return(o, nil).AnyTimes()
		check.Args(arg).Asserts(o, policy.ActionRead).Returns(o)
	}))
	s.Run("GetOrganizationIDsByMemberIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		oa := testutil.Fake(s.T(), faker, database.Organization{})
		ob := testutil.Fake(s.T(), faker, database.Organization{})
		ua := testutil.Fake(s.T(), faker, database.User{})
		ub := testutil.Fake(s.T(), faker, database.User{})
		ids := []uuid.UUID{ua.ID, ub.ID}
		rows := []database.GetOrganizationIDsByMemberIDsRow{
			{UserID: ua.ID, OrganizationIDs: []uuid.UUID{oa.ID}},
			{UserID: ub.ID, OrganizationIDs: []uuid.UUID{ob.ID}},
		}
		dbm.EXPECT().GetOrganizationIDsByMemberIDs(gomock.Any(), ids).Return(rows, nil).AnyTimes()
		check.Args(ids).
			Asserts(rows[0].RBACObject(), policy.ActionRead, rows[1].RBACObject(), policy.ActionRead).
			OutOfOrder()
	}))
	s.Run("GetOrganizations", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		def := testutil.Fake(s.T(), faker, database.Organization{})
		a := testutil.Fake(s.T(), faker, database.Organization{})
		b := testutil.Fake(s.T(), faker, database.Organization{})
		arg := database.GetOrganizationsParams{}
		dbm.EXPECT().GetOrganizations(gomock.Any(), arg).Return([]database.Organization{def, a, b}, nil).AnyTimes()
		check.Args(arg).Asserts(def, policy.ActionRead, a, policy.ActionRead, b, policy.ActionRead).Returns(slice.New(def, a, b))
	}))
	s.Run("GetOrganizationsByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		a := testutil.Fake(s.T(), faker, database.Organization{})
		b := testutil.Fake(s.T(), faker, database.Organization{})
		arg := database.GetOrganizationsByUserIDParams{UserID: u.ID, Deleted: sql.NullBool{Valid: true, Bool: false}}
		dbm.EXPECT().GetOrganizationsByUserID(gomock.Any(), arg).Return([]database.Organization{a, b}, nil).AnyTimes()
		check.Args(arg).Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("InsertOrganization", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertOrganizationParams{ID: uuid.New(), Name: "new-org"}
		dbm.EXPECT().InsertOrganization(gomock.Any(), arg).Return(database.Organization{ID: arg.ID, Name: arg.Name}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceOrganization, policy.ActionCreate)
	}))
	s.Run("InsertOrganizationMember", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.InsertOrganizationMemberParams{OrganizationID: o.ID, UserID: u.ID, Roles: []string{codersdk.RoleOrganizationAdmin}}
		dbm.EXPECT().InsertOrganizationMember(gomock.Any(), arg).Return(database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID, Roles: arg.Roles}, nil).AnyTimes()
		check.Args(arg).Asserts(
			rbac.ResourceAssignOrgRole.InOrg(o.ID), policy.ActionAssign,
			rbac.ResourceOrganizationMember.InOrg(o.ID).WithID(u.ID), policy.ActionCreate,
		)
	}))
	s.Run("InsertPreset", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertPresetParams{TemplateVersionID: uuid.New(), Name: "test"}
		dbm.EXPECT().InsertPreset(gomock.Any(), arg).Return(database.TemplateVersionPreset{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionUpdate)
	}))
	s.Run("InsertPresetParameters", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertPresetParametersParams{TemplateVersionPresetID: uuid.New(), Names: []string{"test"}, Values: []string{"test"}}
		dbm.EXPECT().InsertPresetParameters(gomock.Any(), arg).Return([]database.TemplateVersionPresetParameter{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionUpdate)
	}))
	s.Run("InsertPresetPrebuildSchedule", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertPresetPrebuildScheduleParams{PresetID: uuid.New()}
		dbm.EXPECT().InsertPresetPrebuildSchedule(gomock.Any(), arg).Return(database.TemplateVersionPresetPrebuildSchedule{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionUpdate)
	}))
	s.Run("DeleteOrganizationMember", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u := testutil.Fake(s.T(), faker, database.User{})
		member := testutil.Fake(s.T(), faker, database.OrganizationMember{UserID: u.ID, OrganizationID: o.ID})

		params := database.OrganizationMembersParams{OrganizationID: o.ID, UserID: u.ID, IncludeSystem: false}
		dbm.EXPECT().OrganizationMembers(gomock.Any(), params).Return([]database.OrganizationMembersRow{{OrganizationMember: member}}, nil).AnyTimes()
		dbm.EXPECT().DeleteOrganizationMember(gomock.Any(), database.DeleteOrganizationMemberParams{OrganizationID: o.ID, UserID: u.ID}).Return(nil).AnyTimes()

		check.Args(database.DeleteOrganizationMemberParams{OrganizationID: o.ID, UserID: u.ID}).Asserts(
			member, policy.ActionRead,
			member, policy.ActionDelete,
		).WithNotAuthorized("no rows").WithCancelled(sql.ErrNoRows.Error())
	}))
	s.Run("UpdateOrganization", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{Name: "something-unique"})
		arg := database.UpdateOrganizationParams{ID: o.ID, Name: "something-different"}

		dbm.EXPECT().GetOrganizationByID(gomock.Any(), o.ID).Return(o, nil).AnyTimes()
		dbm.EXPECT().UpdateOrganization(gomock.Any(), arg).Return(o, nil).AnyTimes()
		check.Args(arg).Asserts(o, policy.ActionUpdate)
	}))
	s.Run("UpdateOrganizationDeletedByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{Name: "doomed"})
		dbm.EXPECT().GetOrganizationByID(gomock.Any(), o.ID).Return(o, nil).AnyTimes()
		dbm.EXPECT().UpdateOrganizationDeletedByID(gomock.Any(), gomock.AssignableToTypeOf(database.UpdateOrganizationDeletedByIDParams{})).Return(nil).AnyTimes()
		check.Args(database.UpdateOrganizationDeletedByIDParams{ID: o.ID, UpdatedAt: o.UpdatedAt}).Asserts(o, policy.ActionDelete).Returns()
	}))
	s.Run("OrganizationMembers", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u := testutil.Fake(s.T(), faker, database.User{})
		mem := testutil.Fake(s.T(), faker, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID, Roles: []string{rbac.RoleOrgAdmin()}})

		arg := database.OrganizationMembersParams{OrganizationID: o.ID, UserID: u.ID}
		dbm.EXPECT().OrganizationMembers(gomock.Any(), gomock.AssignableToTypeOf(database.OrganizationMembersParams{})).Return([]database.OrganizationMembersRow{{OrganizationMember: mem}}, nil).AnyTimes()

		check.Args(arg).Asserts(mem, policy.ActionRead)
	}))
	s.Run("PaginatedOrganizationMembers", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u := testutil.Fake(s.T(), faker, database.User{})
		mem := testutil.Fake(s.T(), faker, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID, Roles: []string{rbac.RoleOrgAdmin()}})

		arg := database.PaginatedOrganizationMembersParams{OrganizationID: o.ID, LimitOpt: 0}
		rows := []database.PaginatedOrganizationMembersRow{{
			OrganizationMember: mem,
			Username:           u.Username,
			AvatarURL:          u.AvatarURL,
			Name:               u.Name,
			Email:              u.Email,
			GlobalRoles:        u.RBACRoles,
			Count:              1,
		}}
		dbm.EXPECT().PaginatedOrganizationMembers(gomock.Any(), arg).Return(rows, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceOrganizationMember.InOrg(o.ID), policy.ActionRead).Returns(rows)
	}))
	s.Run("UpdateMemberRoles", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		o := testutil.Fake(s.T(), faker, database.Organization{})
		u := testutil.Fake(s.T(), faker, database.User{})
		mem := testutil.Fake(s.T(), faker, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID, Roles: []string{codersdk.RoleOrganizationAdmin}})
		out := mem
		out.Roles = []string{}

		dbm.EXPECT().OrganizationMembers(gomock.Any(), database.OrganizationMembersParams{OrganizationID: o.ID, UserID: u.ID, IncludeSystem: false}).Return([]database.OrganizationMembersRow{{OrganizationMember: mem}}, nil).AnyTimes()
		arg := database.UpdateMemberRolesParams{GrantedRoles: []string{}, UserID: u.ID, OrgID: o.ID}
		dbm.EXPECT().UpdateMemberRoles(gomock.Any(), arg).Return(out, nil).AnyTimes()

		check.Args(arg).
			WithNotAuthorized(sql.ErrNoRows.Error()).
			WithCancelled(sql.ErrNoRows.Error()).
			Asserts(
				mem, policy.ActionRead,
				rbac.ResourceAssignOrgRole.InOrg(o.ID), policy.ActionAssign, // org-mem
				rbac.ResourceAssignOrgRole.InOrg(o.ID), policy.ActionUnassign, // org-admin
			).Returns(out)
	}))
}

func (s *MethodTestSuite) TestWorkspaceProxy() {
	s.Run("InsertWorkspaceProxy", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceProxyParams{ID: uuid.New()}
		dbm.EXPECT().InsertWorkspaceProxy(gomock.Any(), arg).Return(database.WorkspaceProxy{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceWorkspaceProxy, policy.ActionCreate)
	}))
	s.Run("RegisterWorkspaceProxy", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		p := testutil.Fake(s.T(), faker, database.WorkspaceProxy{})
		dbm.EXPECT().GetWorkspaceProxyByID(gomock.Any(), p.ID).Return(p, nil).AnyTimes()
		dbm.EXPECT().RegisterWorkspaceProxy(gomock.Any(), database.RegisterWorkspaceProxyParams{ID: p.ID}).Return(p, nil).AnyTimes()
		check.Args(database.RegisterWorkspaceProxyParams{ID: p.ID}).Asserts(p, policy.ActionUpdate)
	}))
	s.Run("GetWorkspaceProxyByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		p := testutil.Fake(s.T(), faker, database.WorkspaceProxy{})
		dbm.EXPECT().GetWorkspaceProxyByID(gomock.Any(), p.ID).Return(p, nil).AnyTimes()
		check.Args(p.ID).Asserts(p, policy.ActionRead).Returns(p)
	}))
	s.Run("GetWorkspaceProxyByName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		p := testutil.Fake(s.T(), faker, database.WorkspaceProxy{})
		dbm.EXPECT().GetWorkspaceProxyByName(gomock.Any(), p.Name).Return(p, nil).AnyTimes()
		check.Args(p.Name).Asserts(p, policy.ActionRead).Returns(p)
	}))
	s.Run("UpdateWorkspaceProxyDeleted", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		p := testutil.Fake(s.T(), faker, database.WorkspaceProxy{})
		dbm.EXPECT().GetWorkspaceProxyByID(gomock.Any(), p.ID).Return(p, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceProxyDeleted(gomock.Any(), database.UpdateWorkspaceProxyDeletedParams{ID: p.ID, Deleted: true}).Return(nil).AnyTimes()
		check.Args(database.UpdateWorkspaceProxyDeletedParams{ID: p.ID, Deleted: true}).Asserts(p, policy.ActionDelete)
	}))
	s.Run("UpdateWorkspaceProxy", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		p := testutil.Fake(s.T(), faker, database.WorkspaceProxy{})
		dbm.EXPECT().GetWorkspaceProxyByID(gomock.Any(), p.ID).Return(p, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceProxy(gomock.Any(), database.UpdateWorkspaceProxyParams{ID: p.ID}).Return(p, nil).AnyTimes()
		check.Args(database.UpdateWorkspaceProxyParams{ID: p.ID}).Asserts(p, policy.ActionUpdate)
	}))
	s.Run("GetWorkspaceProxies", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		p1 := testutil.Fake(s.T(), faker, database.WorkspaceProxy{})
		p2 := testutil.Fake(s.T(), faker, database.WorkspaceProxy{})
		dbm.EXPECT().GetWorkspaceProxies(gomock.Any()).Return([]database.WorkspaceProxy{p1, p2}, nil).AnyTimes()
		check.Args().Asserts(p1, policy.ActionRead, p2, policy.ActionRead).Returns(slice.New(p1, p2))
	}))
}

func (s *MethodTestSuite) TestTemplate() {
	s.Run("GetPreviousTemplateVersion", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		b := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		arg := database.GetPreviousTemplateVersionParams{Name: b.Name, OrganizationID: t1.OrganizationID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().GetPreviousTemplateVersion(gomock.Any(), arg).Return(b, nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionRead).Returns(b)
	}))
	s.Run("GetTemplateByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		check.Args(t1.ID).Asserts(t1, policy.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateByOrganizationAndName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.GetTemplateByOrganizationAndNameParams{Name: t1.Name, OrganizationID: t1.OrganizationID}
		dbm.EXPECT().GetTemplateByOrganizationAndName(gomock.Any(), arg).Return(t1, nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionRead).Returns(t1)
	}))
	s.Run("GetTemplateVersionByJobID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), tv.JobID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		check.Args(tv.JobID).Asserts(t1, policy.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionByTemplateIDAndName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		arg := database.GetTemplateVersionByTemplateIDAndNameParams{Name: tv.Name, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}}
		dbm.EXPECT().GetTemplateVersionByTemplateIDAndName(gomock.Any(), arg).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionParameters", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionParameters(gomock.Any(), tv.ID).Return([]database.TemplateVersionParameter{}, nil).AnyTimes()
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns([]database.TemplateVersionParameter{})
	}))
	s.Run("GetTemplateVersionTerraformValues", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t.ID, Valid: true}})
		val := testutil.Fake(s.T(), faker, database.TemplateVersionTerraformValue{TemplateVersionID: tv.ID})
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t.ID).Return(t, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionTerraformValues(gomock.Any(), tv.ID).Return(val, nil).AnyTimes()
		check.Args(tv.ID).Asserts(t, policy.ActionRead)
	}))
	s.Run("GetTemplateVersionVariables", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		tvv1 := testutil.Fake(s.T(), faker, database.TemplateVersionVariable{TemplateVersionID: tv.ID})
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionVariables(gomock.Any(), tv.ID).Return([]database.TemplateVersionVariable{tvv1}, nil).AnyTimes()
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns([]database.TemplateVersionVariable{tvv1})
	}))
	s.Run("GetTemplateVersionWorkspaceTags", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		wt1 := testutil.Fake(s.T(), faker, database.TemplateVersionWorkspaceTag{TemplateVersionID: tv.ID})
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionWorkspaceTags(gomock.Any(), tv.ID).Return([]database.TemplateVersionWorkspaceTag{wt1}, nil).AnyTimes()
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns([]database.TemplateVersionWorkspaceTag{wt1})
	}))
	s.Run("GetTemplateGroupRoles", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().GetTemplateGroupRoles(gomock.Any(), t1.ID).Return([]database.TemplateGroup{}, nil).AnyTimes()
		check.Args(t1.ID).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("GetTemplateUserRoles", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().GetTemplateUserRoles(gomock.Any(), t1.ID).Return([]database.TemplateUser{}, nil).AnyTimes()
		check.Args(t1.ID).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("GetTemplateVersionByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		check.Args(tv.ID).Asserts(t1, policy.ActionRead).Returns(tv)
	}))
	s.Run("Orphaned/GetTemplateVersionByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		// uuid.NullUUID{Valid: false} is a zero value. faker overwrites zero values
		// with random data, so we need to set TemplateID after faker is done with it.
		tv.TemplateID = uuid.NullUUID{Valid: false}
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		check.Args(tv.ID).Asserts(tv.RBACObjectNoTemplate(), policy.ActionRead).Returns(tv)
	}))
	s.Run("GetTemplateVersionsByTemplateID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		a := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		b := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		arg := database.GetTemplateVersionsByTemplateIDParams{TemplateID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionsByTemplateID(gomock.Any(), arg).Return([]database.TemplateVersion{a, b}, nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetTemplateVersionsCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		now := time.Now()
		dbm.EXPECT().GetTemplateVersionsCreatedAfter(gomock.Any(), now.Add(-time.Hour)).Return([]database.TemplateVersion{}, nil).AnyTimes()
		check.Args(now.Add(-time.Hour)).Asserts(rbac.ResourceTemplate.All(), policy.ActionRead)
	}))
	s.Run("GetTemplateVersionHasAITask", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t.ID, Valid: true}})
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t.ID).Return(t, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionHasAITask(gomock.Any(), tv.ID).Return(false, nil).AnyTimes()
		check.Args(tv.ID).Asserts(t, policy.ActionRead)
	}))
	s.Run("GetTemplatesWithFilter", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.GetTemplatesWithFilterParams{}
		dbm.EXPECT().GetAuthorizedTemplates(gomock.Any(), arg, gomock.Any()).Return([]database.Template{a}, nil).AnyTimes()
		// No asserts because SQLFilter.
		check.Args(arg).Asserts().Returns(slice.New(a))
	}))
	s.Run("GetAuthorizedTemplates", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.GetTemplatesWithFilterParams{}
		dbm.EXPECT().GetAuthorizedTemplates(gomock.Any(), arg, gomock.Any()).Return([]database.Template{a}, nil).AnyTimes()
		// No asserts because SQLFilter.
		check.Args(arg, emptyPreparedAuthorized{}).Asserts().Returns(slice.New(a))
	}))
	s.Run("InsertTemplate", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertTemplateParams{OrganizationID: uuid.New()}
		dbm.EXPECT().InsertTemplate(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate.InOrg(arg.OrganizationID), policy.ActionCreate)
	}))
	s.Run("InsertTemplateVersion", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.InsertTemplateVersionParams{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, OrganizationID: t1.OrganizationID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().InsertTemplateVersion(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionRead, t1, policy.ActionCreate)
	}))
	s.Run("InsertTemplateVersionTerraformValuesByJobID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		job := uuid.New()
		arg := database.InsertTemplateVersionTerraformValuesByJobIDParams{JobID: job, CachedPlan: []byte("{}")}
		dbm.EXPECT().InsertTemplateVersionTerraformValuesByJobID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("SoftDeleteTemplateByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateDeletedByID(gomock.Any(), gomock.AssignableToTypeOf(database.UpdateTemplateDeletedByIDParams{})).Return(nil).AnyTimes()
		check.Args(t1.ID).Asserts(t1, policy.ActionDelete)
	}))
	s.Run("UpdateTemplateACLByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateTemplateACLByIDParams{ID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateACLByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionCreate)
	}))
	s.Run("UpdateTemplateAccessControlByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateTemplateAccessControlByIDParams{ID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateAccessControlByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateScheduleByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateTemplateScheduleByIDParams{ID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateScheduleByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateVersionFlagsByJobID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t.ID, Valid: true}})
		arg := database.UpdateTemplateVersionFlagsByJobIDParams{JobID: tv.JobID, HasAITask: sql.NullBool{Bool: true, Valid: true}, HasExternalAgent: sql.NullBool{Bool: true, Valid: true}}
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), tv.JobID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t.ID).Return(t, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateVersionFlagsByJobID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateWorkspacesLastUsedAt", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateTemplateWorkspacesLastUsedAtParams{TemplateID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateWorkspacesLastUsedAt(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspacesDormantDeletingAtByTemplateID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateWorkspacesDormantDeletingAtByTemplateIDParams{TemplateID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspacesDormantDeletingAtByTemplateID(gomock.Any(), arg).Return([]database.WorkspaceTable{}, nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspacesTTLByTemplateID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateWorkspacesTTLByTemplateIDParams{TemplateID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspacesTTLByTemplateID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateActiveVersionByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{ActiveVersionID: uuid.New()})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{ID: t1.ActiveVersionID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		arg := database.UpdateTemplateActiveVersionByIDParams{ID: t1.ID, ActiveVersionID: tv.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateActiveVersionByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateDeletedByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateTemplateDeletedByIDParams{ID: t1.ID, Deleted: true}
		// The method delegates to SoftDeleteTemplateByID, which fetches then updates.
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateDeletedByID(gomock.Any(), gomock.AssignableToTypeOf(database.UpdateTemplateDeletedByIDParams{})).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateTemplateMetaByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdateTemplateMetaByIDParams{ID: t1.ID, MaxPortSharingLevel: "owner", CorsBehavior: database.CorsBehaviorSimple}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateMetaByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateVersionByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		arg := database.UpdateTemplateVersionByIDParams{ID: tv.ID, TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}, Name: tv.Name, UpdatedAt: tv.UpdatedAt}
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateVersionByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
	s.Run("UpdateTemplateVersionDescriptionByJobID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		tv := database.TemplateVersion{ID: uuid.New(), JobID: uuid.New(), TemplateID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
		t1 := database.Template{ID: tv.TemplateID.UUID}
		arg := database.UpdateTemplateVersionDescriptionByJobIDParams{JobID: tv.JobID, Readme: "foo"}
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), tv.JobID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateVersionDescriptionByJobID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateTemplateVersionExternalAuthProvidersByJobID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		tv := database.TemplateVersion{ID: uuid.New(), JobID: uuid.New(), TemplateID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}
		t1 := database.Template{ID: tv.TemplateID.UUID}
		arg := database.UpdateTemplateVersionExternalAuthProvidersByJobIDParams{JobID: tv.JobID, ExternalAuthProviders: json.RawMessage("{}")}
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), tv.JobID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdateTemplateVersionExternalAuthProvidersByJobID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate).Returns()
	}))
	s.Run("GetTemplateInsights", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateInsightsParams{}
		dbm.EXPECT().GetTemplateInsights(gomock.Any(), arg).Return(database.GetTemplateInsightsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetUserLatencyInsights", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetUserLatencyInsightsParams{}
		dbm.EXPECT().GetUserLatencyInsights(gomock.Any(), arg).Return([]database.GetUserLatencyInsightsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetUserActivityInsights", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetUserActivityInsightsParams{}
		dbm.EXPECT().GetUserActivityInsights(gomock.Any(), arg).Return([]database.GetUserActivityInsightsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights).Returns([]database.GetUserActivityInsightsRow{})
	}))
	s.Run("GetTemplateParameterInsights", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateParameterInsightsParams{}
		dbm.EXPECT().GetTemplateParameterInsights(gomock.Any(), arg).Return([]database.GetTemplateParameterInsightsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateInsightsByInterval", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateInsightsByIntervalParams{IntervalDays: 7, StartTime: dbtime.Now().Add(-time.Hour * 24 * 7), EndTime: dbtime.Now()}
		dbm.EXPECT().GetTemplateInsightsByInterval(gomock.Any(), arg).Return([]database.GetTemplateInsightsByIntervalRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateInsightsByTemplate", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateInsightsByTemplateParams{}
		dbm.EXPECT().GetTemplateInsightsByTemplate(gomock.Any(), arg).Return([]database.GetTemplateInsightsByTemplateRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateAppInsights", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateAppInsightsParams{}
		dbm.EXPECT().GetTemplateAppInsights(gomock.Any(), arg).Return([]database.GetTemplateAppInsightsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateAppInsightsByTemplate", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateAppInsightsByTemplateParams{}
		dbm.EXPECT().GetTemplateAppInsightsByTemplate(gomock.Any(), arg).Return([]database.GetTemplateAppInsightsByTemplateRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights)
	}))
	s.Run("GetTemplateUsageStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateUsageStatsParams{}
		dbm.EXPECT().GetTemplateUsageStats(gomock.Any(), arg).Return([]database.TemplateUsageStat{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate, policy.ActionViewInsights).Returns([]database.TemplateUsageStat{})
	}))
	s.Run("UpsertTemplateUsageStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertTemplateUsageStats(gomock.Any()).Return(nil).AnyTimes()
		check.Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("UpdatePresetsLastInvalidatedAt", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.UpdatePresetsLastInvalidatedAtParams{LastInvalidatedAt: sql.NullTime{Valid: true, Time: dbtime.Now()}, TemplateID: t1.ID}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		dbm.EXPECT().UpdatePresetsLastInvalidatedAt(gomock.Any(), arg).Return([]database.UpdatePresetsLastInvalidatedAtRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(t1, policy.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestUser() {
	s.Run("GetAuthorizedUsers", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetUsersParams{}
		dbm.EXPECT().GetAuthorizedUsers(gomock.Any(), arg, gomock.Any()).Return([]database.GetUsersRow{}, nil).AnyTimes()
		// No asserts because SQLFilter.
		check.Args(arg, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("DeleteAPIKeysByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.APIKey{})
		dbm.EXPECT().DeleteAPIKeysByUserID(gomock.Any(), key.UserID).Return(nil).AnyTimes()
		check.Args(key.UserID).Asserts(rbac.ResourceApiKey.WithOwner(key.UserID.String()), policy.ActionDelete).Returns()
	}))
	s.Run("ExpirePrebuildsAPIKeys", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		dbm.EXPECT().ExpirePrebuildsAPIKeys(gomock.Any(), gomock.Any()).Times(1).Return(nil)
		check.Args(dbtime.Now()).Asserts(rbac.ResourceApiKey, policy.ActionDelete).Returns()
	}))
	s.Run("GetQuotaAllowanceForUser", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.GetQuotaAllowanceForUserParams{UserID: u.ID, OrganizationID: uuid.New()}
		dbm.EXPECT().GetQuotaAllowanceForUser(gomock.Any(), arg).Return(int64(0), nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetQuotaConsumedForUser", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.GetQuotaConsumedForUserParams{OwnerID: u.ID, OrganizationID: uuid.New()}
		dbm.EXPECT().GetQuotaConsumedForUser(gomock.Any(), arg).Return(int64(0), nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetUserByEmailOrUsername", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.GetUserByEmailOrUsernameParams{Email: u.Email}
		dbm.EXPECT().GetUserByEmailOrUsername(gomock.Any(), arg).Return(u, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionRead).Returns(u)
	}))
	s.Run("GetUserByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		check.Args(u.ID).Asserts(u, policy.ActionRead).Returns(u)
	}))
	s.Run("GetUsersByIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.User{CreatedAt: dbtime.Now().Add(-time.Hour)})
		b := testutil.Fake(s.T(), faker, database.User{CreatedAt: dbtime.Now()})
		ids := []uuid.UUID{a.ID, b.ID}
		dbm.EXPECT().GetUsersByIDs(gomock.Any(), ids).Return([]database.User{a, b}, nil).AnyTimes()
		check.Args(ids).Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns(slice.New(a, b))
	}))
	s.Run("GetUsers", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetUsersParams{}
		dbm.EXPECT().GetAuthorizedUsers(gomock.Any(), arg, gomock.Any()).Return([]database.GetUsersRow{}, nil).AnyTimes()
		// Asserts are done in a SQL filter
		check.Args(arg).Asserts()
	}))
	s.Run("InsertUser", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertUserParams{ID: uuid.New(), LoginType: database.LoginTypePassword, RBACRoles: []string{}}
		dbm.EXPECT().InsertUser(gomock.Any(), arg).Return(database.User{ID: arg.ID, LoginType: arg.LoginType}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceAssignRole, policy.ActionAssign, rbac.ResourceUser, policy.ActionCreate)
	}))
	s.Run("InsertUserLink", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.InsertUserLinkParams{UserID: u.ID, LoginType: database.LoginTypeOIDC}
		dbm.EXPECT().InsertUserLink(gomock.Any(), arg).Return(database.UserLink{}, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdate)
	}))
	s.Run("UpdateUserDeletedByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserDeletedByID(gomock.Any(), u.ID).Return(nil).AnyTimes()
		check.Args(u.ID).Asserts(u, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateUserGithubComUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserGithubComUserIDParams{ID: u.ID}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserGithubComUserID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateUserHashedPassword", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserHashedPasswordParams{ID: u.ID}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserHashedPassword(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal).Returns()
	}))
	s.Run("UpdateUserHashedOneTimePasscode", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserHashedOneTimePasscodeParams{ID: u.ID}
		dbm.EXPECT().UpdateUserHashedOneTimePasscode(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateUserQuietHoursSchedule", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserQuietHoursScheduleParams{ID: u.ID}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserQuietHoursSchedule(gomock.Any(), arg).Return(database.User{}, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateUserLastSeenAt", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserLastSeenAtParams{ID: u.ID, UpdatedAt: u.UpdatedAt, LastSeenAt: u.LastSeenAt}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserLastSeenAt(gomock.Any(), arg).Return(u, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdate).Returns(u)
	}))
	s.Run("UpdateUserProfile", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserProfileParams{ID: u.ID, Email: u.Email, Username: u.Username, Name: u.Name, UpdatedAt: u.UpdatedAt}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserProfile(gomock.Any(), arg).Return(u, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal).Returns(u)
	}))
	s.Run("GetUserWorkspaceBuildParameters", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.GetUserWorkspaceBuildParametersParams{OwnerID: u.ID, TemplateID: uuid.Nil}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().GetUserWorkspaceBuildParameters(gomock.Any(), arg).Return([]database.GetUserWorkspaceBuildParametersRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionReadPersonal).Returns([]database.GetUserWorkspaceBuildParametersRow{})
	}))
	s.Run("GetUserThemePreference", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().GetUserThemePreference(gomock.Any(), u.ID).Return("light", nil).AnyTimes()
		check.Args(u.ID).Asserts(u, policy.ActionReadPersonal).Returns("light")
	}))
	s.Run("UpdateUserThemePreference", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		uc := database.UserConfig{UserID: u.ID, Key: "theme_preference", Value: "dark"}
		arg := database.UpdateUserThemePreferenceParams{UserID: u.ID, ThemePreference: uc.Value}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserThemePreference(gomock.Any(), arg).Return(uc, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal).Returns(uc)
	}))
	s.Run("GetUserTerminalFont", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().GetUserTerminalFont(gomock.Any(), u.ID).Return("ibm-plex-mono", nil).AnyTimes()
		check.Args(u.ID).Asserts(u, policy.ActionReadPersonal).Returns("ibm-plex-mono")
	}))
	s.Run("UpdateUserTerminalFont", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		uc := database.UserConfig{UserID: u.ID, Key: "terminal_font", Value: "ibm-plex-mono"}
		arg := database.UpdateUserTerminalFontParams{UserID: u.ID, TerminalFont: uc.Value}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserTerminalFont(gomock.Any(), arg).Return(uc, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal).Returns(uc)
	}))
	s.Run("UpdateUserStatus", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserStatusParams{ID: u.ID, Status: u.Status, UpdatedAt: u.UpdatedAt}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserStatus(gomock.Any(), arg).Return(u, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdate).Returns(u)
	}))
	s.Run("DeleteGitSSHKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.GitSSHKey{})
		dbm.EXPECT().GetGitSSHKey(gomock.Any(), key.UserID).Return(key, nil).AnyTimes()
		dbm.EXPECT().DeleteGitSSHKey(gomock.Any(), key.UserID).Return(nil).AnyTimes()
		check.Args(key.UserID).Asserts(rbac.ResourceUserObject(key.UserID), policy.ActionUpdatePersonal).Returns()
	}))
	s.Run("GetGitSSHKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.GitSSHKey{})
		dbm.EXPECT().GetGitSSHKey(gomock.Any(), key.UserID).Return(key, nil).AnyTimes()
		check.Args(key.UserID).Asserts(rbac.ResourceUserObject(key.UserID), policy.ActionReadPersonal).Returns(key)
	}))
	s.Run("InsertGitSSHKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.InsertGitSSHKeyParams{UserID: u.ID}
		dbm.EXPECT().InsertGitSSHKey(gomock.Any(), arg).Return(database.GitSSHKey{UserID: u.ID}, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateGitSSHKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.GitSSHKey{})
		arg := database.UpdateGitSSHKeyParams{UserID: key.UserID, UpdatedAt: key.UpdatedAt}
		dbm.EXPECT().GetGitSSHKey(gomock.Any(), key.UserID).Return(key, nil).AnyTimes()
		dbm.EXPECT().UpdateGitSSHKey(gomock.Any(), arg).Return(key, nil).AnyTimes()
		check.Args(arg).Asserts(key, policy.ActionUpdatePersonal).Returns(key)
	}))
	s.Run("GetExternalAuthLink", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		link := testutil.Fake(s.T(), faker, database.ExternalAuthLink{})
		arg := database.GetExternalAuthLinkParams{ProviderID: link.ProviderID, UserID: link.UserID}
		dbm.EXPECT().GetExternalAuthLink(gomock.Any(), arg).Return(link, nil).AnyTimes()
		check.Args(arg).Asserts(link, policy.ActionReadPersonal).Returns(link)
	}))
	s.Run("InsertExternalAuthLink", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.InsertExternalAuthLinkParams{ProviderID: uuid.NewString(), UserID: u.ID}
		dbm.EXPECT().InsertExternalAuthLink(gomock.Any(), arg).Return(database.ExternalAuthLink{}, nil).AnyTimes()
		check.Args(arg).Asserts(u, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateExternalAuthLinkRefreshToken", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		link := testutil.Fake(s.T(), faker, database.ExternalAuthLink{})
		arg := database.UpdateExternalAuthLinkRefreshTokenParams{OAuthRefreshToken: "", OAuthRefreshTokenKeyID: "", ProviderID: link.ProviderID, UserID: link.UserID, UpdatedAt: link.UpdatedAt}
		dbm.EXPECT().GetExternalAuthLink(gomock.Any(), database.GetExternalAuthLinkParams{ProviderID: link.ProviderID, UserID: link.UserID}).Return(link, nil).AnyTimes()
		dbm.EXPECT().UpdateExternalAuthLinkRefreshToken(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(link, policy.ActionUpdatePersonal)
	}))
	s.Run("UpdateExternalAuthLink", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		link := testutil.Fake(s.T(), faker, database.ExternalAuthLink{})
		arg := database.UpdateExternalAuthLinkParams{ProviderID: link.ProviderID, UserID: link.UserID, OAuthAccessToken: link.OAuthAccessToken, OAuthRefreshToken: link.OAuthRefreshToken, OAuthExpiry: link.OAuthExpiry, UpdatedAt: link.UpdatedAt}
		dbm.EXPECT().GetExternalAuthLink(gomock.Any(), database.GetExternalAuthLinkParams{ProviderID: link.ProviderID, UserID: link.UserID}).Return(link, nil).AnyTimes()
		dbm.EXPECT().UpdateExternalAuthLink(gomock.Any(), arg).Return(link, nil).AnyTimes()
		check.Args(arg).Asserts(link, policy.ActionUpdatePersonal).Returns(link)
	}))
	s.Run("UpdateUserLink", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		link := testutil.Fake(s.T(), faker, database.UserLink{})
		arg := database.UpdateUserLinkParams{OAuthAccessToken: link.OAuthAccessToken, OAuthRefreshToken: link.OAuthRefreshToken, OAuthExpiry: link.OAuthExpiry, UserID: link.UserID, LoginType: link.LoginType, Claims: database.UserLinkClaims{}}
		dbm.EXPECT().GetUserLinkByUserIDLoginType(gomock.Any(), database.GetUserLinkByUserIDLoginTypeParams{UserID: link.UserID, LoginType: link.LoginType}).Return(link, nil).AnyTimes()
		dbm.EXPECT().UpdateUserLink(gomock.Any(), arg).Return(link, nil).AnyTimes()
		check.Args(arg).Asserts(link, policy.ActionUpdatePersonal).Returns(link)
	}))
	s.Run("UpdateUserRoles", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{RBACRoles: []string{codersdk.RoleTemplateAdmin}})
		o := u
		o.RBACRoles = []string{codersdk.RoleUserAdmin}
		arg := database.UpdateUserRolesParams{GrantedRoles: []string{codersdk.RoleUserAdmin}, ID: u.ID}
		dbm.EXPECT().GetUserByID(gomock.Any(), u.ID).Return(u, nil).AnyTimes()
		dbm.EXPECT().UpdateUserRoles(gomock.Any(), arg).Return(o, nil).AnyTimes()
		check.Args(arg).Asserts(
			u, policy.ActionRead,
			rbac.ResourceAssignRole, policy.ActionAssign,
			rbac.ResourceAssignRole, policy.ActionUnassign,
		).Returns(o)
	}))
	s.Run("AllUserIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.User{})
		b := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().AllUserIDs(gomock.Any(), false).Return([]uuid.UUID{a.ID, b.ID}, nil).AnyTimes()
		check.Args(false).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(slice.New(a.ID, b.ID))
	}))
	s.Run("CustomRoles", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.CustomRolesParams{}
		dbm.EXPECT().CustomRoles(gomock.Any(), arg).Return([]database.CustomRole{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceAssignRole, policy.ActionRead).Returns([]database.CustomRole{})
	}))
	s.Run("Organization/DeleteCustomRole", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		orgID := uuid.New()
		arg := database.DeleteCustomRoleParams{Name: "role", OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true}}
		dbm.EXPECT().DeleteCustomRole(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceAssignOrgRole.InOrg(orgID), policy.ActionDelete)
	}))
	s.Run("Site/DeleteCustomRole", s.Mocked(func(_ *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.DeleteCustomRoleParams{Name: "role"}
		check.Args(arg).Asserts().Errors(dbauthz.NotAuthorizedError{Err: xerrors.New("custom roles must belong to an organization")})
	}))
	s.Run("Blank/UpdateCustomRole", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		orgID := uuid.New()
		arg := database.UpdateCustomRoleParams{Name: "name", DisplayName: "Test Name", OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true}}
		dbm.EXPECT().UpdateCustomRole(gomock.Any(), arg).Return(database.CustomRole{}, nil).AnyTimes()
		// Blank perms -> no escalation asserts beyond org role update
		check.Args(arg).Asserts(rbac.ResourceAssignOrgRole.InOrg(orgID), policy.ActionUpdate)
	}))
	s.Run("SitePermissions/UpdateCustomRole", s.Mocked(func(_ *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpdateCustomRoleParams{
			Name:           "",
			OrganizationID: uuid.NullUUID{UUID: uuid.Nil, Valid: false},
			DisplayName:    "Test Name",
			SitePermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionDelete, codersdk.ActionViewInsights},
			}), convertSDKPerm),
			OrgPermissions: nil,
			UserPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}), convertSDKPerm),
		}
		check.Args(arg).Asserts().Errors(dbauthz.NotAuthorizedError{Err: xerrors.New("custom roles must belong to an organization")})
	}))
	s.Run("OrgPermissions/UpdateCustomRole", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		orgID := uuid.New()
		arg := database.UpdateCustomRoleParams{
			Name:           "name",
			DisplayName:    "Test Name",
			OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true},
			OrgPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead},
			}), convertSDKPerm),
		}
		dbm.EXPECT().UpdateCustomRole(gomock.Any(), arg).Return(database.CustomRole{}, nil).AnyTimes()
		check.Args(arg).Asserts(
			rbac.ResourceAssignOrgRole.InOrg(orgID), policy.ActionUpdate,
			// Escalation checks
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionCreate,
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionRead,
		)
	}))
	s.Run("Blank/InsertCustomRole", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		orgID := uuid.New()
		arg := database.InsertCustomRoleParams{Name: "test", DisplayName: "Test Name", OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true}}
		dbm.EXPECT().InsertCustomRole(gomock.Any(), arg).Return(database.CustomRole{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceAssignOrgRole.InOrg(orgID), policy.ActionCreate)
	}))
	s.Run("SitePermissions/InsertCustomRole", s.Mocked(func(_ *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertCustomRoleParams{
			Name:        "test",
			DisplayName: "Test Name",
			SitePermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionDelete, codersdk.ActionViewInsights},
			}), convertSDKPerm),
			OrgPermissions: nil,
			UserPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}), convertSDKPerm),
		}
		check.Args(arg).Asserts().Errors(dbauthz.NotAuthorizedError{Err: xerrors.New("custom roles must belong to an organization")})
	}))
	s.Run("OrgPermissions/InsertCustomRole", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		orgID := uuid.New()
		arg := database.InsertCustomRoleParams{
			Name:           "test",
			DisplayName:    "Test Name",
			OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true},
			OrgPermissions: db2sdk.List(codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate: {codersdk.ActionCreate, codersdk.ActionRead},
			}), convertSDKPerm),
		}
		dbm.EXPECT().InsertCustomRole(gomock.Any(), arg).Return(database.CustomRole{}, nil).AnyTimes()
		check.Args(arg).Asserts(
			rbac.ResourceAssignOrgRole.InOrg(orgID), policy.ActionCreate,
			// Escalation checks
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionCreate,
			rbac.ResourceTemplate.InOrg(orgID), policy.ActionRead,
		)
	}))
	s.Run("GetUserStatusCounts", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetUserStatusCountsParams{StartTime: time.Now().Add(-time.Hour * 24 * 30), EndTime: time.Now(), Interval: int32((time.Hour * 24).Seconds())}
		dbm.EXPECT().GetUserStatusCounts(gomock.Any(), arg).Return([]database.GetUserStatusCountsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceUser, policy.ActionRead)
	}))
	s.Run("ValidateUserIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		ids := []uuid.UUID{u.ID}
		dbm.EXPECT().ValidateUserIDs(gomock.Any(), ids).Return(database.ValidateUserIDsRow{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestWorkspace() {
	// The Workspace object differs it's type based on whether it's dormant or
	// not, which is why we have two tests for it. To ensure we are actually
	// testing the correct RBAC objects, we also explicitly create the expected
	// object here rather than passing in the model.
	s.Run("GetWorkspaceByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		ws.DormantAt = sql.NullTime{
			Time:  time.Time{},
			Valid: false,
		}
		// Ensure the RBAC is not the dormant type.
		require.Equal(s.T(), rbac.ResourceWorkspace.Type, ws.RBACObject().Type)
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		// Explicitly create the expected object.
		expected := rbac.ResourceWorkspace.WithID(ws.ID).
			InOrg(ws.OrganizationID).
			WithOwner(ws.OwnerID.String()).
			WithGroupACL(ws.GroupACL.RBACACL()).
			WithACLUserList(ws.UserACL.RBACACL())
		check.Args(ws.ID).Asserts(expected, policy.ActionRead).Returns(ws)
	}))
	s.Run("DormantWorkspace/GetWorkspaceByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{
			DormantAt: sql.NullTime{
				Time:  time.Now().Add(-time.Hour),
				Valid: true,
			},
		})
		// Ensure the RBAC changed automatically.
		require.Equal(s.T(), rbac.ResourceWorkspaceDormant.Type, ws.RBACObject().Type)
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		// Explicitly create the expected object.
		expected := rbac.ResourceWorkspaceDormant.
			WithID(ws.ID).
			InOrg(ws.OrganizationID).
			WithOwner(ws.OwnerID.String())
		check.Args(ws.ID).Asserts(expected, policy.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaceByResourceID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		res := testutil.Fake(s.T(), faker, database.WorkspaceResource{})
		dbm.EXPECT().GetWorkspaceByResourceID(gomock.Any(), res.ID).Return(ws, nil).AnyTimes()
		check.Args(res.ID).Asserts(ws, policy.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaces", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetWorkspacesParams{}
		dbm.EXPECT().GetAuthorizedWorkspaces(gomock.Any(), arg, gomock.Any()).Return([]database.GetWorkspacesRow{}, nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(arg).Asserts()
	}))
	s.Run("GetWorkspaceAgentsForMetrics", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		row := testutil.Fake(s.T(), faker, database.GetWorkspaceAgentsForMetricsRow{})
		dbm.EXPECT().GetWorkspaceAgentsForMetrics(gomock.Any()).Return([]database.GetWorkspaceAgentsForMetricsRow{row}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceWorkspace, policy.ActionRead).Returns([]database.GetWorkspaceAgentsForMetricsRow{row})
	}))
	s.Run("GetWorkspacesForWorkspaceMetrics", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetWorkspacesForWorkspaceMetrics(gomock.Any()).Return([]database.GetWorkspacesForWorkspaceMetricsRow{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceWorkspace, policy.ActionRead)
	}))
	s.Run("GetAuthorizedWorkspaces", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetWorkspacesParams{}
		dbm.EXPECT().GetAuthorizedWorkspaces(gomock.Any(), arg, gomock.Any()).Return([]database.GetWorkspacesRow{}, nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(arg, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("GetWorkspacesAndAgentsByOwnerID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		dbm.EXPECT().GetAuthorizedWorkspacesAndAgentsByOwnerID(gomock.Any(), ws.OwnerID, gomock.Any()).Return([]database.GetWorkspacesAndAgentsByOwnerIDRow{}, nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(ws.OwnerID).Asserts()
	}))
	s.Run("GetAuthorizedWorkspacesAndAgentsByOwnerID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		dbm.EXPECT().GetAuthorizedWorkspacesAndAgentsByOwnerID(gomock.Any(), ws.OwnerID, gomock.Any()).Return([]database.GetWorkspacesAndAgentsByOwnerIDRow{}, nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(ws.OwnerID, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("GetWorkspaceBuildParametersByBuildIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{}
		dbm.EXPECT().GetAuthorizedWorkspaceBuildParametersByBuildIDs(gomock.Any(), ids, gomock.Any()).Return([]database.WorkspaceBuildParameter{}, nil).AnyTimes()
		// no asserts here because SQLFilter
		check.Args(ids).Asserts()
	}))
	s.Run("GetAuthorizedWorkspaceBuildParametersByBuildIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{}
		dbm.EXPECT().GetAuthorizedWorkspaceBuildParametersByBuildIDs(gomock.Any(), ids, gomock.Any()).Return([]database.WorkspaceBuildParameter{}, nil).AnyTimes()
		// no asserts here because SQLFilter
		check.Args(ids, emptyPreparedAuthorized{}).Asserts()
	}))
	s.Run("GetWorkspaceACLByID", s.Mocked(func(dbM *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		dbM.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbM.EXPECT().GetWorkspaceACLByID(gomock.Any(), ws.ID).Return(database.GetWorkspaceACLByIDRow{}, nil).AnyTimes()
		check.Args(ws.ID).Asserts(ws, policy.ActionShare)
	}))
	s.Run("UpdateWorkspaceACLByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.UpdateWorkspaceACLByIDParams{ID: w.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceACLByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionShare)
	}))
	s.Run("DeleteWorkspaceACLByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().DeleteWorkspaceACLByID(gomock.Any(), w.ID).Return(nil).AnyTimes()
		check.Args(w.ID).Asserts(w, policy.ActionShare)
	}))
	s.Run("GetLatestWorkspaceBuildByWorkspaceID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: w.ID})
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetLatestWorkspaceBuildByWorkspaceID(gomock.Any(), w.ID).Return(b, nil).AnyTimes()
		check.Args(w.ID).Asserts(w, policy.ActionRead).Returns(b)
	}))
	s.Run("GetWorkspaceAgentByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agt.ID).Return(agt, nil).AnyTimes()
		check.Args(agt.ID).Asserts(w, policy.ActionRead).Returns(agt)
	}))
	s.Run("GetWorkspaceAgentsByWorkspaceAndBuildNumber", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.GetWorkspaceAgentsByWorkspaceAndBuildNumberParams{WorkspaceID: w.ID, BuildNumber: 1}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentsByWorkspaceAndBuildNumber(gomock.Any(), arg).Return([]database.WorkspaceAgent{agt}, nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionRead).Returns([]database.WorkspaceAgent{agt})
	}))
	s.Run("GetWorkspaceAgentLifecycleStateByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		row := testutil.Fake(s.T(), faker, database.GetWorkspaceAgentLifecycleStateByIDRow{})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agt.ID).Return(agt, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentLifecycleStateByID(gomock.Any(), agt.ID).Return(row, nil).AnyTimes()
		check.Args(agt.ID).Asserts(w, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentMetadata", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.GetWorkspaceAgentMetadataParams{
			WorkspaceAgentID: agt.ID,
			Keys:             []string{"test"},
		}
		dt := testutil.Fake(s.T(), faker, database.WorkspaceAgentMetadatum{})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentMetadata(gomock.Any(), arg).Return([]database.WorkspaceAgentMetadatum{dt}, nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionRead).Returns([]database.WorkspaceAgentMetadatum{dt})
	}))
	s.Run("GetWorkspaceAgentByInstanceID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		authInstanceID := "instance-id"
		dbm.EXPECT().GetWorkspaceAgentByInstanceID(gomock.Any(), authInstanceID).Return(agt, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		check.Args(authInstanceID).Asserts(w, policy.ActionRead).Returns(agt)
	}))
	s.Run("UpdateWorkspaceAgentLifecycleStateByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.UpdateWorkspaceAgentLifecycleStateByIDParams{ID: agt.ID, LifecycleState: database.WorkspaceAgentLifecycleStateCreated}
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceAgentLifecycleStateByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentMetadata", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.UpdateWorkspaceAgentMetadataParams{WorkspaceAgentID: agt.ID}
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceAgentMetadata(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentLogOverflowByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.UpdateWorkspaceAgentLogOverflowByIDParams{ID: agt.ID, LogsOverflowed: true}
		dbm.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agt.ID).Return(agt, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceAgentLogOverflowByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAgentStartupByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.UpdateWorkspaceAgentStartupByIDParams{
			ID: agt.ID,
			Subsystems: []database.WorkspaceAgentSubsystem{
				database.WorkspaceAgentSubsystemEnvbox,
			},
		}
		dbm.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agt.ID).Return(agt, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceAgentStartupByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceAgentLogsAfter", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		log := testutil.Fake(s.T(), faker, database.WorkspaceAgentLog{})
		arg := database.GetWorkspaceAgentLogsAfterParams{AgentID: agt.ID}
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agt.ID).Return(agt, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentLogsAfter(gomock.Any(), arg).Return([]database.WorkspaceAgentLog{log}, nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceAgentLog{log})
	}))
	s.Run("GetWorkspaceAppByAgentIDAndSlug", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		app := testutil.Fake(s.T(), faker, database.WorkspaceApp{AgentID: agt.ID})
		arg := database.GetWorkspaceAppByAgentIDAndSlugParams{AgentID: agt.ID, Slug: app.Slug}
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAppByAgentIDAndSlug(gomock.Any(), arg).Return(app, nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionRead).Returns(app)
	}))
	s.Run("GetWorkspaceAppsByAgentID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		appA := testutil.Fake(s.T(), faker, database.WorkspaceApp{})
		appB := testutil.Fake(s.T(), faker, database.WorkspaceApp{AgentID: appA.AgentID})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), appA.AgentID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAppsByAgentID(gomock.Any(), appA.AgentID).Return([]database.WorkspaceApp{appA, appB}, nil).AnyTimes()
		check.Args(appA.AgentID).Asserts(ws, policy.ActionRead).Returns(slice.New(appA, appB))
	}))
	s.Run("GetWorkspaceBuildByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID})
		dbm.EXPECT().GetWorkspaceBuildByID(gomock.Any(), build.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		check.Args(build.ID).Asserts(ws, policy.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByJobID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID})
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), build.JobID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		check.Args(build.JobID).Asserts(ws, policy.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildByWorkspaceIDAndBuildNumber", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID})
		arg := database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{WorkspaceID: ws.ID, BuildNumber: build.BuildNumber}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByWorkspaceIDAndBuildNumber(gomock.Any(), arg).Return(build, nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionRead).Returns(build)
	}))
	s.Run("GetWorkspaceBuildParameters", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID})
		p1 := testutil.Fake(s.T(), faker, database.WorkspaceBuildParameter{})
		p2 := testutil.Fake(s.T(), faker, database.WorkspaceBuildParameter{})
		dbm.EXPECT().GetWorkspaceBuildByID(gomock.Any(), build.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildParameters(gomock.Any(), build.ID).Return([]database.WorkspaceBuildParameter{p1, p2}, nil).AnyTimes()
		check.Args(build.ID).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceBuildParameter{p1, p2})
	}))
	s.Run("GetWorkspaceBuildsByWorkspaceID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		b1 := testutil.Fake(s.T(), faker, database.WorkspaceBuild{})
		arg := database.GetWorkspaceBuildsByWorkspaceIDParams{WorkspaceID: ws.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildsByWorkspaceID(gomock.Any(), arg).Return([]database.WorkspaceBuild{b1}, nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceBuild{b1})
	}))
	s.Run("GetWorkspaceByAgentID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(ws, nil).AnyTimes()
		check.Args(agt.ID).Asserts(ws, policy.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaceAgentsInLatestBuildByWorkspaceID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentsInLatestBuildByWorkspaceID(gomock.Any(), ws.ID).Return([]database.WorkspaceAgent{agt}, nil).AnyTimes()
		check.Args(ws.ID).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceAgent{agt})
	}))
	s.Run("GetWorkspaceByOwnerIDAndName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.GetWorkspaceByOwnerIDAndNameParams{
			OwnerID: ws.OwnerID,
			Deleted: ws.Deleted,
			Name:    ws.Name,
		}
		dbm.EXPECT().GetWorkspaceByOwnerIDAndName(gomock.Any(), arg).Return(ws, nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionRead).Returns(ws)
	}))
	s.Run("GetWorkspaceResourceByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID})
		job := testutil.Fake(s.T(), faker, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		res := testutil.Fake(s.T(), faker, database.WorkspaceResource{JobID: build.JobID})
		dbm.EXPECT().GetWorkspaceResourceByID(gomock.Any(), res.ID).Return(res, nil).AnyTimes()
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), res.JobID).Return(job, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), res.JobID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), build.WorkspaceID).Return(ws, nil).AnyTimes()
		check.Args(res.ID).Asserts(ws, policy.ActionRead).Returns(res)
	}))
	s.Run("Build/GetWorkspaceResourcesByJobID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: ws.ID})
		job := testutil.Fake(s.T(), faker, database.ProvisionerJob{ID: build.JobID, Type: database.ProvisionerJobTypeWorkspaceBuild})
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), job.ID).Return(job, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), job.ID).Return(build, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceResourcesByJobID(gomock.Any(), job.ID).Return([]database.WorkspaceResource{}, nil).AnyTimes()
		check.Args(job.ID).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceResource{})
	}))
	s.Run("Template/GetWorkspaceResourcesByJobID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		job := testutil.Fake(s.T(), faker, database.ProvisionerJob{ID: v.JobID, Type: database.ProvisionerJobTypeTemplateVersionImport})
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), job.ID).Return(job, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), job.ID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceResourcesByJobID(gomock.Any(), job.ID).Return([]database.WorkspaceResource{}, nil).AnyTimes()
		check.Args(job.ID).Asserts(v.RBACObject(tpl), []policy.Action{policy.ActionRead, policy.ActionRead}).Returns([]database.WorkspaceResource{})
	}))
	s.Run("InsertWorkspace", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		arg := database.InsertWorkspaceParams{
			ID:               uuid.New(),
			OwnerID:          uuid.New(),
			OrganizationID:   uuid.New(),
			AutomaticUpdates: database.AutomaticUpdatesNever,
			TemplateID:       tpl.ID,
		}
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspace(gomock.Any(), arg).Return(database.WorkspaceTable{}, nil).AnyTimes()
		check.Args(arg).Asserts(tpl, policy.ActionRead, tpl, policy.ActionUse, rbac.ResourceWorkspace.WithOwner(arg.OwnerID.String()).InOrg(arg.OrganizationID), policy.ActionCreate)
	}))
	s.Run("Start/InsertWorkspaceBuild", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t := testutil.Fake(s.T(), faker, database.Template{})
		// Ensure active-version requirement is disabled to avoid extra RBAC checks.
		// This case is covered by the `Start/RequireActiveVersion` test.
		t.RequireActiveVersion = false
		w := testutil.Fake(s.T(), faker, database.Workspace{TemplateID: t.ID})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t.ID, Valid: true}})
		pj := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			TemplateVersionID: tv.ID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			JobID:             pj.ID,
		}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t.ID).Return(t, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceBuild(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionWorkspaceStart)
	}))
	s.Run("Stop/InsertWorkspaceBuild", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		pj := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			TemplateVersionID: tv.ID,
			Transition:        database.WorkspaceTransitionStop,
			Reason:            database.BuildReasonInitiator,
			JobID:             pj.ID,
		}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceBuild(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionWorkspaceStop)
	}))
	s.Run("Start/RequireActiveVersion/VersionMismatch/InsertWorkspaceBuild", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		// Require active version and mismatch triggers template update authorization
		t := testutil.Fake(s.T(), faker, database.Template{RequireActiveVersion: true, ActiveVersionID: uuid.New()})
		w := testutil.Fake(s.T(), faker, database.Workspace{TemplateID: t.ID})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t.ID, Valid: true}})
		pj := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: v.ID,
			JobID:             pj.ID,
		}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t.ID).Return(t, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceBuild(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(
			w, policy.ActionWorkspaceStart,
			t, policy.ActionUpdate,
		)
	}))
	s.Run("Start/RequireActiveVersion/VersionsMatch/InsertWorkspaceBuild", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		t := testutil.Fake(s.T(), faker, database.Template{RequireActiveVersion: true, ActiveVersionID: v.ID})
		w := testutil.Fake(s.T(), faker, database.Workspace{TemplateID: t.ID})
		pj := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			Transition:        database.WorkspaceTransitionStart,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: v.ID,
			JobID:             pj.ID,
		}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t.ID).Return(t, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceBuild(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(
			w, policy.ActionWorkspaceStart,
		)
	}))
	s.Run("Delete/InsertWorkspaceBuild", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		pj := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			Transition:        database.WorkspaceTransitionDelete,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: tv.ID,
			JobID:             pj.ID,
		}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceBuild(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionDelete)
	}))
	s.Run("InsertWorkspaceBuildParameters", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: w.ID})
		arg := database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: b.ID,
			Name:             []string{"foo", "bar"},
			Value:            []string{"baz", "qux"},
		}
		dbm.EXPECT().GetWorkspaceBuildByID(gomock.Any(), b.ID).Return(b, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceBuildParameters(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspace", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		expected := testutil.Fake(s.T(), faker, database.WorkspaceTable{ID: w.ID})
		expected.Name = ""
		arg := database.UpdateWorkspaceParams{ID: w.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspace(gomock.Any(), arg).Return(expected, nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns(expected)
	}))
	s.Run("UpdateWorkspaceDormantDeletingAt", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.UpdateWorkspaceDormantDeletingAtParams{ID: w.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceDormantDeletingAt(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.WorkspaceTable{ID: w.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceAutomaticUpdates", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.UpdateWorkspaceAutomaticUpdatesParams{ID: w.ID, AutomaticUpdates: database.AutomaticUpdatesAlways}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceAutomaticUpdates(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceAppHealthByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		app := testutil.Fake(s.T(), faker, database.WorkspaceApp{})
		arg := database.UpdateWorkspaceAppHealthByIDParams{ID: app.ID, Health: database.WorkspaceAppHealthDisabled}
		dbm.EXPECT().GetWorkspaceByWorkspaceAppID(gomock.Any(), app.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceAppHealthByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceAutostart", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.UpdateWorkspaceAutostartParams{ID: w.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceAutostart(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceBuildDeadlineByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{WorkspaceID: w.ID})
		arg := database.UpdateWorkspaceBuildDeadlineByIDParams{ID: b.ID, UpdatedAt: b.UpdatedAt, Deadline: b.Deadline}
		dbm.EXPECT().GetWorkspaceBuildByID(gomock.Any(), b.ID).Return(b, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceBuildDeadlineByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceBuildFlagsByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		o := testutil.Fake(s.T(), faker, database.Organization{})
		tpl := testutil.Fake(s.T(), faker, database.Template{
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		w := testutil.Fake(s.T(), faker, database.Workspace{
			TemplateID:     tpl.ID,
			OrganizationID: o.ID,
			OwnerID:        u.ID,
		})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{
			JobID:             j.ID,
			WorkspaceID:       w.ID,
			TemplateVersionID: tv.ID,
		})
		res := testutil.Fake(s.T(), faker, database.WorkspaceResource{JobID: b.JobID})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{ResourceID: res.ID})
		_ = testutil.Fake(s.T(), faker, database.WorkspaceApp{AgentID: agt.ID})

		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByID(gomock.Any(), b.ID).Return(b, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceBuildFlagsByID(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		check.Args(database.UpdateWorkspaceBuildFlagsByIDParams{
			ID:               b.ID,
			HasAITask:        sql.NullBool{Bool: true, Valid: true},
			HasExternalAgent: sql.NullBool{Bool: true, Valid: true},
			UpdatedAt:        b.UpdatedAt,
		}).Asserts(w, policy.ActionUpdate)
	}))
	s.Run("SoftDeleteWorkspaceByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		w.Deleted = true
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceDeletedByID(gomock.Any(), database.UpdateWorkspaceDeletedByIDParams{ID: w.ID, Deleted: true}).Return(nil).AnyTimes()
		check.Args(w.ID).Asserts(w, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceDeletedByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{Deleted: true})
		arg := database.UpdateWorkspaceDeletedByIDParams{ID: w.ID, Deleted: true}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceDeletedByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionDelete).Returns()
	}))
	s.Run("UpdateWorkspaceLastUsedAt", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.UpdateWorkspaceLastUsedAtParams{ID: w.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceLastUsedAt(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceNextStartAt", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), gofakeit.New(0), database.Workspace{})
		arg := database.UpdateWorkspaceNextStartAtParams{ID: ws.ID, NextStartAt: sql.NullTime{Valid: true, Time: dbtime.Now()}}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceNextStartAt(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionUpdate)
	}))
	s.Run("BatchUpdateWorkspaceNextStartAt", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.BatchUpdateWorkspaceNextStartAtParams{IDs: []uuid.UUID{uuid.New()}, NextStartAts: []time.Time{dbtime.Now()}}
		dbm.EXPECT().BatchUpdateWorkspaceNextStartAt(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceWorkspace.All(), policy.ActionUpdate)
	}))
	s.Run("BatchUpdateWorkspaceLastUsedAt", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w1 := testutil.Fake(s.T(), faker, database.Workspace{})
		w2 := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.BatchUpdateWorkspaceLastUsedAtParams{IDs: []uuid.UUID{w1.ID, w2.ID}}
		dbm.EXPECT().BatchUpdateWorkspaceLastUsedAt(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceWorkspace.All(), policy.ActionUpdate).Returns()
	}))
	s.Run("UpdateWorkspaceTTL", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.UpdateWorkspaceTTLParams{ID: w.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UpdateWorkspaceTTL(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceByWorkspaceAppID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		app := testutil.Fake(s.T(), faker, database.WorkspaceApp{})
		dbm.EXPECT().GetWorkspaceByWorkspaceAppID(gomock.Any(), app.ID).Return(w, nil).AnyTimes()
		check.Args(app.ID).Asserts(w, policy.ActionRead)
	}))
	s.Run("ActivityBumpWorkspace", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.ActivityBumpWorkspaceParams{WorkspaceID: w.ID}
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().ActivityBumpWorkspace(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("FavoriteWorkspace", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().FavoriteWorkspace(gomock.Any(), w.ID).Return(nil).AnyTimes()
		check.Args(w.ID).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("UnfavoriteWorkspace", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), w.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().UnfavoriteWorkspace(gomock.Any(), w.ID).Return(nil).AnyTimes()
		check.Args(w.ID).Asserts(w, policy.ActionUpdate).Returns()
	}))
	s.Run("GetWorkspaceAgentDevcontainersByAgentID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		w := testutil.Fake(s.T(), faker, database.Workspace{})
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		d := testutil.Fake(s.T(), faker, database.WorkspaceAgentDevcontainer{WorkspaceAgentID: agt.ID})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agt.ID).Return(w, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentByID(gomock.Any(), agt.ID).Return(agt, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentDevcontainersByAgentID(gomock.Any(), agt.ID).Return([]database.WorkspaceAgentDevcontainer{d}, nil).AnyTimes()
		check.Args(agt.ID).Asserts(w, policy.ActionRead).Returns([]database.WorkspaceAgentDevcontainer{d})
	}))
	s.Run("GetRegularWorkspaceCreateMetrics", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceWorkspace.All(), policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestWorkspacePortSharing() {
	s.Run("UpsertWorkspaceAgentPortShare", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		org := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
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
		org := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		ps := dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(database.GetWorkspaceAgentPortShareParams{
			WorkspaceID: ps.WorkspaceID,
			AgentName:   ps.AgentName,
			Port:        ps.Port,
		}).Asserts(ws, policy.ActionRead).Returns(ps)
	}))
	s.Run("ListWorkspaceAgentPortShares", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		org := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		ps := dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(ws.ID).Asserts(ws, policy.ActionRead).Returns([]database.WorkspaceAgentPortShare{ps})
	}))
	s.Run("DeleteWorkspaceAgentPortShare", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		org := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		ps := dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(database.DeleteWorkspaceAgentPortShareParams{
			WorkspaceID: ps.WorkspaceID,
			AgentName:   ps.AgentName,
			Port:        ps.Port,
		}).Asserts(ws, policy.ActionUpdate).Returns()
	}))
	s.Run("DeleteWorkspaceAgentPortSharesByTemplate", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		org := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		_ = dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(tpl.ID).Asserts(tpl, policy.ActionUpdate).Returns()
	}))
	s.Run("ReduceWorkspaceAgentShareLevelToAuthenticatedByTemplate", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		org := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		ws := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		_ = dbgen.WorkspaceAgentPortShare(s.T(), db, database.WorkspaceAgentPortShare{WorkspaceID: ws.ID})
		check.Args(tpl.ID).Asserts(tpl, policy.ActionUpdate).Returns()
	}))
}

func (s *MethodTestSuite) TestTasks() {
	s.Run("GetTaskByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		task := testutil.Fake(s.T(), faker, database.Task{})
		dbm.EXPECT().GetTaskByID(gomock.Any(), task.ID).Return(task, nil).AnyTimes()
		check.Args(task.ID).Asserts(task, policy.ActionRead).Returns(task)
	}))
	s.Run("GetTaskByOwnerIDAndName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		task := testutil.Fake(s.T(), faker, database.Task{})
		dbm.EXPECT().GetTaskByOwnerIDAndName(gomock.Any(), database.GetTaskByOwnerIDAndNameParams{
			OwnerID: task.OwnerID,
			Name:    task.Name,
		}).Return(task, nil).AnyTimes()
		check.Args(database.GetTaskByOwnerIDAndNameParams{
			OwnerID: task.OwnerID,
			Name:    task.Name,
		}).Asserts(task, policy.ActionRead).Returns(task)
	}))
	s.Run("DeleteTask", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		task := testutil.Fake(s.T(), faker, database.Task{})
		arg := database.DeleteTaskParams{
			ID:        task.ID,
			DeletedAt: dbtime.Now(),
		}
		dbm.EXPECT().GetTaskByID(gomock.Any(), task.ID).Return(task, nil).AnyTimes()
		dbm.EXPECT().DeleteTask(gomock.Any(), arg).Return(database.TaskTable{}, nil).AnyTimes()
		check.Args(arg).Asserts(task, policy.ActionDelete).Returns(database.TaskTable{})
	}))
	s.Run("InsertTask", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			OrganizationID: tpl.OrganizationID,
		})

		arg := testutil.Fake(s.T(), faker, database.InsertTaskParams{
			OrganizationID:    tpl.OrganizationID,
			TemplateVersionID: tv.ID,
		})

		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().InsertTask(gomock.Any(), arg).Return(database.TaskTable{}, nil).AnyTimes()

		check.Args(arg).Asserts(
			tpl, policy.ActionRead,
			rbac.ResourceTask.InOrg(arg.OrganizationID).WithOwner(arg.OwnerID.String()), policy.ActionCreate,
		).Returns(database.TaskTable{})
	}))
	s.Run("UpsertTaskWorkspaceApp", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		task := testutil.Fake(s.T(), faker, database.Task{})
		arg := database.UpsertTaskWorkspaceAppParams{
			TaskID:               task.ID,
			WorkspaceBuildNumber: 1,
		}

		dbm.EXPECT().GetTaskByID(gomock.Any(), task.ID).Return(task, nil).AnyTimes()
		dbm.EXPECT().UpsertTaskWorkspaceApp(gomock.Any(), arg).Return(database.TaskWorkspaceApp{}, nil).AnyTimes()

		check.Args(arg).Asserts(task, policy.ActionUpdate).Returns(database.TaskWorkspaceApp{})
	}))
	s.Run("UpdateTaskWorkspaceID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		task := testutil.Fake(s.T(), faker, database.Task{})
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		arg := database.UpdateTaskWorkspaceIDParams{
			ID:          task.ID,
			WorkspaceID: uuid.NullUUID{UUID: ws.ID, Valid: true},
		}

		dbm.EXPECT().GetTaskByID(gomock.Any(), task.ID).Return(task, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), ws.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().UpdateTaskWorkspaceID(gomock.Any(), arg).Return(database.TaskTable{}, nil).AnyTimes()

		check.Args(arg).Asserts(task, policy.ActionUpdate, ws, policy.ActionUpdate).Returns(database.TaskTable{})
	}))
	s.Run("GetTaskByWorkspaceID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		task := testutil.Fake(s.T(), faker, database.Task{})
		task.WorkspaceID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		dbm.EXPECT().GetTaskByWorkspaceID(gomock.Any(), task.WorkspaceID.UUID).Return(task, nil).AnyTimes()
		check.Args(task.WorkspaceID.UUID).Asserts(task, policy.ActionRead).Returns(task)
	}))
	s.Run("ListTasks", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u1 := testutil.Fake(s.T(), faker, database.User{})
		u2 := testutil.Fake(s.T(), faker, database.User{})
		org1 := testutil.Fake(s.T(), faker, database.Organization{})
		org2 := testutil.Fake(s.T(), faker, database.Organization{})
		_ = testutil.Fake(s.T(), faker, database.OrganizationMember{UserID: u1.ID, OrganizationID: org1.ID})
		_ = testutil.Fake(s.T(), faker, database.OrganizationMember{UserID: u2.ID, OrganizationID: org2.ID})
		t1 := testutil.Fake(s.T(), faker, database.Task{OwnerID: u1.ID})
		t2 := testutil.Fake(s.T(), faker, database.Task{OwnerID: u2.ID})
		dbm.EXPECT().ListTasks(gomock.Any(), gomock.Any()).Return([]database.Task{t1, t2}, nil).AnyTimes()
		check.Args(database.ListTasksParams{}).Asserts(t1, policy.ActionRead, t2, policy.ActionRead).Returns([]database.Task{t1, t2})
	}))
}

func (s *MethodTestSuite) TestProvisionerKeys() {
	s.Run("InsertProvisionerKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		pk := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID})
		arg := database.InsertProvisionerKeyParams{ID: pk.ID, CreatedAt: pk.CreatedAt, OrganizationID: pk.OrganizationID, Name: pk.Name, HashedSecret: pk.HashedSecret}
		dbm.EXPECT().InsertProvisionerKey(gomock.Any(), arg).Return(pk, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerDaemon.InOrg(org.ID).WithID(pk.ID), policy.ActionCreate).Returns(pk)
	}))
	s.Run("GetProvisionerKeyByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		pk := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID})
		dbm.EXPECT().GetProvisionerKeyByID(gomock.Any(), pk.ID).Return(pk, nil).AnyTimes()
		check.Args(pk.ID).Asserts(pk, policy.ActionRead).Returns(pk)
	}))
	s.Run("GetProvisionerKeyByHashedSecret", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		pk := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID, HashedSecret: []byte("foo")})
		dbm.EXPECT().GetProvisionerKeyByHashedSecret(gomock.Any(), []byte("foo")).Return(pk, nil).AnyTimes()
		check.Args([]byte("foo")).Asserts(pk, policy.ActionRead).Returns(pk)
	}))
	s.Run("GetProvisionerKeyByName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		pk := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID})
		arg := database.GetProvisionerKeyByNameParams{OrganizationID: org.ID, Name: pk.Name}
		dbm.EXPECT().GetProvisionerKeyByName(gomock.Any(), arg).Return(pk, nil).AnyTimes()
		check.Args(arg).Asserts(pk, policy.ActionRead).Returns(pk)
	}))
	s.Run("ListProvisionerKeysByOrganization", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		a := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID})
		b := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID})
		dbm.EXPECT().ListProvisionerKeysByOrganization(gomock.Any(), org.ID).Return([]database.ProvisionerKey{a, b}, nil).AnyTimes()
		check.Args(org.ID).Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns([]database.ProvisionerKey{a, b})
	}))
	s.Run("ListProvisionerKeysByOrganizationExcludeReserved", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		pk := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID})
		dbm.EXPECT().ListProvisionerKeysByOrganizationExcludeReserved(gomock.Any(), org.ID).Return([]database.ProvisionerKey{pk}, nil).AnyTimes()
		check.Args(org.ID).Asserts(pk, policy.ActionRead).Returns([]database.ProvisionerKey{pk})
	}))
	s.Run("DeleteProvisionerKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		pk := testutil.Fake(s.T(), faker, database.ProvisionerKey{OrganizationID: org.ID})
		dbm.EXPECT().GetProvisionerKeyByID(gomock.Any(), pk.ID).Return(pk, nil).AnyTimes()
		dbm.EXPECT().DeleteProvisionerKey(gomock.Any(), pk.ID).Return(nil).AnyTimes()
		check.Args(pk.ID).Asserts(pk, policy.ActionDelete).Returns()
	}))
}

func (s *MethodTestSuite) TestExtraMethods() {
	s.Run("GetProvisionerDaemons", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		d, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			Provisioners: []database.ProvisionerType{},
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		check.Args().Asserts(d, policy.ActionRead)
	}))
	s.Run("GetProvisionerDaemonsByOrganization", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		org := dbgen.Organization(s.T(), db, database.Organization{})
		d, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{},
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		ds, err := db.GetProvisionerDaemonsByOrganization(context.Background(), database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: org.ID})
		s.NoError(err, "get provisioner daemon by org")
		check.Args(database.GetProvisionerDaemonsByOrganizationParams{OrganizationID: org.ID}).Asserts(d, policy.ActionRead).Returns(ds)
	}))
	s.Run("GetProvisionerDaemonsWithStatusByOrganization", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		d := dbgen.ProvisionerDaemon(s.T(), db, database.ProvisionerDaemon{
			OrganizationID: org.ID,
			Tags: map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			},
		})
		ds, err := db.GetProvisionerDaemonsWithStatusByOrganization(context.Background(), database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID:  org.ID,
			StaleIntervalMS: 24 * time.Hour.Milliseconds(),
		})
		s.NoError(err, "get provisioner daemon with status by org")
		check.Args(database.GetProvisionerDaemonsWithStatusByOrganizationParams{
			OrganizationID:  org.ID,
			StaleIntervalMS: 24 * time.Hour.Milliseconds(),
		}).Asserts(d, policy.ActionRead).Returns(ds)
	}))
	s.Run("GetEligibleProvisionerDaemonsByProvisionerJobIDs", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		org := dbgen.Organization(s.T(), db, database.Organization{})
		tags := database.StringMap(map[string]string{
			provisionersdk.TagScope: provisionersdk.ScopeOrganization,
		})
		j, err := db.InsertProvisionerJob(context.Background(), database.InsertProvisionerJobParams{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Tags:           tags,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			Input:          json.RawMessage("{}"),
		})
		s.NoError(err, "insert provisioner job")
		d, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			OrganizationID: org.ID,
			Tags:           tags,
			Provisioners:   []database.ProvisionerType{database.ProvisionerTypeEcho},
		})
		s.NoError(err, "insert provisioner daemon")
		ds, err := db.GetEligibleProvisionerDaemonsByProvisionerJobIDs(context.Background(), []uuid.UUID{j.ID})
		s.NoError(err, "get provisioner daemon by org")
		check.Args(uuid.UUIDs{j.ID}).Asserts(d, policy.ActionRead).Returns(ds)
	}))
	s.Run("DeleteOldProvisionerDaemons", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		_, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			Provisioners: []database.ProvisionerType{},
			Tags: database.StringMap(map[string]string{
				provisionersdk.TagScope: provisionersdk.ScopeOrganization,
			}),
		})
		s.NoError(err, "insert provisioner daemon")
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("UpdateProvisionerDaemonLastSeenAt", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		d, err := db.UpsertProvisionerDaemon(context.Background(), database.UpsertProvisionerDaemonParams{
			Provisioners: []database.ProvisionerType{},
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
	s.Run("GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner", s.Subtest(func(db database.Store, check *expects) {
		org := dbgen.Organization(s.T(), db, database.Organization{})
		user := dbgen.User(s.T(), db, database.User{})
		tags := database.StringMap(map[string]string{
			provisionersdk.TagScope: provisionersdk.ScopeOrganization,
		})
		t := dbgen.Template(s.T(), db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{OrganizationID: org.ID, CreatedBy: user.ID, TemplateID: uuid.NullUUID{UUID: t.ID, Valid: true}})
		j1 := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte(`{"template_version_id":"` + tv.ID.String() + `"}`),
			Tags:           tags,
		})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{OrganizationID: org.ID, OwnerID: user.ID, TemplateID: t.ID})
		wbID := uuid.New()
		j2 := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			OrganizationID: org.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte(`{"workspace_build_id":"` + wbID.String() + `"}`),
			Tags:           tags,
		})
		dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{ID: wbID, WorkspaceID: w.ID, TemplateVersionID: tv.ID, JobID: j2.ID})

		ds, err := db.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisioner(context.Background(), database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams{
			OrganizationID: org.ID,
			InitiatorID:    uuid.Nil,
		})
		s.NoError(err, "get provisioner jobs by org")
		check.Args(database.GetProvisionerJobsByOrganizationAndStatusWithQueuePositionAndProvisionerParams{
			OrganizationID: org.ID,
			InitiatorID:    uuid.Nil,
		}).Asserts(j1, policy.ActionRead, j2, policy.ActionRead).Returns(ds)
	}))
}

func (s *MethodTestSuite) TestTailnetFunctions() {
	s.Run("CleanTailnetCoordinators", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete)
	}))
	s.Run("CleanTailnetLostPeers", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete)
	}))
	s.Run("CleanTailnetTunnels", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete)
	}))
	s.Run("DeleteAllTailnetClientSubscriptions", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.DeleteAllTailnetClientSubscriptionsParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete)
	}))
	s.Run("DeleteAllTailnetTunnels", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.DeleteAllTailnetTunnelsParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete)
	}))
	s.Run("DeleteCoordinator", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete)
	}))
	s.Run("DeleteTailnetAgent", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.DeleteTailnetAgentParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate).Errors(sql.ErrNoRows)
	}))
	s.Run("DeleteTailnetClient", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.DeleteTailnetClientParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).Errors(sql.ErrNoRows)
	}))
	s.Run("DeleteTailnetClientSubscription", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.DeleteTailnetClientSubscriptionParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete)
	}))
	s.Run("DeleteTailnetPeer", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.DeleteTailnetPeerParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).Errors(sql.ErrNoRows)
	}))
	s.Run("DeleteTailnetTunnel", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(database.DeleteTailnetTunnelParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionDelete).Errors(sql.ErrNoRows)
	}))
	s.Run("GetAllTailnetAgents", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetTailnetAgents", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetTailnetClientsForAgent", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetTailnetPeers", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetTailnetTunnelPeerBindings", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetTailnetTunnelPeerIDs", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetAllTailnetCoordinators", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetAllTailnetPeers", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("GetAllTailnetTunnels", s.Subtest(func(_ database.Store, check *expects) {
		check.Args().
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionRead)
	}))
	s.Run("UpsertTailnetAgent", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		check.Args(database.UpsertTailnetAgentParams{Node: json.RawMessage("{}")}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate)
	}))
	s.Run("UpsertTailnetClient", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		check.Args(database.UpsertTailnetClientParams{Node: json.RawMessage("{}")}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate)
	}))
	s.Run("UpsertTailnetClientSubscription", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		check.Args(database.UpsertTailnetClientSubscriptionParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate)
	}))
	s.Run("UpsertTailnetCoordinator", s.Subtest(func(_ database.Store, check *expects) {
		check.Args(uuid.New()).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate)
	}))
	s.Run("UpsertTailnetPeer", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		check.Args(database.UpsertTailnetPeerParams{
			Status: database.TailnetStatusOk,
		}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionCreate)
	}))
	s.Run("UpsertTailnetTunnel", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		check.Args(database.UpsertTailnetTunnelParams{}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionCreate)
	}))
	s.Run("UpdateTailnetPeerStatusByCoordinator", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		check.Args(database.UpdateTailnetPeerStatusByCoordinatorParams{Status: database.TailnetStatusOk}).
			Asserts(rbac.ResourceTailnetCoordinator, policy.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestDBCrypt() {
	s.Run("GetDBCryptKeys", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetDBCryptKeys(gomock.Any()).Return([]database.DBCryptKey{}, nil).AnyTimes()
		check.Args().
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.DBCryptKey{})
	}))
	s.Run("InsertDBCryptKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().InsertDBCryptKey(gomock.Any(), database.InsertDBCryptKeyParams{}).Return(nil).AnyTimes()
		check.Args(database.InsertDBCryptKeyParams{}).
			Asserts(rbac.ResourceSystem, policy.ActionCreate).
			Returns()
	}))
	s.Run("RevokeDBCryptKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().RevokeDBCryptKey(gomock.Any(), "revoke me").Return(nil).AnyTimes()
		check.Args("revoke me").
			Asserts(rbac.ResourceSystem, policy.ActionUpdate).
			Returns()
	}))
}

func (s *MethodTestSuite) TestCryptoKeys() {
	s.Run("GetCryptoKeys", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetCryptoKeys(gomock.Any()).Return([]database.CryptoKey{}, nil).AnyTimes()
		check.Args().
			Asserts(rbac.ResourceCryptoKey, policy.ActionRead)
	}))
	s.Run("InsertCryptoKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertCryptoKeyParams{Feature: database.CryptoKeyFeatureWorkspaceAppsAPIKey}
		dbm.EXPECT().InsertCryptoKey(gomock.Any(), arg).Return(database.CryptoKey{}, nil).AnyTimes()
		check.Args(arg).
			Asserts(rbac.ResourceCryptoKey, policy.ActionCreate)
	}))
	s.Run("DeleteCryptoKey", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.CryptoKey{Feature: database.CryptoKeyFeatureWorkspaceAppsAPIKey, Sequence: 4})
		arg := database.DeleteCryptoKeyParams{Feature: key.Feature, Sequence: key.Sequence}
		dbm.EXPECT().DeleteCryptoKey(gomock.Any(), arg).Return(key, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceCryptoKey, policy.ActionDelete)
	}))
	s.Run("GetCryptoKeyByFeatureAndSequence", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.CryptoKey{Feature: database.CryptoKeyFeatureWorkspaceAppsAPIKey, Sequence: 4})
		arg := database.GetCryptoKeyByFeatureAndSequenceParams{Feature: key.Feature, Sequence: key.Sequence}
		dbm.EXPECT().GetCryptoKeyByFeatureAndSequence(gomock.Any(), arg).Return(key, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceCryptoKey, policy.ActionRead).Returns(key)
	}))
	s.Run("GetLatestCryptoKeyByFeature", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		feature := database.CryptoKeyFeatureWorkspaceAppsAPIKey
		dbm.EXPECT().GetLatestCryptoKeyByFeature(gomock.Any(), feature).Return(database.CryptoKey{}, nil).AnyTimes()
		check.Args(feature).Asserts(rbac.ResourceCryptoKey, policy.ActionRead)
	}))
	s.Run("UpdateCryptoKeyDeletesAt", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		key := testutil.Fake(s.T(), faker, database.CryptoKey{Feature: database.CryptoKeyFeatureWorkspaceAppsAPIKey, Sequence: 4})
		arg := database.UpdateCryptoKeyDeletesAtParams{Feature: key.Feature, Sequence: key.Sequence, DeletesAt: sql.NullTime{Time: time.Now(), Valid: true}}
		dbm.EXPECT().UpdateCryptoKeyDeletesAt(gomock.Any(), arg).Return(key, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceCryptoKey, policy.ActionUpdate)
	}))
	s.Run("GetCryptoKeysByFeature", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		feature := database.CryptoKeyFeatureWorkspaceAppsAPIKey
		dbm.EXPECT().GetCryptoKeysByFeature(gomock.Any(), feature).Return([]database.CryptoKey{}, nil).AnyTimes()
		check.Args(feature).
			Asserts(rbac.ResourceCryptoKey, policy.ActionRead)
	}))
}

func (s *MethodTestSuite) TestSystemFunctions() {
	s.Run("UpdateUserLinkedID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		l := testutil.Fake(s.T(), faker, database.UserLink{UserID: u.ID})
		arg := database.UpdateUserLinkedIDParams{UserID: u.ID, LinkedID: l.LinkedID, LoginType: database.LoginTypeGithub}
		dbm.EXPECT().UpdateUserLinkedID(gomock.Any(), arg).Return(l, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns(l)
	}))
	s.Run("GetLatestWorkspaceAppStatusesByAppID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		appID := uuid.New()
		dbm.EXPECT().GetLatestWorkspaceAppStatusesByAppID(gomock.Any(), appID).Return([]database.WorkspaceAppStatus{}, nil).AnyTimes()
		check.Args(appID).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetLatestWorkspaceAppStatusesByWorkspaceIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{uuid.New()}
		dbm.EXPECT().GetLatestWorkspaceAppStatusesByWorkspaceIDs(gomock.Any(), ids).Return([]database.WorkspaceAppStatus{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAppStatusesByAppIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{uuid.New()}
		dbm.EXPECT().GetWorkspaceAppStatusesByAppIDs(gomock.Any(), ids).Return([]database.WorkspaceAppStatus{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetLatestWorkspaceBuildsByWorkspaceIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		wsID := uuid.New()
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{})
		dbm.EXPECT().GetLatestWorkspaceBuildsByWorkspaceIDs(gomock.Any(), []uuid.UUID{wsID}).Return([]database.WorkspaceBuild{b}, nil).AnyTimes()
		check.Args([]uuid.UUID{wsID}).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(slice.New(b))
	}))
	s.Run("UpsertDefaultProxy", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpsertDefaultProxyParams{}
		dbm.EXPECT().UpsertDefaultProxy(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))
	s.Run("GetUserLinkByLinkedID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		l := testutil.Fake(s.T(), faker, database.UserLink{})
		dbm.EXPECT().GetUserLinkByLinkedID(gomock.Any(), l.LinkedID).Return(l, nil).AnyTimes()
		check.Args(l.LinkedID).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(l)
	}))
	s.Run("GetUserLinkByUserIDLoginType", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		l := testutil.Fake(s.T(), faker, database.UserLink{})
		arg := database.GetUserLinkByUserIDLoginTypeParams{UserID: l.UserID, LoginType: l.LoginType}
		dbm.EXPECT().GetUserLinkByUserIDLoginType(gomock.Any(), arg).Return(l, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(l)
	}))
	s.Run("GetActiveUserCount", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetActiveUserCount(gomock.Any(), false).Return(int64(0), nil).AnyTimes()
		check.Args(false).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetAuthorizationUserRoles", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().GetAuthorizationUserRoles(gomock.Any(), u.ID).Return(database.GetAuthorizationUserRolesRow{}, nil).AnyTimes()
		check.Args(u.ID).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetDERPMeshKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetDERPMeshKey(gomock.Any()).Return("testing", nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("InsertDERPMeshKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().InsertDERPMeshKey(gomock.Any(), "value").Return(nil).AnyTimes()
		check.Args("value").Asserts(rbac.ResourceSystem, policy.ActionCreate).Returns()
	}))
	s.Run("InsertDeploymentID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().InsertDeploymentID(gomock.Any(), "value").Return(nil).AnyTimes()
		check.Args("value").Asserts(rbac.ResourceSystem, policy.ActionCreate).Returns()
	}))
	s.Run("InsertReplica", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertReplicaParams{ID: uuid.New()}
		dbm.EXPECT().InsertReplica(gomock.Any(), arg).Return(database.Replica{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpdateReplica", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		rep := testutil.Fake(s.T(), faker, database.Replica{})
		arg := database.UpdateReplicaParams{ID: rep.ID, DatabaseLatency: 100}
		dbm.EXPECT().UpdateReplica(gomock.Any(), arg).Return(rep, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("DeleteReplicasUpdatedBefore", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := dbtime.Now().Add(time.Hour)
		dbm.EXPECT().DeleteReplicasUpdatedBefore(gomock.Any(), t).Return(nil).AnyTimes()
		check.Args(t).Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("GetReplicasUpdatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := dbtime.Now().Add(-time.Hour)
		dbm.EXPECT().GetReplicasUpdatedAfter(gomock.Any(), t).Return([]database.Replica{}, nil).AnyTimes()
		check.Args(t).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetUserCount", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetUserCount(gomock.Any(), false).Return(int64(0), nil).AnyTimes()
		check.Args(false).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(int64(0))
	}))
	s.Run("GetTemplates", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetTemplates(gomock.Any()).Return([]database.Template{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("UpdateWorkspaceBuildCostByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{})
		arg := database.UpdateWorkspaceBuildCostByIDParams{ID: b.ID, DailyCost: 10}
		dbm.EXPECT().UpdateWorkspaceBuildCostByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("UpdateWorkspaceBuildProvisionerStateByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{})
		arg := database.UpdateWorkspaceBuildProvisionerStateByIDParams{ID: b.ID, ProvisionerState: []byte("testing")}
		dbm.EXPECT().UpdateWorkspaceBuildProvisionerStateByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("UpsertLastUpdateCheck", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertLastUpdateCheck(gomock.Any(), "value").Return(nil).AnyTimes()
		check.Args("value").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetLastUpdateCheck", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetLastUpdateCheck(gomock.Any()).Return("value", nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceBuildsCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ts := dbtime.Now()
		dbm.EXPECT().GetWorkspaceBuildsCreatedAfter(gomock.Any(), ts).Return([]database.WorkspaceBuild{}, nil).AnyTimes()
		check.Args(ts).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentsCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ts := dbtime.Now()
		dbm.EXPECT().GetWorkspaceAgentsCreatedAfter(gomock.Any(), ts).Return([]database.WorkspaceAgent{}, nil).AnyTimes()
		check.Args(ts).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAppsCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ts := dbtime.Now()
		dbm.EXPECT().GetWorkspaceAppsCreatedAfter(gomock.Any(), ts).Return([]database.WorkspaceApp{}, nil).AnyTimes()
		check.Args(ts).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceResourcesCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ts := dbtime.Now()
		dbm.EXPECT().GetWorkspaceResourcesCreatedAfter(gomock.Any(), ts).Return([]database.WorkspaceResource{}, nil).AnyTimes()
		check.Args(ts).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceResourceMetadataCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ts := dbtime.Now()
		dbm.EXPECT().GetWorkspaceResourceMetadataCreatedAfter(gomock.Any(), ts).Return([]database.WorkspaceResourceMetadatum{}, nil).AnyTimes()
		check.Args(ts).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("DeleteOldWorkspaceAgentStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().DeleteOldWorkspaceAgentStats(gomock.Any()).Return(nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("GetProvisionerJobsCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ts := dbtime.Now()
		dbm.EXPECT().GetProvisionerJobsCreatedAfter(gomock.Any(), ts).Return([]database.ProvisionerJob{}, nil).AnyTimes()
		check.Args(ts).Asserts(rbac.ResourceProvisionerJobs, policy.ActionRead)
	}))
	s.Run("GetTemplateVersionsByIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tv1 := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		tv2 := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		tv3 := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		ids := []uuid.UUID{tv1.ID, tv2.ID, tv3.ID}
		dbm.EXPECT().GetTemplateVersionsByIDs(gomock.Any(), ids).Return([]database.TemplateVersion{tv1, tv2, tv3}, nil).AnyTimes()
		check.Args(ids).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns(slice.New(tv1, tv2, tv3))
	}))
	s.Run("GetParameterSchemasByJobID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		jobID := v.JobID
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), jobID).Return(v, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().GetParameterSchemasByJobID(gomock.Any(), jobID).Return([]database.ParameterSchema{}, nil).AnyTimes()
		check.Args(jobID).
			Asserts(tpl, policy.ActionRead).
			Returns([]database.ParameterSchema{})
	}))
	s.Run("GetWorkspaceAppsByAgentIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.WorkspaceApp{})
		b := testutil.Fake(s.T(), faker, database.WorkspaceApp{})
		ids := []uuid.UUID{a.AgentID, b.AgentID}
		dbm.EXPECT().GetWorkspaceAppsByAgentIDs(gomock.Any(), ids).Return([]database.WorkspaceApp{a, b}, nil).AnyTimes()
		check.Args(ids).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.WorkspaceApp{a, b})
	}))
	s.Run("GetWorkspaceResourcesByJobIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{uuid.New(), uuid.New()}
		dbm.EXPECT().GetWorkspaceResourcesByJobIDs(gomock.Any(), ids).Return([]database.WorkspaceResource{}, nil).AnyTimes()
		check.Args(ids).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.WorkspaceResource{})
	}))
	s.Run("GetWorkspaceResourceMetadataByResourceIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{uuid.New(), uuid.New()}
		dbm.EXPECT().GetWorkspaceResourceMetadataByResourceIDs(gomock.Any(), ids).Return([]database.WorkspaceResourceMetadatum{}, nil).AnyTimes()
		check.Args(ids).
			Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentsByResourceIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		resID := uuid.New()
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		dbm.EXPECT().GetWorkspaceAgentsByResourceIDs(gomock.Any(), []uuid.UUID{resID}).Return([]database.WorkspaceAgent{agt}, nil).AnyTimes()
		check.Args([]uuid.UUID{resID}).
			Asserts(rbac.ResourceSystem, policy.ActionRead).
			Returns([]database.WorkspaceAgent{agt})
	}))
	s.Run("GetProvisionerJobsByIDs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		a := testutil.Fake(s.T(), faker, database.ProvisionerJob{OrganizationID: org.ID})
		b := testutil.Fake(s.T(), faker, database.ProvisionerJob{OrganizationID: org.ID})
		ids := []uuid.UUID{a.ID, b.ID}
		dbm.EXPECT().GetProvisionerJobsByIDs(gomock.Any(), ids).Return([]database.ProvisionerJob{a, b}, nil).AnyTimes()
		check.Args(ids).
			Asserts(rbac.ResourceProvisionerJobs.InOrg(org.ID), policy.ActionRead).
			Returns(slice.New(a, b))
	}))
	s.Run("DeleteWorkspaceSubAgentByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		agent := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().DeleteWorkspaceSubAgentByID(gomock.Any(), agent.ID).Return(nil).AnyTimes()
		check.Args(agent.ID).Asserts(ws, policy.ActionDeleteAgent)
	}))
	s.Run("GetWorkspaceAgentsByParentID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		parent := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		child := testutil.Fake(s.T(), faker, database.WorkspaceAgent{ParentID: uuid.NullUUID{Valid: true, UUID: parent.ID}})
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), parent.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceAgentsByParentID(gomock.Any(), parent.ID).Return([]database.WorkspaceAgent{child}, nil).AnyTimes()
		check.Args(parent.ID).Asserts(ws, policy.ActionRead)
	}))
	s.Run("InsertWorkspaceAgent", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		res := testutil.Fake(s.T(), faker, database.WorkspaceResource{})
		arg := database.InsertWorkspaceAgentParams{ID: uuid.New(), ResourceID: res.ID, Name: "dev", APIKeyScope: database.AgentKeyScopeEnumAll}
		dbm.EXPECT().GetWorkspaceByResourceID(gomock.Any(), res.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().InsertWorkspaceAgent(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.WorkspaceAgent{ResourceID: res.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionCreateAgent)
	}))
	s.Run("UpsertWorkspaceApp", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ws := testutil.Fake(s.T(), faker, database.Workspace{})
		agent := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.UpsertWorkspaceAppParams{ID: uuid.New(), AgentID: agent.ID, Health: database.WorkspaceAppHealthDisabled, SharingLevel: database.AppSharingLevelOwner, OpenIn: database.WorkspaceAppOpenInSlimWindow}
		dbm.EXPECT().GetWorkspaceByAgentID(gomock.Any(), agent.ID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().UpsertWorkspaceApp(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.WorkspaceApp{AgentID: agent.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(ws, policy.ActionUpdate)
	}))
	s.Run("InsertWorkspaceResourceMetadata", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceResourceMetadataParams{WorkspaceResourceID: uuid.New()}
		dbm.EXPECT().InsertWorkspaceResourceMetadata(gomock.Any(), arg).Return([]database.WorkspaceResourceMetadatum{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpdateWorkspaceAgentConnectionByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		agt := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		arg := database.UpdateWorkspaceAgentConnectionByIDParams{ID: agt.ID}
		dbm.EXPECT().UpdateWorkspaceAgentConnectionByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate).Returns()
	}))
	s.Run("AcquireProvisionerJob", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := database.AcquireProvisionerJobParams{StartedAt: sql.NullTime{Valid: true, Time: dbtime.Now()}, OrganizationID: uuid.New(), Types: []database.ProvisionerType{database.ProvisionerTypeEcho}, ProvisionerTags: json.RawMessage("{}")}
		dbm.EXPECT().AcquireProvisionerJob(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.ProvisionerJob{}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionUpdate)
	}))
	s.Run("UpdateProvisionerJobWithCompleteByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.UpdateProvisionerJobWithCompleteByIDParams{ID: j.ID}
		dbm.EXPECT().UpdateProvisionerJobWithCompleteByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionUpdate)
	}))
	s.Run("UpdateProvisionerJobWithCompleteWithStartedAtByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.UpdateProvisionerJobWithCompleteWithStartedAtByIDParams{ID: j.ID}
		dbm.EXPECT().UpdateProvisionerJobWithCompleteWithStartedAtByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionUpdate)
	}))
	s.Run("UpdateProvisionerJobByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.UpdateProvisionerJobByIDParams{ID: j.ID, UpdatedAt: dbtime.Now()}
		dbm.EXPECT().UpdateProvisionerJobByID(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionUpdate)
	}))
	s.Run("UpdateProvisionerJobLogsLength", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.UpdateProvisionerJobLogsLengthParams{ID: j.ID, LogsLength: 100}
		dbm.EXPECT().UpdateProvisionerJobLogsLength(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionUpdate)
	}))
	s.Run("UpdateProvisionerJobLogsOverflowed", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.UpdateProvisionerJobLogsOverflowedParams{ID: j.ID, LogsOverflowed: true}
		dbm.EXPECT().UpdateProvisionerJobLogsOverflowed(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionUpdate)
	}))
	s.Run("InsertProvisionerJob", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertProvisionerJobParams{
			ID:            uuid.New(),
			Provisioner:   database.ProvisionerTypeEcho,
			StorageMethod: database.ProvisionerStorageMethodFile,
			Type:          database.ProvisionerJobTypeWorkspaceBuild,
			Input:         json.RawMessage("{}"),
		}
		dbm.EXPECT().InsertProvisionerJob(gomock.Any(), arg).Return(testutil.Fake(s.T(), gofakeit.New(0), database.ProvisionerJob{}), nil).AnyTimes()
		check.Args(arg).Asserts( /* rbac.ResourceProvisionerJobs, policy.ActionCreate */ )
	}))
	s.Run("InsertProvisionerJobLogs", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.InsertProvisionerJobLogsParams{JobID: j.ID}
		dbm.EXPECT().InsertProvisionerJobLogs(gomock.Any(), arg).Return([]database.ProvisionerJobLog{}, nil).AnyTimes()
		check.Args(arg).Asserts( /* rbac.ResourceProvisionerJobs, policy.ActionUpdate */ )
	}))
	s.Run("InsertProvisionerJobTimings", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		arg := database.InsertProvisionerJobTimingsParams{JobID: j.ID}
		dbm.EXPECT().InsertProvisionerJobTimings(gomock.Any(), arg).Return([]database.ProvisionerJobTiming{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionUpdate)
	}))
	s.Run("UpsertProvisionerDaemon", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		pd := rbac.ResourceProvisionerDaemon.InOrg(org.ID)
		argOrg := database.UpsertProvisionerDaemonParams{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{},
			Tags:           database.StringMap(map[string]string{provisionersdk.TagScope: provisionersdk.ScopeOrganization}),
		}
		dbm.EXPECT().UpsertProvisionerDaemon(gomock.Any(), argOrg).Return(testutil.Fake(s.T(), faker, database.ProvisionerDaemon{OrganizationID: org.ID}), nil).AnyTimes()
		check.Args(argOrg).Asserts(pd, policy.ActionCreate)

		argUser := database.UpsertProvisionerDaemonParams{
			OrganizationID: org.ID,
			Provisioners:   []database.ProvisionerType{},
			Tags:           database.StringMap(map[string]string{provisionersdk.TagScope: provisionersdk.ScopeUser, provisionersdk.TagOwner: "11111111-1111-1111-1111-111111111111"}),
		}
		dbm.EXPECT().UpsertProvisionerDaemon(gomock.Any(), argUser).Return(testutil.Fake(s.T(), faker, database.ProvisionerDaemon{OrganizationID: org.ID}), nil).AnyTimes()
		check.Args(argUser).Asserts(pd.WithOwner("11111111-1111-1111-1111-111111111111"), policy.ActionCreate)
	}))
	s.Run("InsertTemplateVersionParameter", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		v := testutil.Fake(s.T(), faker, database.TemplateVersion{})
		arg := database.InsertTemplateVersionParameterParams{TemplateVersionID: v.ID, Options: json.RawMessage("{}")}
		dbm.EXPECT().InsertTemplateVersionParameter(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.TemplateVersionParameter{TemplateVersionID: v.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAppStatus", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAppStatusParams{ID: uuid.New(), State: "working"}
		dbm.EXPECT().InsertWorkspaceAppStatus(gomock.Any(), arg).Return(testutil.Fake(s.T(), gofakeit.New(0), database.WorkspaceAppStatus{ID: arg.ID, State: arg.State}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceResource", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceResourceParams{ID: uuid.New(), Transition: database.WorkspaceTransitionStart}
		dbm.EXPECT().InsertWorkspaceResource(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.WorkspaceResource{ID: arg.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("DeleteOldWorkspaceAgentLogs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().DeleteOldWorkspaceAgentLogs(gomock.Any(), t).Return(nil).AnyTimes()
		check.Args(t).Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("InsertWorkspaceAgentStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAgentStatsParams{}
		dbm.EXPECT().InsertWorkspaceAgentStats(gomock.Any(), arg).Return(xerrors.New("any error")).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate).Errors(errMatchAny)
	}))
	s.Run("InsertWorkspaceAppStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAppStatsParams{}
		dbm.EXPECT().InsertWorkspaceAppStats(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpsertWorkspaceAppAuditSession", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		agent := testutil.Fake(s.T(), faker, database.WorkspaceAgent{})
		app := testutil.Fake(s.T(), faker, database.WorkspaceApp{})
		arg := database.UpsertWorkspaceAppAuditSessionParams{AgentID: agent.ID, AppID: app.ID, UserID: u.ID, Ip: "127.0.0.1"}
		dbm.EXPECT().UpsertWorkspaceAppAuditSession(gomock.Any(), arg).Return(true, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("InsertWorkspaceAgentScriptTimings", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAgentScriptTimingsParams{ScriptID: uuid.New(), Stage: database.WorkspaceAgentScriptTimingStageStart, Status: database.WorkspaceAgentScriptTimingStatusOk}
		dbm.EXPECT().InsertWorkspaceAgentScriptTimings(gomock.Any(), arg).Return(testutil.Fake(s.T(), gofakeit.New(0), database.WorkspaceAgentScriptTiming{ScriptID: arg.ScriptID}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentScripts", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAgentScriptsParams{}
		dbm.EXPECT().InsertWorkspaceAgentScripts(gomock.Any(), arg).Return([]database.WorkspaceAgentScript{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentMetadata", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAgentMetadataParams{}
		dbm.EXPECT().InsertWorkspaceAgentMetadata(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertWorkspaceAgentLogs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAgentLogsParams{}
		dbm.EXPECT().InsertWorkspaceAgentLogs(gomock.Any(), arg).Return([]database.WorkspaceAgentLog{}, nil).AnyTimes()
		check.Args(arg).Asserts()
	}))
	s.Run("InsertWorkspaceAgentLogSources", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertWorkspaceAgentLogSourcesParams{}
		dbm.EXPECT().InsertWorkspaceAgentLogSources(gomock.Any(), arg).Return([]database.WorkspaceAgentLogSource{}, nil).AnyTimes()
		check.Args(arg).Asserts()
	}))
	s.Run("GetTemplateDAUs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetTemplateDAUsParams{}
		dbm.EXPECT().GetTemplateDAUs(gomock.Any(), arg).Return([]database.GetTemplateDAUsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetActiveWorkspaceBuildsByTemplateID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		dbm.EXPECT().GetActiveWorkspaceBuildsByTemplateID(gomock.Any(), id).Return([]database.WorkspaceBuild{}, nil).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns([]database.WorkspaceBuild{})
	}))
	s.Run("GetDeploymentDAUs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		tz := int32(0)
		dbm.EXPECT().GetDeploymentDAUs(gomock.Any(), tz).Return([]database.GetDeploymentDAUsRow{}, nil).AnyTimes()
		check.Args(tz).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetAppSecurityKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetAppSecurityKey(gomock.Any()).Return("", sql.ErrNoRows).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("UpsertAppSecurityKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertAppSecurityKey(gomock.Any(), "foo").Return(nil).AnyTimes()
		check.Args("foo").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetApplicationName", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetApplicationName(gomock.Any()).Return("foo", nil).AnyTimes()
		check.Args().Asserts()
	}))
	s.Run("UpsertApplicationName", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertApplicationName(gomock.Any(), "").Return(nil).AnyTimes()
		check.Args("").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetHealthSettings", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetHealthSettings(gomock.Any()).Return("{}", nil).AnyTimes()
		check.Args().Asserts()
	}))
	s.Run("UpsertHealthSettings", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertHealthSettings(gomock.Any(), "foo").Return(nil).AnyTimes()
		check.Args("foo").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetNotificationsSettings", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetNotificationsSettings(gomock.Any()).Return("{}", nil).AnyTimes()
		check.Args().Asserts()
	}))
	s.Run("UpsertNotificationsSettings", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertNotificationsSettings(gomock.Any(), "foo").Return(nil).AnyTimes()
		check.Args("foo").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetDeploymentWorkspaceAgentStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().GetDeploymentWorkspaceAgentStats(gomock.Any(), t).Return(database.GetDeploymentWorkspaceAgentStatsRow{}, nil).AnyTimes()
		check.Args(t).Asserts()
	}))
	s.Run("GetDeploymentWorkspaceAgentUsageStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().GetDeploymentWorkspaceAgentUsageStats(gomock.Any(), t).Return(database.GetDeploymentWorkspaceAgentUsageStatsRow{}, nil).AnyTimes()
		check.Args(t).Asserts()
	}))
	s.Run("GetDeploymentWorkspaceStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetDeploymentWorkspaceStats(gomock.Any()).Return(database.GetDeploymentWorkspaceStatsRow{}, nil).AnyTimes()
		check.Args().Asserts()
	}))
	s.Run("GetFileTemplates", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		dbm.EXPECT().GetFileTemplates(gomock.Any(), id).Return([]database.GetFileTemplatesRow{}, nil).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetProvisionerJobsToBeReaped", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetProvisionerJobsToBeReapedParams{}
		dbm.EXPECT().GetProvisionerJobsToBeReaped(gomock.Any(), arg).Return([]database.ProvisionerJob{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceProvisionerJobs, policy.ActionRead)
	}))
	s.Run("UpsertOAuthSigningKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertOAuthSigningKey(gomock.Any(), "foo").Return(nil).AnyTimes()
		check.Args("foo").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetOAuthSigningKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetOAuthSigningKey(gomock.Any()).Return("foo", nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("UpsertCoordinatorResumeTokenSigningKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertCoordinatorResumeTokenSigningKey(gomock.Any(), "foo").Return(nil).AnyTimes()
		check.Args("foo").Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetCoordinatorResumeTokenSigningKey", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetCoordinatorResumeTokenSigningKey(gomock.Any()).Return("foo", nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("InsertMissingGroups", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertMissingGroupsParams{}
		dbm.EXPECT().InsertMissingGroups(gomock.Any(), arg).Return([]database.Group{}, xerrors.New("any error")).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate).Errors(errMatchAny)
	}))
	s.Run("UpdateUserLoginType", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserLoginTypeParams{NewLoginType: database.LoginTypePassword, UserID: u.ID}
		dbm.EXPECT().UpdateUserLoginType(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.User{}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetWorkspaceAgentStatsAndLabels", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().GetWorkspaceAgentStatsAndLabels(gomock.Any(), t).Return([]database.GetWorkspaceAgentStatsAndLabelsRow{}, nil).AnyTimes()
		check.Args(t).Asserts()
	}))
	s.Run("GetWorkspaceAgentUsageStatsAndLabels", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().GetWorkspaceAgentUsageStatsAndLabels(gomock.Any(), t).Return([]database.GetWorkspaceAgentUsageStatsAndLabelsRow{}, nil).AnyTimes()
		check.Args(t).Asserts()
	}))
	s.Run("GetWorkspaceAgentStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().GetWorkspaceAgentStats(gomock.Any(), t).Return([]database.GetWorkspaceAgentStatsRow{}, nil).AnyTimes()
		check.Args(t).Asserts()
	}))
	s.Run("GetWorkspaceAgentUsageStats", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().GetWorkspaceAgentUsageStats(gomock.Any(), t).Return([]database.GetWorkspaceAgentUsageStatsRow{}, nil).AnyTimes()
		check.Args(t).Asserts()
	}))
	s.Run("GetWorkspaceProxyByHostname", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		p := testutil.Fake(s.T(), faker, database.WorkspaceProxy{WildcardHostname: "*.example.com"})
		arg := database.GetWorkspaceProxyByHostnameParams{Hostname: "foo.example.com", AllowWildcardHostname: true}
		dbm.EXPECT().GetWorkspaceProxyByHostname(gomock.Any(), arg).Return(p, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns(p)
	}))
	s.Run("GetTemplateAverageBuildTime", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := uuid.NullUUID{}
		dbm.EXPECT().GetTemplateAverageBuildTime(gomock.Any(), arg).Return(database.GetTemplateAverageBuildTimeRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspacesByTemplateID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.Nil
		dbm.EXPECT().GetWorkspacesByTemplateID(gomock.Any(), id).Return([]database.WorkspaceTable{}, nil).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspacesEligibleForTransition", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t := time.Time{}
		dbm.EXPECT().GetWorkspacesEligibleForTransition(gomock.Any(), t).Return([]database.GetWorkspacesEligibleForTransitionRow{}, nil).AnyTimes()
		check.Args(t).Asserts()
	}))
	s.Run("InsertTemplateVersionVariable", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertTemplateVersionVariableParams{}
		dbm.EXPECT().InsertTemplateVersionVariable(gomock.Any(), arg).Return(testutil.Fake(s.T(), gofakeit.New(0), database.TemplateVersionVariable{}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("InsertTemplateVersionWorkspaceTag", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertTemplateVersionWorkspaceTagParams{}
		dbm.EXPECT().InsertTemplateVersionWorkspaceTag(gomock.Any(), arg).Return(testutil.Fake(s.T(), gofakeit.New(0), database.TemplateVersionWorkspaceTag{}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpdateInactiveUsersToDormant", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpdateInactiveUsersToDormantParams{}
		dbm.EXPECT().UpdateInactiveUsersToDormant(gomock.Any(), arg).Return([]database.UpdateInactiveUsersToDormantRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate).Returns([]database.UpdateInactiveUsersToDormantRow{})
	}))
	s.Run("GetWorkspaceUniqueOwnerCountByTemplateIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{uuid.New()}
		dbm.EXPECT().GetWorkspaceUniqueOwnerCountByTemplateIDs(gomock.Any(), ids).Return([]database.GetWorkspaceUniqueOwnerCountByTemplateIDsRow{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentScriptsByAgentIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{uuid.New()}
		dbm.EXPECT().GetWorkspaceAgentScriptsByAgentIDs(gomock.Any(), ids).Return([]database.WorkspaceAgentScript{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentLogSourcesByAgentIDs", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{uuid.New()}
		dbm.EXPECT().GetWorkspaceAgentLogSourcesByAgentIDs(gomock.Any(), ids).Return([]database.WorkspaceAgentLogSource{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetProvisionerJobsByIDsWithQueuePosition", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetProvisionerJobsByIDsWithQueuePositionParams{}
		dbm.EXPECT().GetProvisionerJobsByIDsWithQueuePosition(gomock.Any(), arg).Return([]database.GetProvisionerJobsByIDsWithQueuePositionRow{}, nil).AnyTimes()
		check.Args(arg).Asserts()
	}))
	s.Run("GetReplicaByID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		dbm.EXPECT().GetReplicaByID(gomock.Any(), id).Return(database.Replica{}, sql.ErrNoRows).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceSystem, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetWorkspaceAgentAndLatestBuildByAuthToken", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		tok := uuid.New()
		dbm.EXPECT().GetWorkspaceAgentAndLatestBuildByAuthToken(gomock.Any(), tok).Return(database.GetWorkspaceAgentAndLatestBuildByAuthTokenRow{}, sql.ErrNoRows).AnyTimes()
		check.Args(tok).Asserts(rbac.ResourceSystem, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetUserLinksByUserID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		dbm.EXPECT().GetUserLinksByUserID(gomock.Any(), id).Return([]database.UserLink{}, nil).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("DeleteRuntimeConfig", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().DeleteRuntimeConfig(gomock.Any(), "test").Return(nil).AnyTimes()
		check.Args("test").Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("GetRuntimeConfig", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetRuntimeConfig(gomock.Any(), "test").Return("value", nil).AnyTimes()
		check.Args("test").Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("UpsertRuntimeConfig", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpsertRuntimeConfigParams{Key: "test", Value: "value"}
		dbm.EXPECT().UpsertRuntimeConfig(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("GetFailedWorkspaceBuildsByTemplateID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetFailedWorkspaceBuildsByTemplateIDParams{TemplateID: uuid.New(), Since: dbtime.Now()}
		dbm.EXPECT().GetFailedWorkspaceBuildsByTemplateID(gomock.Any(), arg).Return([]database.GetFailedWorkspaceBuildsByTemplateIDRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetNotificationReportGeneratorLogByTemplate", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetNotificationReportGeneratorLogByTemplate(gomock.Any(), notifications.TemplateWorkspaceBuildsFailedReport).Return(database.NotificationReportGeneratorLog{}, nil).AnyTimes()
		check.Args(notifications.TemplateWorkspaceBuildsFailedReport).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceBuildStatsByTemplates", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		at := dbtime.Now()
		dbm.EXPECT().GetWorkspaceBuildStatsByTemplates(gomock.Any(), at).Return([]database.GetWorkspaceBuildStatsByTemplatesRow{}, nil).AnyTimes()
		check.Args(at).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("UpsertNotificationReportGeneratorLog", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpsertNotificationReportGeneratorLogParams{NotificationTemplateID: uuid.New(), LastGeneratedAt: dbtime.Now()}
		dbm.EXPECT().UpsertNotificationReportGeneratorLog(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("GetProvisionerJobTimingsByJobID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{JobID: j.ID})
		ws := testutil.Fake(s.T(), faker, database.Workspace{ID: b.WorkspaceID})
		dbm.EXPECT().GetProvisionerJobByID(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), j.ID).Return(b, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), b.WorkspaceID).Return(ws, nil).AnyTimes()
		dbm.EXPECT().GetProvisionerJobTimingsByJobID(gomock.Any(), j.ID).Return([]database.ProvisionerJobTiming{}, nil).AnyTimes()
		check.Args(j.ID).Asserts(ws, policy.ActionRead)
	}))
	s.Run("GetWorkspaceAgentScriptTimingsByBuildID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		build := testutil.Fake(s.T(), faker, database.WorkspaceBuild{})
		dbm.EXPECT().GetWorkspaceAgentScriptTimingsByBuildID(gomock.Any(), build.ID).Return([]database.GetWorkspaceAgentScriptTimingsByBuildIDRow{}, nil).AnyTimes()
		check.Args(build.ID).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns([]database.GetWorkspaceAgentScriptTimingsByBuildIDRow{})
	}))
	s.Run("DisableForeignKeysAndTriggers", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().DisableForeignKeysAndTriggers(gomock.Any()).Return(nil).AnyTimes()
		check.Args().Asserts()
	}))
	s.Run("InsertWorkspaceModule", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		arg := database.InsertWorkspaceModuleParams{JobID: j.ID, Transition: database.WorkspaceTransitionStart}
		dbm.EXPECT().InsertWorkspaceModule(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.WorkspaceModule{JobID: j.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("GetWorkspaceModulesByJobID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		id := uuid.New()
		dbm.EXPECT().GetWorkspaceModulesByJobID(gomock.Any(), id).Return([]database.WorkspaceModule{}, nil).AnyTimes()
		check.Args(id).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetWorkspaceModulesCreatedAfter", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		at := dbtime.Now()
		dbm.EXPECT().GetWorkspaceModulesCreatedAfter(gomock.Any(), at).Return([]database.WorkspaceModule{}, nil).AnyTimes()
		check.Args(at).Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("GetTelemetryItem", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetTelemetryItem(gomock.Any(), "test").Return(database.TelemetryItem{}, sql.ErrNoRows).AnyTimes()
		check.Args("test").Asserts(rbac.ResourceSystem, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("GetTelemetryItems", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetTelemetryItems(gomock.Any()).Return([]database.TelemetryItem{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceSystem, policy.ActionRead)
	}))
	s.Run("InsertTelemetryItemIfNotExists", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.InsertTelemetryItemIfNotExistsParams{Key: "test", Value: "value"}
		dbm.EXPECT().InsertTelemetryItemIfNotExists(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))
	s.Run("UpsertTelemetryItem", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpsertTelemetryItemParams{Key: "test", Value: "value"}
		dbm.EXPECT().UpsertTelemetryItem(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceSystem, policy.ActionUpdate)
	}))
	s.Run("GetOAuth2GithubDefaultEligible", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetOAuth2GithubDefaultEligible(gomock.Any()).Return(false, sql.ErrNoRows).AnyTimes()
		check.Args().Asserts(rbac.ResourceDeploymentConfig, policy.ActionRead).Errors(sql.ErrNoRows)
	}))
	s.Run("UpsertOAuth2GithubDefaultEligible", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertOAuth2GithubDefaultEligible(gomock.Any(), true).Return(nil).AnyTimes()
		check.Args(true).Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("GetWebpushVAPIDKeys", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetWebpushVAPIDKeys(gomock.Any()).Return(database.GetWebpushVAPIDKeysRow{VapidPublicKey: "test", VapidPrivateKey: "test"}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceDeploymentConfig, policy.ActionRead).Returns(database.GetWebpushVAPIDKeysRow{VapidPublicKey: "test", VapidPrivateKey: "test"})
	}))
	s.Run("UpsertWebpushVAPIDKeys", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpsertWebpushVAPIDKeysParams{VapidPublicKey: "test", VapidPrivateKey: "test"}
		dbm.EXPECT().UpsertWebpushVAPIDKeys(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("Build/GetProvisionerJobByIDForUpdate", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeWorkspaceBuild})
		dbm.EXPECT().GetProvisionerJobByIDForUpdate(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		// Minimal assertion check argument
		b := testutil.Fake(s.T(), faker, database.WorkspaceBuild{JobID: j.ID})
		w := testutil.Fake(s.T(), faker, database.Workspace{ID: b.WorkspaceID})
		dbm.EXPECT().GetWorkspaceBuildByJobID(gomock.Any(), j.ID).Return(b, nil).AnyTimes()
		dbm.EXPECT().GetWorkspaceByID(gomock.Any(), b.WorkspaceID).Return(w, nil).AnyTimes()
		check.Args(j.ID).Asserts(w, policy.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersion/GetProvisionerJobByIDForUpdate", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{Type: database.ProvisionerJobTypeTemplateVersionImport})
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		dbm.EXPECT().GetProvisionerJobByIDForUpdate(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByJobID(gomock.Any(), j.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		check.Args(j.ID).Asserts(tv.RBACObject(tpl), policy.ActionRead).Returns(j)
	}))
	s.Run("TemplateVersionDryRun/GetProvisionerJobByIDForUpdate", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}})
		j := testutil.Fake(s.T(), faker, database.ProvisionerJob{})
		j.Type = database.ProvisionerJobTypeTemplateVersionDryRun
		j.Input = must(json.Marshal(struct {
			TemplateVersionID uuid.UUID `json:"template_version_id"`
		}{TemplateVersionID: tv.ID}))
		dbm.EXPECT().GetProvisionerJobByIDForUpdate(gomock.Any(), j.ID).Return(j, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		check.Args(j.ID).Asserts(tv.RBACObject(tpl), policy.ActionRead).Returns(j)
	}))
}

func (s *MethodTestSuite) TestNotifications() {
	// System functions
	s.Run("AcquireNotificationMessages", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().AcquireNotificationMessages(gomock.Any(), database.AcquireNotificationMessagesParams{}).Return([]database.AcquireNotificationMessagesRow{}, nil).AnyTimes()
		check.Args(database.AcquireNotificationMessagesParams{}).Asserts(rbac.ResourceNotificationMessage, policy.ActionUpdate)
	}))
	s.Run("BulkMarkNotificationMessagesFailed", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().BulkMarkNotificationMessagesFailed(gomock.Any(), database.BulkMarkNotificationMessagesFailedParams{}).Return(int64(0), nil).AnyTimes()
		check.Args(database.BulkMarkNotificationMessagesFailedParams{}).Asserts(rbac.ResourceNotificationMessage, policy.ActionUpdate)
	}))
	s.Run("BulkMarkNotificationMessagesSent", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().BulkMarkNotificationMessagesSent(gomock.Any(), database.BulkMarkNotificationMessagesSentParams{}).Return(int64(0), nil).AnyTimes()
		check.Args(database.BulkMarkNotificationMessagesSentParams{}).Asserts(rbac.ResourceNotificationMessage, policy.ActionUpdate)
	}))
	s.Run("DeleteOldNotificationMessages", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().DeleteOldNotificationMessages(gomock.Any()).Return(nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceNotificationMessage, policy.ActionDelete)
	}))
	s.Run("EnqueueNotificationMessage", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.EnqueueNotificationMessageParams{Method: database.NotificationMethodWebhook, Payload: []byte("{}")}
		dbm.EXPECT().EnqueueNotificationMessage(gomock.Any(), arg).Return(nil).AnyTimes()
		// TODO: update this test once we have a specific role for notifications
		check.Args(arg).Asserts(rbac.ResourceNotificationMessage, policy.ActionCreate)
	}))
	s.Run("FetchNewMessageMetadata", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().FetchNewMessageMetadata(gomock.Any(), database.FetchNewMessageMetadataParams{UserID: u.ID}).Return(database.FetchNewMessageMetadataRow{}, nil).AnyTimes()
		check.Args(database.FetchNewMessageMetadataParams{UserID: u.ID}).
			Asserts(rbac.ResourceNotificationMessage, policy.ActionRead)
	}))
	s.Run("GetNotificationMessagesByStatus", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.GetNotificationMessagesByStatusParams{Status: database.NotificationMessageStatusLeased, Limit: 10}
		dbm.EXPECT().GetNotificationMessagesByStatus(gomock.Any(), arg).Return([]database.NotificationMessage{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceNotificationMessage, policy.ActionRead)
	}))

	// webpush subscriptions
	s.Run("GetWebpushSubscriptionsByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().GetWebpushSubscriptionsByUserID(gomock.Any(), user.ID).Return([]database.WebpushSubscription{}, nil).AnyTimes()
		check.Args(user.ID).Asserts(rbac.ResourceWebpushSubscription.WithOwner(user.ID.String()), policy.ActionRead)
	}))
	s.Run("InsertWebpushSubscription", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		arg := database.InsertWebpushSubscriptionParams{UserID: user.ID}
		dbm.EXPECT().InsertWebpushSubscription(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.WebpushSubscription{UserID: user.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceWebpushSubscription.WithOwner(user.ID.String()), policy.ActionCreate)
	}))
	s.Run("DeleteWebpushSubscriptions", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		push := testutil.Fake(s.T(), faker, database.WebpushSubscription{UserID: user.ID})
		dbm.EXPECT().DeleteWebpushSubscriptions(gomock.Any(), []uuid.UUID{push.ID}).Return(nil).AnyTimes()
		check.Args([]uuid.UUID{push.ID}).Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))
	s.Run("DeleteWebpushSubscriptionByUserIDAndEndpoint", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		push := testutil.Fake(s.T(), faker, database.WebpushSubscription{UserID: user.ID})
		arg := database.DeleteWebpushSubscriptionByUserIDAndEndpointParams{UserID: user.ID, Endpoint: push.Endpoint}
		dbm.EXPECT().DeleteWebpushSubscriptionByUserIDAndEndpoint(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceWebpushSubscription.WithOwner(user.ID.String()), policy.ActionDelete)
	}))
	s.Run("DeleteAllWebpushSubscriptions", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().DeleteAllWebpushSubscriptions(gomock.Any()).Return(nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceWebpushSubscription, policy.ActionDelete)
	}))

	// Notification templates
	s.Run("GetNotificationTemplateByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.NotificationTemplate{})
		dbm.EXPECT().GetNotificationTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		check.Args(tpl.ID).Asserts(rbac.ResourceNotificationTemplate, policy.ActionRead)
	}))
	s.Run("GetNotificationTemplatesByKind", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetNotificationTemplatesByKind(gomock.Any(), database.NotificationTemplateKindSystem).Return([]database.NotificationTemplate{}, nil).AnyTimes()
		check.Args(database.NotificationTemplateKindSystem).Asserts()
		// TODO(dannyk): add support for other database.NotificationTemplateKind types once implemented.
	}))
	s.Run("UpdateNotificationTemplateMethodByID", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		arg := database.UpdateNotificationTemplateMethodByIDParams{Method: database.NullNotificationMethod{NotificationMethod: database.NotificationMethodWebhook, Valid: true}, ID: notifications.TemplateWorkspaceDormant}
		dbm.EXPECT().UpdateNotificationTemplateMethodByID(gomock.Any(), arg).Return(database.NotificationTemplate{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceNotificationTemplate, policy.ActionUpdate)
	}))

	// Notification preferences
	s.Run("GetUserNotificationPreferences", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().GetUserNotificationPreferences(gomock.Any(), user.ID).Return([]database.NotificationPreference{}, nil).AnyTimes()
		check.Args(user.ID).Asserts(rbac.ResourceNotificationPreference.WithOwner(user.ID.String()), policy.ActionRead)
	}))
	s.Run("UpdateUserNotificationPreferences", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		arg := database.UpdateUserNotificationPreferencesParams{UserID: user.ID, NotificationTemplateIds: []uuid.UUID{notifications.TemplateWorkspaceAutoUpdated, notifications.TemplateWorkspaceDeleted}, Disableds: []bool{true, false}}
		dbm.EXPECT().UpdateUserNotificationPreferences(gomock.Any(), arg).Return(int64(2), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceNotificationPreference.WithOwner(user.ID.String()), policy.ActionUpdate)
	}))

	s.Run("GetInboxNotificationsByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		notif := testutil.Fake(s.T(), faker, database.InboxNotification{UserID: u.ID, TemplateID: notifications.TemplateWorkspaceAutoUpdated})
		arg := database.GetInboxNotificationsByUserIDParams{UserID: u.ID, ReadStatus: database.InboxNotificationReadStatusAll}
		dbm.EXPECT().GetInboxNotificationsByUserID(gomock.Any(), arg).Return([]database.InboxNotification{notif}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceInboxNotification.WithID(notif.ID).WithOwner(u.ID.String()), policy.ActionRead).Returns([]database.InboxNotification{notif})
	}))

	s.Run("GetFilteredInboxNotificationsByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		notif := testutil.Fake(s.T(), faker, database.InboxNotification{UserID: u.ID, TemplateID: notifications.TemplateWorkspaceAutoUpdated, Targets: []uuid.UUID{u.ID, notifications.TemplateWorkspaceAutoUpdated}})
		arg := database.GetFilteredInboxNotificationsByUserIDParams{UserID: u.ID, Templates: []uuid.UUID{notifications.TemplateWorkspaceAutoUpdated}, Targets: []uuid.UUID{u.ID}, ReadStatus: database.InboxNotificationReadStatusAll}
		dbm.EXPECT().GetFilteredInboxNotificationsByUserID(gomock.Any(), arg).Return([]database.InboxNotification{notif}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceInboxNotification.WithID(notif.ID).WithOwner(u.ID.String()), policy.ActionRead).Returns([]database.InboxNotification{notif})
	}))

	s.Run("GetInboxNotificationByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		notif := testutil.Fake(s.T(), faker, database.InboxNotification{UserID: u.ID, TemplateID: notifications.TemplateWorkspaceAutoUpdated, Targets: []uuid.UUID{u.ID, notifications.TemplateWorkspaceAutoUpdated}})
		dbm.EXPECT().GetInboxNotificationByID(gomock.Any(), notif.ID).Return(notif, nil).AnyTimes()
		check.Args(notif.ID).Asserts(rbac.ResourceInboxNotification.WithID(notif.ID).WithOwner(u.ID.String()), policy.ActionRead).Returns(notif)
	}))

	s.Run("CountUnreadInboxNotificationsByUserID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		dbm.EXPECT().CountUnreadInboxNotificationsByUserID(gomock.Any(), u.ID).Return(int64(1), nil).AnyTimes()
		check.Args(u.ID).Asserts(rbac.ResourceInboxNotification.WithOwner(u.ID.String()), policy.ActionRead).Returns(int64(1))
	}))

	s.Run("InsertInboxNotification", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		notifID := uuid.New()
		arg := database.InsertInboxNotificationParams{ID: notifID, UserID: u.ID, TemplateID: notifications.TemplateWorkspaceAutoUpdated, Targets: []uuid.UUID{u.ID, notifications.TemplateWorkspaceAutoUpdated}, Title: "test title", Content: "test content notification", Icon: "https://coder.com/favicon.ico", Actions: json.RawMessage("{}")}
		dbm.EXPECT().InsertInboxNotification(gomock.Any(), arg).Return(testutil.Fake(s.T(), faker, database.InboxNotification{ID: notifID, UserID: u.ID}), nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceInboxNotification.WithOwner(u.ID.String()), policy.ActionCreate)
	}))

	s.Run("UpdateInboxNotificationReadStatus", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		notif := testutil.Fake(s.T(), faker, database.InboxNotification{UserID: u.ID})
		arg := database.UpdateInboxNotificationReadStatusParams{ID: notif.ID}

		dbm.EXPECT().GetInboxNotificationByID(gomock.Any(), notif.ID).Return(notif, nil).AnyTimes()
		dbm.EXPECT().UpdateInboxNotificationReadStatus(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(notif, policy.ActionUpdate)
	}))

	s.Run("MarkAllInboxNotificationsAsRead", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		u := testutil.Fake(s.T(), faker, database.User{})
		arg := database.MarkAllInboxNotificationsAsReadParams{UserID: u.ID, ReadAt: sql.NullTime{Time: dbtestutil.NowInDefaultTimezone(), Valid: true}}
		dbm.EXPECT().MarkAllInboxNotificationsAsRead(gomock.Any(), arg).Return(nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceInboxNotification.WithOwner(u.ID.String()), policy.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestPrebuilds() {
	s.Run("GetPresetByWorkspaceBuildID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		wbID := uuid.New()
		dbm.EXPECT().GetPresetByWorkspaceBuildID(gomock.Any(), wbID).Return(testutil.Fake(s.T(), faker, database.TemplateVersionPreset{}), nil).AnyTimes()
		check.Args(wbID).Asserts(rbac.ResourceTemplate, policy.ActionRead)
	}))
	s.Run("GetPresetParametersByTemplateVersionID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, OrganizationID: tpl.OrganizationID, CreatedBy: tpl.CreatedBy})
		resp := []database.TemplateVersionPresetParameter{testutil.Fake(s.T(), faker, database.TemplateVersionPresetParameter{})}

		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().GetPresetParametersByTemplateVersionID(gomock.Any(), tv.ID).Return(resp, nil).AnyTimes()
		check.Args(tv.ID).Asserts(tpl.RBACObject(), policy.ActionRead).Returns(resp)
	}))
	s.Run("GetPresetParametersByPresetID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		prow := database.GetPresetByIDRow{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, OrganizationID: tpl.OrganizationID}
		resp := []database.TemplateVersionPresetParameter{testutil.Fake(s.T(), faker, database.TemplateVersionPresetParameter{})}

		dbm.EXPECT().GetPresetByID(gomock.Any(), prow.ID).Return(prow, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().GetPresetParametersByPresetID(gomock.Any(), prow.ID).Return(resp, nil).AnyTimes()
		check.Args(prow.ID).Asserts(tpl.RBACObject(), policy.ActionRead).Returns(resp)
	}))
	s.Run("GetActivePresetPrebuildSchedules", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetActivePresetPrebuildSchedules(gomock.Any()).Return([]database.TemplateVersionPresetPrebuildSchedule{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceTemplate.All(), policy.ActionRead).Returns([]database.TemplateVersionPresetPrebuildSchedule{})
	}))
	s.Run("GetPresetsByTemplateVersionID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		tpl := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, OrganizationID: tpl.OrganizationID, CreatedBy: tpl.CreatedBy})
		presets := []database.TemplateVersionPreset{testutil.Fake(s.T(), faker, database.TemplateVersionPreset{TemplateVersionID: tv.ID})}

		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().GetPresetsByTemplateVersionID(gomock.Any(), tv.ID).Return(presets, nil).AnyTimes()
		check.Args(tv.ID).Asserts(tpl.RBACObject(), policy.ActionRead).Returns(presets)
	}))
	s.Run("ClaimPrebuiltWorkspace", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		tpl := testutil.Fake(s.T(), faker, database.Template{CreatedBy: user.ID})
		arg := database.ClaimPrebuiltWorkspaceParams{NewUserID: user.ID, NewName: "", PresetID: uuid.New()}
		prow := database.GetPresetByIDRow{ID: arg.PresetID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, OrganizationID: tpl.OrganizationID}

		dbm.EXPECT().GetPresetByID(gomock.Any(), arg.PresetID).Return(prow, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		dbm.EXPECT().ClaimPrebuiltWorkspace(gomock.Any(), arg).Return(database.ClaimPrebuiltWorkspaceRow{}, sql.ErrNoRows).AnyTimes()
		check.Args(arg).Asserts(
			rbac.ResourceWorkspace.WithOwner(user.ID.String()).InOrg(tpl.OrganizationID), policy.ActionCreate,
			tpl, policy.ActionRead,
			tpl, policy.ActionUse,
		).Errors(sql.ErrNoRows)
	}))
	s.Run("FindMatchingPresetID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t1 := testutil.Fake(s.T(), faker, database.Template{})
		tv := testutil.Fake(s.T(), faker, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: t1.ID, Valid: true}})
		dbm.EXPECT().FindMatchingPresetID(gomock.Any(), database.FindMatchingPresetIDParams{
			TemplateVersionID: tv.ID,
			ParameterNames:    []string{"test"},
			ParameterValues:   []string{"test"},
		}).Return(uuid.Nil, nil).AnyTimes()
		dbm.EXPECT().GetTemplateVersionByID(gomock.Any(), tv.ID).Return(tv, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), t1.ID).Return(t1, nil).AnyTimes()
		check.Args(database.FindMatchingPresetIDParams{
			TemplateVersionID: tv.ID,
			ParameterNames:    []string{"test"},
			ParameterValues:   []string{"test"},
		}).Asserts(tv.RBACObject(t1), policy.ActionRead).Returns(uuid.Nil)
	}))
	s.Run("GetPrebuildMetrics", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetPrebuildMetrics(gomock.Any()).Return([]database.GetPrebuildMetricsRow{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceWorkspace.All(), policy.ActionRead)
	}))
	s.Run("GetOrganizationsWithPrebuildStatus", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := database.GetOrganizationsWithPrebuildStatusParams{
			UserID:    uuid.New(),
			GroupName: "test",
		}
		dbm.EXPECT().GetOrganizationsWithPrebuildStatus(gomock.Any(), arg).Return([]database.GetOrganizationsWithPrebuildStatusRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceOrganization.All(), policy.ActionRead)
	}))
	s.Run("GetPrebuildsSettings", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetPrebuildsSettings(gomock.Any()).Return("{}", nil).AnyTimes()
		check.Args().Asserts()
	}))
	s.Run("UpsertPrebuildsSettings", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().UpsertPrebuildsSettings(gomock.Any(), "foo").Return(nil).AnyTimes()
		check.Args("foo").Asserts(rbac.ResourceDeploymentConfig, policy.ActionUpdate)
	}))
	s.Run("CountInProgressPrebuilds", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().CountInProgressPrebuilds(gomock.Any()).Return([]database.CountInProgressPrebuildsRow{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceWorkspace.All(), policy.ActionRead)
	}))
	s.Run("CountPendingNonActivePrebuilds", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().CountPendingNonActivePrebuilds(gomock.Any()).Return([]database.CountPendingNonActivePrebuildsRow{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceWorkspace.All(), policy.ActionRead)
	}))
	s.Run("GetPresetsAtFailureLimit", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetPresetsAtFailureLimit(gomock.Any(), int64(0)).Return([]database.GetPresetsAtFailureLimitRow{}, nil).AnyTimes()
		check.Args(int64(0)).Asserts(rbac.ResourceTemplate.All(), policy.ActionViewInsights)
	}))
	s.Run("GetPresetsBackoff", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		t0 := time.Time{}
		dbm.EXPECT().GetPresetsBackoff(gomock.Any(), t0).Return([]database.GetPresetsBackoffRow{}, nil).AnyTimes()
		check.Args(t0).Asserts(rbac.ResourceTemplate.All(), policy.ActionViewInsights)
	}))
	s.Run("GetRunningPrebuiltWorkspaces", s.Mocked(func(dbm *dbmock.MockStore, _ *gofakeit.Faker, check *expects) {
		dbm.EXPECT().GetRunningPrebuiltWorkspaces(gomock.Any()).Return([]database.GetRunningPrebuiltWorkspacesRow{}, nil).AnyTimes()
		check.Args().Asserts(rbac.ResourceWorkspace.All(), policy.ActionRead)
	}))
	s.Run("GetTemplatePresetsWithPrebuilds", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		arg := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		dbm.EXPECT().GetTemplatePresetsWithPrebuilds(gomock.Any(), arg).Return([]database.GetTemplatePresetsWithPrebuildsRow{}, nil).AnyTimes()
		check.Args(arg).Asserts(rbac.ResourceTemplate.All(), policy.ActionRead)
	}))
	s.Run("GetPresetByID", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		tpl := testutil.Fake(s.T(), faker, database.Template{OrganizationID: org.ID})
		presetID := uuid.New()
		prow := database.GetPresetByIDRow{ID: presetID, TemplateVersionID: uuid.New(), Name: "test", TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, InvalidateAfterSecs: sql.NullInt32{}, OrganizationID: org.ID, PrebuildStatus: database.PrebuildStatusHealthy}

		dbm.EXPECT().GetPresetByID(gomock.Any(), presetID).Return(prow, nil).AnyTimes()
		dbm.EXPECT().GetTemplateByID(gomock.Any(), tpl.ID).Return(tpl, nil).AnyTimes()
		check.Args(presetID).Asserts(tpl, policy.ActionRead).Returns(prow)
	}))
	s.Run("UpdatePresetPrebuildStatus", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		org := testutil.Fake(s.T(), faker, database.Organization{})
		tpl := testutil.Fake(s.T(), faker, database.Template{OrganizationID: org.ID})
		presetID := uuid.New()
		prow := database.GetPresetByIDRow{ID: presetID, TemplateID: uuid.NullUUID{UUID: tpl.ID, Valid: true}, OrganizationID: org.ID}
		req := database.UpdatePresetPrebuildStatusParams{PresetID: presetID, Status: database.PrebuildStatusHealthy}

		dbm.EXPECT().GetPresetByID(gomock.Any(), presetID).Return(prow, nil).AnyTimes()
		dbm.EXPECT().UpdatePresetPrebuildStatus(gomock.Any(), req).Return(nil).AnyTimes()
		// TODO: This does not check the acl list on the template. Should it?
		check.Args(req).Asserts(rbac.ResourceTemplate.WithID(tpl.ID).InOrg(org.ID), policy.ActionUpdate)
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
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		user := dbgen.User(s.T(), db, database.User{})
		key, _ := dbgen.APIKey(s.T(), db, database.APIKey{
			UserID: user.ID,
		})
		// Use a fixed timestamp for consistent test results across all database types
		fixedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{
			CreatedAt: fixedTime,
			UpdatedAt: fixedTime,
		})
		_ = dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{
			CreatedAt: fixedTime,
			UpdatedAt: fixedTime,
		})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		for i := 0; i < 5; i++ {
			_ = dbgen.OAuth2ProviderAppToken(s.T(), db, database.OAuth2ProviderAppToken{
				AppSecretID: secret.ID,
				APIKeyID:    key.ID,
				UserID:      user.ID,
				HashPrefix:  []byte(fmt.Sprintf("%d", i)),
			})
		}
		expectedApp := app
		expectedApp.CreatedAt = fixedTime
		expectedApp.UpdatedAt = fixedTime
		check.Args(user.ID).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionRead).Returns([]database.GetOAuth2ProviderAppsByUserIDRow{
			{
				OAuth2ProviderApp: expectedApp,
				TokenCount:        5,
			},
		})
	}))
	s.Run("InsertOAuth2ProviderApp", s.Subtest(func(db database.Store, check *expects) {
		check.Args(database.InsertOAuth2ProviderAppParams{}).Asserts(rbac.ResourceOauth2App, policy.ActionCreate)
	}))
	s.Run("UpdateOAuth2ProviderAppByID", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		app.Name = "my-new-name"
		app.UpdatedAt = dbtestutil.NowInDefaultTimezone()
		check.Args(database.UpdateOAuth2ProviderAppByIDParams{
			ID:                      app.ID,
			Name:                    app.Name,
			Icon:                    app.Icon,
			CallbackURL:             app.CallbackURL,
			RedirectUris:            app.RedirectUris,
			ClientType:              app.ClientType,
			DynamicallyRegistered:   app.DynamicallyRegistered,
			ClientSecretExpiresAt:   app.ClientSecretExpiresAt,
			GrantTypes:              app.GrantTypes,
			ResponseTypes:           app.ResponseTypes,
			TokenEndpointAuthMethod: app.TokenEndpointAuthMethod,
			Scope:                   app.Scope,
			Contacts:                app.Contacts,
			ClientUri:               app.ClientUri,
			LogoUri:                 app.LogoUri,
			TosUri:                  app.TosUri,
			PolicyUri:               app.PolicyUri,
			JwksUri:                 app.JwksUri,
			Jwks:                    app.Jwks,
			SoftwareID:              app.SoftwareID,
			SoftwareVersion:         app.SoftwareVersion,
			UpdatedAt:               app.UpdatedAt,
		}).Asserts(rbac.ResourceOauth2App, policy.ActionUpdate).Returns(app)
	}))
	s.Run("DeleteOAuth2ProviderAppByID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(app.ID).Asserts(rbac.ResourceOauth2App, policy.ActionDelete)
	}))
	s.Run("GetOAuth2ProviderAppByClientID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(app.ID).Asserts(rbac.ResourceOauth2App, policy.ActionRead).Returns(app)
	}))
	s.Run("DeleteOAuth2ProviderAppByClientID", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		check.Args(app.ID).Asserts(rbac.ResourceOauth2App, policy.ActionDelete)
	}))
	s.Run("UpdateOAuth2ProviderAppByClientID", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		app.Name = "updated-name"
		app.UpdatedAt = dbtestutil.NowInDefaultTimezone()
		check.Args(database.UpdateOAuth2ProviderAppByClientIDParams{
			ID:                      app.ID,
			Name:                    app.Name,
			Icon:                    app.Icon,
			CallbackURL:             app.CallbackURL,
			RedirectUris:            app.RedirectUris,
			ClientType:              app.ClientType,
			ClientSecretExpiresAt:   app.ClientSecretExpiresAt,
			GrantTypes:              app.GrantTypes,
			ResponseTypes:           app.ResponseTypes,
			TokenEndpointAuthMethod: app.TokenEndpointAuthMethod,
			Scope:                   app.Scope,
			Contacts:                app.Contacts,
			ClientUri:               app.ClientUri,
			LogoUri:                 app.LogoUri,
			TosUri:                  app.TosUri,
			PolicyUri:               app.PolicyUri,
			JwksUri:                 app.JwksUri,
			Jwks:                    app.Jwks,
			SoftwareID:              app.SoftwareID,
			SoftwareVersion:         app.SoftwareVersion,
			UpdatedAt:               app.UpdatedAt,
		}).Asserts(rbac.ResourceOauth2App, policy.ActionUpdate).Returns(app)
	}))
	s.Run("GetOAuth2ProviderAppByRegistrationToken", s.Subtest(func(db database.Store, check *expects) {
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{
			RegistrationAccessToken: []byte("test-token"),
		})
		check.Args([]byte("test-token")).Asserts(rbac.ResourceOauth2App, policy.ActionRead).Returns(app)
	}))
}

func (s *MethodTestSuite) TestOAuth2ProviderAppSecrets() {
	s.Run("GetOAuth2ProviderAppSecretsByAppID", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		app1 := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		app2 := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secrets := []database.OAuth2ProviderAppSecret{
			dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
				AppID:        app1.ID,
				CreatedAt:    time.Now().Add(-time.Hour), // For ordering.
				SecretPrefix: []byte("1"),
			}),
			dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
				AppID:        app1.ID,
				SecretPrefix: []byte("2"),
			}),
		}
		_ = dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID:        app2.ID,
			SecretPrefix: []byte("3"),
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
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		secret := dbgen.OAuth2ProviderAppSecret(s.T(), db, database.OAuth2ProviderAppSecret{
			AppID: app.ID,
		})
		secret.LastUsedAt = sql.NullTime{Time: dbtestutil.NowInDefaultTimezone(), Valid: true}
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
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
		user := dbgen.User(s.T(), db, database.User{})
		app := dbgen.OAuth2ProviderApp(s.T(), db, database.OAuth2ProviderApp{})
		for i := 0; i < 5; i++ {
			_ = dbgen.OAuth2ProviderAppCode(s.T(), db, database.OAuth2ProviderAppCode{
				AppID:        app.ID,
				UserID:       user.ID,
				SecretPrefix: []byte(fmt.Sprintf("%d", i)),
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
			UserID:      user.ID,
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
			UserID:      user.ID,
		})
		check.Args(token.HashPrefix).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()).WithID(token.ID), policy.ActionRead).Returns(token)
	}))
	s.Run("GetOAuth2ProviderAppTokenByAPIKeyID", s.Subtest(func(db database.Store, check *expects) {
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
			UserID:      user.ID,
		})
		check.Args(token.APIKeyID).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()).WithID(token.ID), policy.ActionRead).Returns(token)
	}))
	s.Run("DeleteOAuth2ProviderAppTokensByAppAndUserID", s.Subtest(func(db database.Store, check *expects) {
		dbtestutil.DisableForeignKeysAndTriggers(s.T(), db)
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
				UserID:      user.ID,
				HashPrefix:  []byte(fmt.Sprintf("%d", i)),
			})
		}
		check.Args(database.DeleteOAuth2ProviderAppTokensByAppAndUserIDParams{
			AppID:  app.ID,
			UserID: user.ID,
		}).Asserts(rbac.ResourceOauth2AppCodeToken.WithOwner(user.ID.String()), policy.ActionDelete)
	}))
}

func (s *MethodTestSuite) TestResourcesMonitor() {
	createAgent := func(t *testing.T, db database.Store) (database.WorkspaceAgent, database.WorkspaceTable) {
		t.Helper()

		u := dbgen.User(t, db, database.User{})
		o := dbgen.Organization(t, db, database.Organization{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		w := dbgen.Workspace(t, db, database.WorkspaceTable{
			TemplateID:     tpl.ID,
			OrganizationID: o.ID,
			OwnerID:        u.ID,
		})
		j := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		b := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			JobID:             j.ID,
			WorkspaceID:       w.ID,
			TemplateVersionID: tv.ID,
		})
		res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: b.JobID})
		agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})

		return agt, w
	}

	s.Run("InsertMemoryResourceMonitor", s.Subtest(func(db database.Store, check *expects) {
		agt, _ := createAgent(s.T(), db)

		check.Args(database.InsertMemoryResourceMonitorParams{
			AgentID: agt.ID,
			State:   database.WorkspaceAgentMonitorStateOK,
		}).Asserts(rbac.ResourceWorkspaceAgentResourceMonitor, policy.ActionCreate)
	}))

	s.Run("InsertVolumeResourceMonitor", s.Subtest(func(db database.Store, check *expects) {
		agt, _ := createAgent(s.T(), db)

		check.Args(database.InsertVolumeResourceMonitorParams{
			AgentID: agt.ID,
			State:   database.WorkspaceAgentMonitorStateOK,
		}).Asserts(rbac.ResourceWorkspaceAgentResourceMonitor, policy.ActionCreate)
	}))

	s.Run("UpdateMemoryResourceMonitor", s.Subtest(func(db database.Store, check *expects) {
		agt, _ := createAgent(s.T(), db)

		check.Args(database.UpdateMemoryResourceMonitorParams{
			AgentID: agt.ID,
			State:   database.WorkspaceAgentMonitorStateOK,
		}).Asserts(rbac.ResourceWorkspaceAgentResourceMonitor, policy.ActionUpdate)
	}))

	s.Run("UpdateVolumeResourceMonitor", s.Subtest(func(db database.Store, check *expects) {
		agt, _ := createAgent(s.T(), db)

		check.Args(database.UpdateVolumeResourceMonitorParams{
			AgentID: agt.ID,
			State:   database.WorkspaceAgentMonitorStateOK,
		}).Asserts(rbac.ResourceWorkspaceAgentResourceMonitor, policy.ActionUpdate)
	}))

	s.Run("FetchMemoryResourceMonitorsUpdatedAfter", s.Subtest(func(db database.Store, check *expects) {
		check.Args(dbtime.Now()).Asserts(rbac.ResourceWorkspaceAgentResourceMonitor, policy.ActionRead)
	}))

	s.Run("FetchVolumesResourceMonitorsUpdatedAfter", s.Subtest(func(db database.Store, check *expects) {
		check.Args(dbtime.Now()).Asserts(rbac.ResourceWorkspaceAgentResourceMonitor, policy.ActionRead)
	}))

	s.Run("FetchMemoryResourceMonitorsByAgentID", s.Subtest(func(db database.Store, check *expects) {
		agt, w := createAgent(s.T(), db)

		dbgen.WorkspaceAgentMemoryResourceMonitor(s.T(), db, database.WorkspaceAgentMemoryResourceMonitor{
			AgentID:   agt.ID,
			Enabled:   true,
			Threshold: 80,
			CreatedAt: dbtime.Now(),
		})

		monitor, err := db.FetchMemoryResourceMonitorsByAgentID(context.Background(), agt.ID)
		require.NoError(s.T(), err)

		check.Args(agt.ID).Asserts(w, policy.ActionRead).Returns(monitor)
	}))

	s.Run("FetchVolumesResourceMonitorsByAgentID", s.Subtest(func(db database.Store, check *expects) {
		agt, w := createAgent(s.T(), db)

		dbgen.WorkspaceAgentVolumeResourceMonitor(s.T(), db, database.WorkspaceAgentVolumeResourceMonitor{
			AgentID:   agt.ID,
			Path:      "/var/lib",
			Enabled:   true,
			Threshold: 80,
			CreatedAt: dbtime.Now(),
		})

		monitors, err := db.FetchVolumesResourceMonitorsByAgentID(context.Background(), agt.ID)
		require.NoError(s.T(), err)

		check.Args(agt.ID).Asserts(w, policy.ActionRead).Returns(monitors)
	}))
}

func (s *MethodTestSuite) TestResourcesProvisionerdserver() {
	createAgent := func(t *testing.T, db database.Store) (database.WorkspaceAgent, database.WorkspaceTable) {
		t.Helper()

		u := dbgen.User(t, db, database.User{})
		o := dbgen.Organization(t, db, database.Organization{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		w := dbgen.Workspace(t, db, database.WorkspaceTable{
			TemplateID:     tpl.ID,
			OrganizationID: o.ID,
			OwnerID:        u.ID,
		})
		j := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
			Type: database.ProvisionerJobTypeWorkspaceBuild,
		})
		b := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
			JobID:             j.ID,
			WorkspaceID:       w.ID,
			TemplateVersionID: tv.ID,
		})
		res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: b.JobID})
		agt := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{ResourceID: res.ID})

		return agt, w
	}

	s.Run("InsertWorkspaceAgentDevcontainers", s.Subtest(func(db database.Store, check *expects) {
		agt, _ := createAgent(s.T(), db)
		check.Args(database.InsertWorkspaceAgentDevcontainersParams{
			WorkspaceAgentID: agt.ID,
		}).Asserts(rbac.ResourceWorkspaceAgentDevcontainers, policy.ActionCreate)
	}))
}

func (s *MethodTestSuite) TestAuthorizePrebuiltWorkspace() {
	s.Run("PrebuildDelete/InsertWorkspaceBuild", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		o := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID:     tpl.ID,
			OrganizationID: o.ID,
			OwnerID:        database.PrebuildsSystemUserID,
		})
		pj := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			OrganizationID: o.ID,
		})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		check.Args(database.InsertWorkspaceBuildParams{
			WorkspaceID:       w.ID,
			Transition:        database.WorkspaceTransitionDelete,
			Reason:            database.BuildReasonInitiator,
			TemplateVersionID: tv.ID,
			JobID:             pj.ID,
		}).
			// Simulate a fallback authorization flow:
			// - First, the default workspace authorization fails (simulated by returning an error).
			// - Then, authorization is retried using the prebuilt workspace object, which succeeds.
			// The test asserts that both authorization attempts occur in the correct order.
			WithSuccessAuthorizer(func(ctx context.Context, subject rbac.Subject, action policy.Action, obj rbac.Object) error {
				if obj.Type == rbac.ResourceWorkspace.Type {
					return xerrors.Errorf("not authorized for workspace type")
				}
				return nil
			}).Asserts(w, policy.ActionDelete, w.AsPrebuild(), policy.ActionDelete)
	}))
	s.Run("PrebuildUpdate/InsertWorkspaceBuildParameters", s.Subtest(func(db database.Store, check *expects) {
		u := dbgen.User(s.T(), db, database.User{})
		o := dbgen.Organization(s.T(), db, database.Organization{})
		tpl := dbgen.Template(s.T(), db, database.Template{
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		w := dbgen.Workspace(s.T(), db, database.WorkspaceTable{
			TemplateID:     tpl.ID,
			OrganizationID: o.ID,
			OwnerID:        database.PrebuildsSystemUserID,
		})
		pj := dbgen.ProvisionerJob(s.T(), db, nil, database.ProvisionerJob{
			OrganizationID: o.ID,
		})
		tv := dbgen.TemplateVersion(s.T(), db, database.TemplateVersion{
			TemplateID:     uuid.NullUUID{UUID: tpl.ID, Valid: true},
			OrganizationID: o.ID,
			CreatedBy:      u.ID,
		})
		wb := dbgen.WorkspaceBuild(s.T(), db, database.WorkspaceBuild{
			JobID:             pj.ID,
			WorkspaceID:       w.ID,
			TemplateVersionID: tv.ID,
		})
		check.Args(database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: wb.ID,
		}).
			// Simulate a fallback authorization flow:
			// - First, the default workspace authorization fails (simulated by returning an error).
			// - Then, authorization is retried using the prebuilt workspace object, which succeeds.
			// The test asserts that both authorization attempts occur in the correct order.
			WithSuccessAuthorizer(func(ctx context.Context, subject rbac.Subject, action policy.Action, obj rbac.Object) error {
				if obj.Type == rbac.ResourceWorkspace.Type {
					return xerrors.Errorf("not authorized for workspace type")
				}
				return nil
			}).Asserts(w, policy.ActionUpdate, w.AsPrebuild(), policy.ActionUpdate)
	}))
}

func (s *MethodTestSuite) TestUserSecrets() {
	s.Run("GetUserSecretByUserIDAndName", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		secret := testutil.Fake(s.T(), faker, database.UserSecret{UserID: user.ID})
		arg := database.GetUserSecretByUserIDAndNameParams{UserID: user.ID, Name: secret.Name}
		dbm.EXPECT().GetUserSecretByUserIDAndName(gomock.Any(), arg).Return(secret, nil).AnyTimes()
		check.Args(arg).
			Asserts(rbac.ResourceUserSecret.WithOwner(user.ID.String()), policy.ActionRead).
			Returns(secret)
	}))
	s.Run("GetUserSecret", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		secret := testutil.Fake(s.T(), faker, database.UserSecret{})
		dbm.EXPECT().GetUserSecret(gomock.Any(), secret.ID).Return(secret, nil).AnyTimes()
		check.Args(secret.ID).
			Asserts(secret, policy.ActionRead).
			Returns(secret)
	}))
	s.Run("ListUserSecrets", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		secret := testutil.Fake(s.T(), faker, database.UserSecret{UserID: user.ID})
		dbm.EXPECT().ListUserSecrets(gomock.Any(), user.ID).Return([]database.UserSecret{secret}, nil).AnyTimes()
		check.Args(user.ID).
			Asserts(rbac.ResourceUserSecret.WithOwner(user.ID.String()), policy.ActionRead).
			Returns([]database.UserSecret{secret})
	}))
	s.Run("CreateUserSecret", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		user := testutil.Fake(s.T(), faker, database.User{})
		arg := database.CreateUserSecretParams{UserID: user.ID}
		ret := testutil.Fake(s.T(), faker, database.UserSecret{UserID: user.ID})
		dbm.EXPECT().CreateUserSecret(gomock.Any(), arg).Return(ret, nil).AnyTimes()
		check.Args(arg).
			Asserts(rbac.ResourceUserSecret.WithOwner(user.ID.String()), policy.ActionCreate).
			Returns(ret)
	}))
	s.Run("UpdateUserSecret", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		secret := testutil.Fake(s.T(), faker, database.UserSecret{})
		updated := testutil.Fake(s.T(), faker, database.UserSecret{ID: secret.ID})
		arg := database.UpdateUserSecretParams{ID: secret.ID}
		dbm.EXPECT().GetUserSecret(gomock.Any(), secret.ID).Return(secret, nil).AnyTimes()
		dbm.EXPECT().UpdateUserSecret(gomock.Any(), arg).Return(updated, nil).AnyTimes()
		check.Args(arg).
			Asserts(secret, policy.ActionUpdate).
			Returns(updated)
	}))
	s.Run("DeleteUserSecret", s.Mocked(func(dbm *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		secret := testutil.Fake(s.T(), faker, database.UserSecret{})
		dbm.EXPECT().GetUserSecret(gomock.Any(), secret.ID).Return(secret, nil).AnyTimes()
		dbm.EXPECT().DeleteUserSecret(gomock.Any(), secret.ID).Return(nil).AnyTimes()
		check.Args(secret.ID).
			Asserts(secret, policy.ActionRead, secret, policy.ActionDelete).
			Returns()
	}))
}

func (s *MethodTestSuite) TestUsageEvents() {
	s.Run("InsertUsageEvent", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		params := database.InsertUsageEventParams{
			ID:        "1",
			EventType: "dc_managed_agents_v1",
			EventData: []byte("{}"),
			CreatedAt: dbtime.Now(),
		}
		db.EXPECT().InsertUsageEvent(gomock.Any(), params).Return(nil)
		check.Args(params).Asserts(rbac.ResourceUsageEvent, policy.ActionCreate)
	}))

	s.Run("SelectUsageEventsForPublishing", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		now := dbtime.Now()
		db.EXPECT().SelectUsageEventsForPublishing(gomock.Any(), now).Return([]database.UsageEvent{}, nil)
		check.Args(now).Asserts(rbac.ResourceUsageEvent, policy.ActionUpdate)
	}))

	s.Run("UpdateUsageEventsPostPublish", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		now := dbtime.Now()
		params := database.UpdateUsageEventsPostPublishParams{
			Now:             now,
			IDs:             []string{"1", "2"},
			FailureMessages: []string{"error", "error"},
			SetPublishedAts: []bool{false, false},
		}
		db.EXPECT().UpdateUsageEventsPostPublish(gomock.Any(), params).Return(nil)
		check.Args(params).Asserts(rbac.ResourceUsageEvent, policy.ActionUpdate)
	}))

	s.Run("GetTotalUsageDCManagedAgentsV1", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		db.EXPECT().GetTotalUsageDCManagedAgentsV1(gomock.Any(), gomock.Any()).Return(int64(1), nil)
		check.Args(database.GetTotalUsageDCManagedAgentsV1Params{
			StartDate: time.Time{},
			EndDate:   time.Time{},
		}).Asserts(rbac.ResourceUsageEvent, policy.ActionRead)
	}))
}

// Ensures that the prebuilds actor may never insert an api key.
func TestInsertAPIKey_AsPrebuildsUser(t *testing.T) {
	t.Parallel()
	prebuildsSubj := rbac.Subject{
		ID: database.PrebuildsSystemUserID.String(),
	}
	ctx := dbauthz.As(testutil.Context(t, testutil.WaitShort), prebuildsSubj)
	mDB := dbmock.NewMockStore(gomock.NewController(t))
	log := slogtest.Make(t, nil)
	mDB.EXPECT().Wrappers().Times(1).Return([]string{})
	dbz := dbauthz.New(mDB, nil, log, nil)
	faker := gofakeit.New(0)
	_, err := dbz.InsertAPIKey(ctx, testutil.Fake(t, faker, database.InsertAPIKeyParams{}))
	require.True(t, dbauthz.IsNotAuthorizedError(err))
}

func (s *MethodTestSuite) TestAIBridge() {
	s.Run("InsertAIBridgeInterception", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		initID := uuid.UUID{3}
		user := testutil.Fake(s.T(), faker, database.User{ID: initID})
		// testutil.Fake cannot distinguish between a zero value and an explicitly requested value which is equivalent.
		user.IsSystem = false
		user.Deleted = false

		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID, InitiatorID: initID})

		params := database.InsertAIBridgeInterceptionParams{ID: intc.ID, InitiatorID: intc.InitiatorID, Provider: intc.Provider, Model: intc.Model}
		db.EXPECT().GetUserByID(gomock.Any(), initID).Return(user, nil).AnyTimes() // Validation.
		db.EXPECT().InsertAIBridgeInterception(gomock.Any(), params).Return(intc, nil).AnyTimes()
		check.Args(params).Asserts(intc, policy.ActionCreate)
	}))

	s.Run("InsertAIBridgeTokenUsage", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID})
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intID).Return(intc, nil).AnyTimes() // Validation.

		params := database.InsertAIBridgeTokenUsageParams{InterceptionID: intc.ID}
		expected := testutil.Fake(s.T(), faker, database.AIBridgeTokenUsage{InterceptionID: intc.ID})
		db.EXPECT().InsertAIBridgeTokenUsage(gomock.Any(), params).Return(expected, nil).AnyTimes()
		check.Args(params).Asserts(intc, policy.ActionUpdate)
	}))

	s.Run("InsertAIBridgeUserPrompt", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID})
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intID).Return(intc, nil).AnyTimes() // Validation.

		params := database.InsertAIBridgeUserPromptParams{InterceptionID: intc.ID}
		expected := testutil.Fake(s.T(), faker, database.AIBridgeUserPrompt{InterceptionID: intc.ID})
		db.EXPECT().InsertAIBridgeUserPrompt(gomock.Any(), params).Return(expected, nil).AnyTimes()
		check.Args(params).Asserts(intc, policy.ActionUpdate)
	}))

	s.Run("InsertAIBridgeToolUsage", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID})
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intID).Return(intc, nil).AnyTimes() // Validation.

		params := database.InsertAIBridgeToolUsageParams{InterceptionID: intc.ID}
		expected := testutil.Fake(s.T(), faker, database.AIBridgeToolUsage{InterceptionID: intc.ID})
		db.EXPECT().InsertAIBridgeToolUsage(gomock.Any(), params).Return(expected, nil).AnyTimes()
		check.Args(params).Asserts(intc, policy.ActionUpdate)
	}))

	s.Run("GetAIBridgeInterceptionByID", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID})
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intID).Return(intc, nil).AnyTimes()
		check.Args(intID).Asserts(intc, policy.ActionRead).Returns(intc)
	}))

	s.Run("GetAIBridgeInterceptions", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		a := testutil.Fake(s.T(), faker, database.AIBridgeInterception{})
		b := testutil.Fake(s.T(), faker, database.AIBridgeInterception{})
		db.EXPECT().GetAIBridgeInterceptions(gomock.Any()).Return([]database.AIBridgeInterception{a, b}, nil).AnyTimes()
		check.Args().Asserts(a, policy.ActionRead, b, policy.ActionRead).Returns([]database.AIBridgeInterception{a, b})
	}))

	s.Run("GetAIBridgeTokenUsagesByInterceptionID", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID})
		tok := testutil.Fake(s.T(), faker, database.AIBridgeTokenUsage{InterceptionID: intID})
		toks := []database.AIBridgeTokenUsage{tok}
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intID).Return(intc, nil).AnyTimes() // Validation.
		db.EXPECT().GetAIBridgeTokenUsagesByInterceptionID(gomock.Any(), intID).Return(toks, nil).AnyTimes()
		check.Args(intID).Asserts(intc, policy.ActionRead).Returns(toks)
	}))

	s.Run("GetAIBridgeUserPromptsByInterceptionID", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID})
		pr := testutil.Fake(s.T(), faker, database.AIBridgeUserPrompt{InterceptionID: intID})
		prs := []database.AIBridgeUserPrompt{pr}
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intID).Return(intc, nil).AnyTimes() // Validation.
		db.EXPECT().GetAIBridgeUserPromptsByInterceptionID(gomock.Any(), intID).Return(prs, nil).AnyTimes()
		check.Args(intID).Asserts(intc, policy.ActionRead).Returns(prs)
	}))

	s.Run("GetAIBridgeToolUsagesByInterceptionID", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intID := uuid.UUID{2}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intID})
		tool := testutil.Fake(s.T(), faker, database.AIBridgeToolUsage{InterceptionID: intID})
		tools := []database.AIBridgeToolUsage{tool}
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intID).Return(intc, nil).AnyTimes() // Validation.
		db.EXPECT().GetAIBridgeToolUsagesByInterceptionID(gomock.Any(), intID).Return(tools, nil).AnyTimes()
		check.Args(intID).Asserts(intc, policy.ActionRead).Returns(tools)
	}))

	s.Run("ListAIBridgeInterceptions", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		params := database.ListAIBridgeInterceptionsParams{}
		db.EXPECT().ListAuthorizedAIBridgeInterceptions(gomock.Any(), params, gomock.Any()).Return([]database.ListAIBridgeInterceptionsRow{}, nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(params).Asserts()
	}))

	s.Run("ListAuthorizedAIBridgeInterceptions", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		params := database.ListAIBridgeInterceptionsParams{}
		db.EXPECT().ListAuthorizedAIBridgeInterceptions(gomock.Any(), params, gomock.Any()).Return([]database.ListAIBridgeInterceptionsRow{}, nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(params, emptyPreparedAuthorized{}).Asserts()
	}))

	s.Run("CountAIBridgeInterceptions", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		params := database.CountAIBridgeInterceptionsParams{}
		db.EXPECT().CountAuthorizedAIBridgeInterceptions(gomock.Any(), params, gomock.Any()).Return(int64(0), nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(params).Asserts()
	}))

	s.Run("CountAuthorizedAIBridgeInterceptions", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		params := database.CountAIBridgeInterceptionsParams{}
		db.EXPECT().CountAuthorizedAIBridgeInterceptions(gomock.Any(), params, gomock.Any()).Return(int64(0), nil).AnyTimes()
		// No asserts here because SQLFilter.
		check.Args(params, emptyPreparedAuthorized{}).Asserts()
	}))

	s.Run("ListAIBridgeTokenUsagesByInterceptionIDs", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{{1}}
		db.EXPECT().ListAIBridgeTokenUsagesByInterceptionIDs(gomock.Any(), ids).Return([]database.AIBridgeTokenUsage{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns([]database.AIBridgeTokenUsage{})
	}))

	s.Run("ListAIBridgeUserPromptsByInterceptionIDs", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{{1}}
		db.EXPECT().ListAIBridgeUserPromptsByInterceptionIDs(gomock.Any(), ids).Return([]database.AIBridgeUserPrompt{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns([]database.AIBridgeUserPrompt{})
	}))

	s.Run("ListAIBridgeToolUsagesByInterceptionIDs", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		ids := []uuid.UUID{{1}}
		db.EXPECT().ListAIBridgeToolUsagesByInterceptionIDs(gomock.Any(), ids).Return([]database.AIBridgeToolUsage{}, nil).AnyTimes()
		check.Args(ids).Asserts(rbac.ResourceSystem, policy.ActionRead).Returns([]database.AIBridgeToolUsage{})
	}))

	s.Run("UpdateAIBridgeInterceptionEnded", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		intcID := uuid.UUID{1}
		params := database.UpdateAIBridgeInterceptionEndedParams{ID: intcID}
		intc := testutil.Fake(s.T(), faker, database.AIBridgeInterception{ID: intcID})
		db.EXPECT().GetAIBridgeInterceptionByID(gomock.Any(), intcID).Return(intc, nil).AnyTimes() // Validation.
		db.EXPECT().UpdateAIBridgeInterceptionEnded(gomock.Any(), params).Return(intc, nil).AnyTimes()
		check.Args(params).Asserts(intc, policy.ActionUpdate).Returns(intc)
	}))

	s.Run("DeleteOldAIBridgeRecords", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		t := dbtime.Now()
		db.EXPECT().DeleteOldAIBridgeRecords(gomock.Any(), t).Return(int32(0), nil).AnyTimes()
		check.Args(t).Asserts(rbac.ResourceAibridgeInterception, policy.ActionDelete)
	}))
}

func (s *MethodTestSuite) TestTelemetry() {
	s.Run("InsertTelemetryLock", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		db.EXPECT().InsertTelemetryLock(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		check.Args(database.InsertTelemetryLockParams{}).Asserts(rbac.ResourceSystem, policy.ActionCreate)
	}))

	s.Run("DeleteOldTelemetryLocks", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		db.EXPECT().DeleteOldTelemetryLocks(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		check.Args(time.Time{}).Asserts(rbac.ResourceSystem, policy.ActionDelete)
	}))

	s.Run("ListAIBridgeInterceptionsTelemetrySummaries", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		db.EXPECT().ListAIBridgeInterceptionsTelemetrySummaries(gomock.Any(), gomock.Any()).Return([]database.ListAIBridgeInterceptionsTelemetrySummariesRow{}, nil).AnyTimes()
		check.Args(database.ListAIBridgeInterceptionsTelemetrySummariesParams{}).Asserts(rbac.ResourceAibridgeInterception, policy.ActionRead)
	}))

	s.Run("CalculateAIBridgeInterceptionsTelemetrySummary", s.Mocked(func(db *dbmock.MockStore, faker *gofakeit.Faker, check *expects) {
		db.EXPECT().CalculateAIBridgeInterceptionsTelemetrySummary(gomock.Any(), gomock.Any()).Return(database.CalculateAIBridgeInterceptionsTelemetrySummaryRow{}, nil).AnyTimes()
		check.Args(database.CalculateAIBridgeInterceptionsTelemetrySummaryParams{}).Asserts(rbac.ResourceAibridgeInterception, policy.ActionRead)
	}))
}
