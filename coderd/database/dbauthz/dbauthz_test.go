package dbauthz_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/rbac"
)

func TestAsNoActor(t *testing.T) {
	t.Parallel()

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

	q := dbauthz.New(dbfake.New(), &coderdtest.RecordingAuthorizer{}, slog.Make())
	_, err := q.Ping(context.Background())
	require.NoError(t, err, "must not error")
}

// TestInTX is not perfect, just checks that it properly checks auth.
func TestInTX(t *testing.T) {
	t.Parallel()

	db := dbfake.New()
	q := dbauthz.New(db, &coderdtest.RecordingAuthorizer{
		Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: xerrors.New("custom error")},
	}, slog.Make())
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
}

// TestNew should not double wrap a querier.
func TestNew(t *testing.T) {
	t.Parallel()

	var (
		db  = dbfake.New()
		exp = dbgen.Workspace(t, db, database.Workspace{})
		rec = &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
		}
		subj = rbac.Subject{}
		ctx  = dbauthz.As(context.Background(), rbac.Subject{})
	)

	// Double wrap should not cause an actual double wrap. So only 1 rbac call
	// should be made.
	az := dbauthz.New(db, rec, slog.Make())
	az = dbauthz.New(az, rec, slog.Make())

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
	q := dbauthz.New(dbfake.New(), &coderdtest.RecordingAuthorizer{
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
		ctx := dbauthz.As(context.Background(), actor)

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
