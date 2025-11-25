package coderdtest

import (
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
)

func MockedDatabaseWithAuthz(t testing.TB, logger slog.Logger) (*gomock.Controller, *dbmock.MockStore, database.Store, rbac.Authorizer) {
	ctrl := gomock.NewController(t)
	mDB := dbmock.NewMockStore(ctrl)
	auth := rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry())
	accessControlStore := &atomic.Pointer[dbauthz.AccessControlStore]{}
	var acs dbauthz.AccessControlStore = dbauthz.AGPLTemplateAccessControlStore{}
	accessControlStore.Store(&acs)
	// dbauthz will call Wrappers() to check for wrapped databases
	mDB.EXPECT().Wrappers().Return([]string{}).AnyTimes()
	authDB := dbauthz.New(mDB, auth, logger, accessControlStore)
	return ctrl, mDB, authDB, auth
}
