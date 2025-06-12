package files_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func TestCacheRBAC(t *testing.T) {
	t.Parallel()

	db, cache, rec := cacheAuthzSetup(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	file := dbgen.File(t, db, database.File{})

	nobodyID := uuid.New()
	nobody := dbauthz.As(ctx, rbac.Subject{
		ID:    nobodyID.String(),
		Roles: rbac.Roles{},
		Scope: rbac.ScopeAll,
	})

	userID := uuid.New()
	userReader := dbauthz.As(ctx, rbac.Subject{
		ID: userID.String(),
		Roles: rbac.Roles{
			must(rbac.RoleByName(rbac.RoleTemplateAdmin())),
		},
		Scope: rbac.ScopeAll,
	})

	cacheReader := dbauthz.AsFileReader(ctx)

	t.Run("NoRolesOpen", func(t *testing.T) {
		// Ensure start is clean
		require.Equal(t, 0, cache.Count())
		rec.Reset()

		_, err := cache.Acquire(nobody, file.ID)
		require.Error(t, err)
		require.True(t, rbac.IsUnauthorizedError(err))

		// Ensure that the cache is empty
		require.Equal(t, 0, cache.Count())

		// Check the assertions
		rec.AssertActorID(t, nobodyID.String(), rec.Pair(policy.ActionRead, file))
		rec.AssertActorID(t, rbac.SubjectTypeFileReaderID, rec.Pair(policy.ActionRead, file))
	})

	t.Run("CacheHasFile", func(t *testing.T) {
		rec.Reset()
		require.Equal(t, 0, cache.Count())

		// Read the file with a file reader to put it into the cache.
		_, err := cache.Acquire(cacheReader, file.ID)
		require.NoError(t, err)
		require.Equal(t, 1, cache.Count())

		// "nobody" should not be able to read the file.
		_, err = cache.Acquire(nobody, file.ID)
		require.Error(t, err)
		require.True(t, rbac.IsUnauthorizedError(err))
		require.Equal(t, 1, cache.Count())

		// UserReader can
		_, err = cache.Acquire(userReader, file.ID)
		require.NoError(t, err)
		require.Equal(t, 1, cache.Count())

		cache.Release(file.ID)
		cache.Release(file.ID)
		require.Equal(t, 0, cache.Count())

		rec.AssertActorID(t, nobodyID.String(), rec.Pair(policy.ActionRead, file))
		rec.AssertActorID(t, rbac.SubjectTypeFileReaderID, rec.Pair(policy.ActionRead, file))
		rec.AssertActorID(t, userID.String(), rec.Pair(policy.ActionRead, file))
	})
}

func cacheAuthzSetup(t *testing.T) (database.Store, *files.Cache, *coderdtest.RecordingAuthorizer) {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{})
	reg := prometheus.NewRegistry()

	db, _ := dbtestutil.NewDB(t)
	authz := rbac.NewAuthorizer(reg)
	rec := &coderdtest.RecordingAuthorizer{
		Called:  nil,
		Wrapped: authz,
	}

	// Dbauthz wrap the db
	db = dbauthz.New(db, rec, logger, coderdtest.AccessControlStorePointer())
	c := files.NewFromStore(db, reg, rec.Authorize)
	return db, c, rec
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
