package authzquery_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/databasefake"
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
}

// TestAuthzQueryRecursive is a simple test to search for infinite recursion
// bugs. It isn't perfect, and only catches a subset of the possible bugs
// as only the first db call will be made. But it is better than nothing.
func TestAuthzQueryRecursive(t *testing.T) {
	t.Parallel()
	q := authzquery.NewAuthzQuerier(databasefake.New(), &coderdtest.RecordingAuthorizer{
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

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
