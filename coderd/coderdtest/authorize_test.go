package coderdtest_test

import (
	"context"
	"testing"

	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
)

func TestAuthorizeAllEndpoints(t *testing.T) {
	t.Parallel()
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		// Required for any subdomain-based proxy tests to pass.
		AppHostname:              "*.test.coder.com",
		Authorizer:               &coderdtest.RecordingAuthorizer{Wrapped: &coderdtest.FakeAuthorizer{}},
		IncludeProvisionerDaemon: true,
	})
	admin := coderdtest.CreateFirstUser(t, client)
	a := coderdtest.NewAuthTester(context.Background(), t, client, api, admin)
	skipRoute, assertRoute := coderdtest.AGPLRoutes(a)
	a.Test(context.Background(), assertRoute, skipRoute)
}

func TestAuthzRecorder(t *testing.T) {
	t.Parallel()

	t.Run("Authorize", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		sub := randomSubject()
		pairs := fuzzAuthz(t, sub, rec, 10)
		rec.AssertActor(t, sub, pairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("Authorize2Subjects", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		a := randomSubject()
		aPairs := fuzzAuthz(t, a, rec, 10)

		b := randomSubject()
		bPairs := fuzzAuthz(t, b, rec, 10)

		rec.AssertActor(t, b, bPairs...)
		rec.AssertActor(t, a, aPairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})

	t.Run("Authorize&Prepared", func(t *testing.T) {
		t.Parallel()

		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: &coderdtest.FakeAuthorizer{},
		}
		a := randomSubject()
		aPairs := fuzzAuthz(t, a, rec, 10)

		b := randomSubject()

		act, objTy := randomAction(), randomObject().Type
		prep, _ := rec.Prepare(context.Background(), b, act, objTy)
		bPairs := fuzzAuthzPrep(t, prep, 10, act, objTy)

		rec.AssertActor(t, b, bPairs...)
		rec.AssertActor(t, a, aPairs...)
		require.NoError(t, rec.AllAsserted(), "all assertions should have been made")
	})
}

// fuzzAuthzPrep has same action and object types for all calls.
func fuzzAuthzPrep(t *testing.T, prep rbac.PreparedAuthorized, n int, action rbac.Action, objectType string) []coderdtest.ActionObjectPair {
	t.Helper()
	pairs := make([]coderdtest.ActionObjectPair, 0, n)

	for i := 0; i < n; i++ {
		obj := randomObject()
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
		p := coderdtest.ActionObjectPair{Action: randomAction(), Object: randomObject()}
		_ = rec.Authorize(context.Background(), sub, p.Action, p.Object)
		pairs = append(pairs, p)
	}
	return pairs
}

func randomAction() rbac.Action {
	return rbac.Action(namesgenerator.GetRandomName(1))
}

func randomObject() rbac.Object {
	return rbac.Object{
		ID:    namesgenerator.GetRandomName(1),
		Owner: namesgenerator.GetRandomName(1),
		OrgID: namesgenerator.GetRandomName(1),
		Type:  namesgenerator.GetRandomName(1),
		ACLUserList: map[string][]rbac.Action{
			namesgenerator.GetRandomName(1): {rbac.ActionRead},
		},
		ACLGroupList: map[string][]rbac.Action{
			namesgenerator.GetRandomName(1): {rbac.ActionRead},
		},
	}
}

func randomSubject() rbac.Subject {
	return rbac.Subject{
		ID:     namesgenerator.GetRandomName(1),
		Roles:  rbac.RoleNames{rbac.RoleMember()},
		Groups: []string{namesgenerator.GetRandomName(1)},
		Scope:  rbac.ScopeAll,
	}
}
