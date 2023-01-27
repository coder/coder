package authzquery_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/moby/moby/pkg/namesgenerator"

	"github.com/coder/coder/testutil"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/coder/coder/coderd/rbac"
)

// TestAuthzQueryRecursive is a simple test to search for infinite recursion
// bugs. It isn't perfect, and only catches a subset of the possible bugs
// as only the first db call will be made. But it is better than nothing.
func TestAuthzQueryRecursive(t *testing.T) {
	t.Parallel()
	q := authzquery.NewAuthzQuerier(databasefake.New(), &coderdtest.RecordingAuthorizer{})
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
		t.Logf(method.Name, method.Type.NumIn(), len(ins))
		reflect.ValueOf(q).Method(i).Call(ins)
	}
}

type authorizeTest struct {
	Data func(t *testing.T, tc *authorizeTest) map[string]interface{}
	// Test is all the calls to the AuthzStore
	Test func(ctx context.Context, t *testing.T, tc *authorizeTest, q authzquery.AuthzStore)
	// Assert is the objects and the expected RBAC calls.
	// If 2 reads are expected on the same object, pass in 2 rbac.Reads.
	Asserts map[string][]rbac.Action

	names map[string]uuid.UUID
}

func (tc *authorizeTest) Lookup(name string) uuid.UUID {
	if tc.names == nil {
		tc.names = make(map[string]uuid.UUID)
	}
	if id, ok := tc.names[name]; ok {
		return id
	}
	id := uuid.New()
	tc.names[name] = id
	return id
}

func testAuthorizeFunction(t *testing.T, testCase *authorizeTest) {
	t.Helper()

	// The actor does not really matter since all authz calls will succeed.
	actor := rbac.Subject{
		ID:     uuid.New().String(),
		Roles:  rbac.RoleNames{},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}

	// Always use a fake database.
	db := databasefake.New()

	// Record all authorization calls. This will allow all authorization calls
	// to succeed.
	rec := &coderdtest.RecordingAuthorizer{}
	q := authzquery.NewAuthzQuerier(db, rec)

	// Setup Context
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	ctx = authzquery.WithAuthorizeContext(ctx, actor)
	t.Cleanup(cancel)

	// Seed all data into the database that is required for the test.
	data := setupTestData(t, testCase, db, ctx)

	// Run the test.
	testCase.Test(ctx, t, testCase, q)

	// Asset RBAC calls.
	pairs := make([]coderdtest.ActionObjectPair, 0)
	for objectName, asserts := range testCase.Asserts {
		object := data[objectName]
		for _, assert := range asserts {
			canRBAC, ok := object.(rbac.Objecter)
			require.True(t, ok, "object %q does not implement rbac.Objecter", objectName)
			pairs = append(pairs, rec.Pair(assert, canRBAC.RBACObject()))
		}
	}
	rec.UnorderedAssertActor(t, actor, pairs...)
	require.NoError(t, rec.AllAsserted(), "all authz checks asserted")
}

func setupTestData(t *testing.T, testCase *authorizeTest, db database.Store, ctx context.Context) map[string]interface{} {
	// Setup the test data.
	orgID := uuid.New()
	data := testCase.Data(t, testCase)
	for name, v := range data {
		switch orig := v.(type) {
		case database.Template:
			template, err := db.InsertTemplate(ctx, database.InsertTemplateParams{
				ID:                           testCase.Lookup(name),
				CreatedAt:                    time.Now(),
				UpdatedAt:                    time.Now(),
				OrganizationID:               takeFirst(orig.OrganizationID, orgID),
				Name:                         takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				Provisioner:                  takeFirst(orig.Provisioner, database.ProvisionerTypeEcho),
				ActiveVersionID:              takeFirst(orig.ActiveVersionID, uuid.New()),
				Description:                  takeFirst(orig.Description, namesgenerator.GetRandomName(1)),
				DefaultTTL:                   takeFirst(orig.DefaultTTL, 3600),
				CreatedBy:                    takeFirst(orig.CreatedBy, uuid.New()),
				Icon:                         takeFirst(orig.Icon, namesgenerator.GetRandomName(1)),
				UserACL:                      orig.UserACL,
				GroupACL:                     orig.GroupACL,
				DisplayName:                  takeFirst(orig.DisplayName, namesgenerator.GetRandomName(1)),
				AllowUserCancelWorkspaceJobs: takeFirst(orig.AllowUserCancelWorkspaceJobs, true),
			})
			require.NoError(t, err, "insert template")

			data[name] = template
		case database.Workspace:
			workspace, err := db.InsertWorkspace(ctx, database.InsertWorkspaceParams{
				ID:                testCase.Lookup(name),
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
				OrganizationID:    takeFirst(orig.OrganizationID, orgID),
				TemplateID:        takeFirst(orig.TemplateID, uuid.New()),
				Name:              takeFirst(orig.Name, namesgenerator.GetRandomName(1)),
				AutostartSchedule: orig.AutostartSchedule,
				Ttl:               orig.Ttl,
			})
			require.NoError(t, err, "insert workspace")

			data[name] = workspace
		case database.WorkspaceBuild:
			build, err := db.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
				ID:                testCase.Lookup(name),
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
				WorkspaceID:       takeFirst(orig.WorkspaceID, uuid.New()),
				TemplateVersionID: takeFirst(orig.TemplateVersionID, uuid.New()),
				BuildNumber:       takeFirst(orig.BuildNumber, 0),
				Transition:        takeFirst(orig.Transition, database.WorkspaceTransitionStart),
				InitiatorID:       takeFirst(orig.InitiatorID, uuid.New()),
				JobID:             takeFirst(orig.InitiatorID, uuid.New()),
				ProvisionerState:  []byte{},
				Deadline:          time.Now(),
				Reason:            takeFirst(orig.Reason, database.BuildReasonInitiator),
			})
			require.NoError(t, err, "insert workspace build")

			data[name] = build
		case database.User:
			user, err := db.InsertUser(ctx, database.InsertUserParams{
				ID:             testCase.Lookup(name),
				Email:          takeFirst(orig.Email, namesgenerator.GetRandomName(1)),
				Username:       takeFirst(orig.Username, namesgenerator.GetRandomName(1)),
				HashedPassword: []byte{},
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				RBACRoles:      []string{},
				LoginType:      takeFirst(orig.LoginType, database.LoginTypePassword),
			})
			require.NoError(t, err, "insert user")

			data[name] = user
		}
	}
	return data
}

// takeFirst will take the first non empty value.
func takeFirst[Value comparable](def Value, next Value) Value {
	var empty Value
	if def == empty {
		return next
	}
	return def
}
