package coderdtest_test

import (
	"context"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func TestAuthzRecorder(t *testing.T) {
	t.Parallel()

	t.Run("Authorize", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		sub := coderdtest.RandomRBACSubject()
		pairs := fuzzAuthz(t, sub, rec, 10)
		rec.AssertActor(t, sub, pairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("Authorize2Subjects", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		a := coderdtest.RandomRBACSubject()
		aPairs := fuzzAuthz(t, a, rec, 10)

		b := coderdtest.RandomRBACSubject()
		bPairs := fuzzAuthz(t, b, rec, 10)

		rec.AssertActor(t, b, bPairs...)
		rec.AssertActor(t, a, aPairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("Authorize_Prepared", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		a := coderdtest.RandomRBACSubject()
		aPairs := fuzzAuthz(t, a, rec, 10)

		b := coderdtest.RandomRBACSubject()

		act, objTy := coderdtest.RandomRBACAction(), coderdtest.RandomRBACObject().Type
		prep, _ := rec.Prepare(context.Background(), b, act, objTy)
		bPairs := fuzzAuthzPrep(t, prep, 10, act, objTy)

		rec.AssertActor(t, b, bPairs...)
		rec.AssertActor(t, a, aPairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("AuthorizeOutOfOrder", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		sub := coderdtest.RandomRBACSubject()
		pairs := fuzzAuthz(t, sub, rec, 10)
		rand.Shuffle(len(pairs), func(i, j int) {
			pairs[i], pairs[j] = pairs[j], pairs[i]
		})

		rec.AssertOutOfOrder(t, sub, pairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("AllCalls", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		sub := coderdtest.RandomRBACSubject()
		calls := rec.AllCalls(&sub)
		pairs := make([]coderdtest.ActionObjectPair, 0, len(calls))
		for _, call := range calls {
			pairs = append(pairs, coderdtest.ActionObjectPair{
				Action: call.Action,
				Object: call.Object,
			})
		}

		rec.AssertActor(t, sub, pairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})
}

// fuzzAuthzPrep has same action and object types for all calls.
func fuzzAuthzPrep(t *testing.T, prep rbac.PreparedAuthorized, n int, action policy.Action, objectType string) []coderdtest.ActionObjectPair {
	t.Helper()
	pairs := make([]coderdtest.ActionObjectPair, 0, n)

	for i := 0; i < n; i++ {
		obj := coderdtest.RandomRBACObject()
		obj.Type = objectType
		p := coderdtest.ActionObjectPair{Action: action, Object: obj}
		_ = prep.Authorize(context.Background(), p.Object)
		pairs = append(pairs, p)
	}
	return pairs
}

func fuzzAuthz(t *testing.T, sub rbac.Subject, rec rbac.Authorizer, n int) []coderdtest.ActionObjectPair {
	t.Helper()
	pairs := make([]coderdtest.ActionObjectPair, 0, n)

	for i := 0; i < n; i++ {
		p := coderdtest.ActionObjectPair{Action: coderdtest.RandomRBACAction(), Object: coderdtest.RandomRBACObject()}
		_ = rec.Authorize(context.Background(), sub, p.Action, p.Object)
		pairs = append(pairs, p)
	}
	return pairs
}
