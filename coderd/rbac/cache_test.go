package rbac_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
)

// BenchmarkCacher benchmarks the performance of the cacher with a given
// cache size. The expected cache size in prod will usually be 1-2. In Filter
// cases it can get as high as 10.
func BenchmarkCacher(b *testing.B) {
	b.ResetTimer()
	// Size of the cache.
	sizes := []int{1, 10, 100, 1000}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			ctx := rbac.WithCacheCtx(context.Background())
			authz := rbac.Cacher(&coderdtest.FakeAuthorizer{AlwaysReturn: nil})
			for i := 0; i < size; i++ {
				// Preload the cache of a given size
				subj, obj, action := coderdtest.RandomRBACSubject(), coderdtest.RandomRBACObject(), coderdtest.RandomRBACAction()
				_ = authz.Authorize(ctx, subj, action, obj)
			}

			// Cache is loaded as a slice, so this cache hit is always the last element.
			subj, obj, action := coderdtest.RandomRBACSubject(), coderdtest.RandomRBACObject(), coderdtest.RandomRBACAction()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = authz.Authorize(ctx, subj, action, obj)
			}
		})
	}
}

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
