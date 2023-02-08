package rbac_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
)

func TestCacher(t *testing.T) {
	t.Parallel()

	t.Run("EmptyCacheCtx", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
		}
		authz := rbac.Cacher(rec)
		subj, obj, action := coderdtest.RandomRBACSubject(), coderdtest.RandomRBACObject(), coderdtest.RandomRBACAction()

		// Two identical calls
		_ = authz.Authorize(ctx, subj, action, obj)
		_ = authz.Authorize(ctx, subj, action, obj)

		// Yields two calls to the wrapped Authorizer
		rec.AssertActor(t, subj, rec.Pair(action, obj), rec.Pair(action, obj))
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("CacheCtx", func(t *testing.T) {
		t.Parallel()

		ctx := rbac.WithCacheCtx(context.Background())
		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
		}
		authz := rbac.Cacher(rec)
		subj, obj, action := coderdtest.RandomRBACSubject(), coderdtest.RandomRBACObject(), coderdtest.RandomRBACAction()

		// Two identical calls
		_ = authz.Authorize(ctx, subj, action, obj)
		_ = authz.Authorize(ctx, subj, action, obj)

		// Yields only 1 call to the wrapped Authorizer for that subject
		rec.AssertActor(t, subj, rec.Pair(action, obj))
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("MultipleSubjects", func(t *testing.T) {
		t.Parallel()

		ctx := rbac.WithCacheCtx(context.Background())
		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{AlwaysReturn: nil},
		}
		authz := rbac.Cacher(rec)
		subj1, obj1, action1 := coderdtest.RandomRBACSubject(), coderdtest.RandomRBACObject(), coderdtest.RandomRBACAction()

		// Two identical calls
		_ = authz.Authorize(ctx, subj1, action1, obj1)
		_ = authz.Authorize(ctx, subj1, action1, obj1)

		// Extra unique calls
		var pairs []coderdtest.ActionObjectPair
		subj2, obj2, action2 := coderdtest.RandomRBACSubject(), coderdtest.RandomRBACObject(), coderdtest.RandomRBACAction()
		_ = authz.Authorize(ctx, subj2, action2, obj2)
		pairs = append(pairs, rec.Pair(action2, obj2))

		obj3, action3 := coderdtest.RandomRBACObject(), coderdtest.RandomRBACAction()
		_ = authz.Authorize(ctx, subj2, action3, obj3)
		pairs = append(pairs, rec.Pair(action3, obj3))

		// Extra identical call after some unique calls
		_ = authz.Authorize(ctx, subj1, action1, obj1)

		// Yields 3 calls, 1 for the first subject, 2 for the unique subjects
		rec.AssertActor(t, subj1, rec.Pair(action1, obj1))
		rec.AssertActor(t, subj2, pairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})
}
