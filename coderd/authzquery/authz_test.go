package authzquery_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func TestNotAuthorizedError(t *testing.T) {
	t.Parallel()

	t.Run("Is404", func(t *testing.T) {
		t.Parallel()

		testErr := xerrors.New("custom error")

		err := authzquery.LogNotAuthorizedError(context.Background(), slogtest.Make(t, nil), testErr)
		require.ErrorIs(t, err, sql.ErrNoRows, "must be a sql.ErrNoRows")

		var authErr authzquery.NotAuthorizedError
		require.ErrorAs(t, err, &authErr, "must be a NotAuthorizedError")
		require.ErrorIs(t, authErr.Err, testErr, "internal error must match")
	})

	t.Run("MissingActor", func(t *testing.T) {
		q := authzquery.NewAuthzQuerier(dbfake.New(), &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
		}, slog.Make())
		// This should fail because the actor is missing.
		_, err := q.GetWorkspaceByID(context.Background(), uuid.New())
		require.ErrorIs(t, err, authzquery.NoActorError, "must be a NoActorError")
	})
}

// TestAuthzQueryRecursive is a simple test to search for infinite recursion
// bugs. It isn't perfect, and only catches a subset of the possible bugs
// as only the first db call will be made. But it is better than nothing.
func TestAuthzQueryRecursive(t *testing.T) {
	t.Parallel()
	q := authzquery.NewAuthzQuerier(dbfake.New(), &coderdtest.RecordingAuthorizer{
		Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
	}, slog.Make())
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}
	for i := 0; i < reflect.TypeOf(q).NumMethod(); i++ {
		var ins []reflect.Value
		ctx := authzquery.WithAuthorizeContext(context.Background(), actor)

		ins = append(ins, reflect.ValueOf(ctx))
		method := reflect.TypeOf(q).Method(i)
		for i := 2; i < method.Type.NumIn(); i++ {
			ins = append(ins, reflect.New(method.Type.In(i)).Elem())
		}
		if method.Name == "InTx" || method.Name == "Ping" {
			continue
		}
		// Log the name of the last method, so if there is a panic, it is
		// easy to know which method failed.
		// t.Log(method.Name)
		// Call the function. Any infinite recursion will stack overflow.
		reflect.ValueOf(q).Method(i).Call(ins)
	}
}

func TestPing(t *testing.T) {
	t.Parallel()

	q := authzquery.NewAuthzQuerier(dbfake.New(), &coderdtest.RecordingAuthorizer{}, slog.Make())
	_, err := q.Ping(context.Background())
	require.NoError(t, err, "must not error")
}

// TestInTX is not perfect, just checks that it properly checks auth.
func TestInTX(t *testing.T) {
	t.Parallel()

	db := dbfake.New()
	q := authzquery.NewAuthzQuerier(db, &coderdtest.RecordingAuthorizer{
		Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: xerrors.New("custom error")},
	}, slog.Make())
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}

	w := dbgen.Workspace(t, db, database.Workspace{})
	ctx := authzquery.WithAuthorizeContext(context.Background(), actor)
	err := q.InTx(func(tx database.Store) error {
		// The inner tx should use the parent's authz
		_, err := tx.GetWorkspaceByID(ctx, w.ID)
		return err
	}, nil)
	require.Error(t, err, "must error")
	require.ErrorAs(t, err, &authzquery.NotAuthorizedError{}, "must be an authorized error")
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
