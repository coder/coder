package authzquery_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/coder/coder/coderd/rbac/regosql"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/rbac"
)

var (
	skipMethods = map[string]string{
		"InTx":                    "Not relevant",
		"Ping":                    "Not relevant",
		"GetAuthorizedWorkspaces": "Will not be exposed",
	}
)

// TestMethodTestSuite runs MethodTestSuite.
// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
// nolint: paralleltest
func TestMethodTestSuite(t *testing.T) {
	suite.Run(t, new(MethodTestSuite))
}

// MethodTestSuite runs all methods tests for AuthzQuerier. We use
// a test suite so we can account for all functions tested on the AuthzQuerier.
// We can then assert all methods were tested and asserted for proper RBAC
// checks. This forces RBAC checks to be written for all methods.
// Additionally, the way unit tests are written allows for easily executing
// a single test for debugging.
type MethodTestSuite struct {
	suite.Suite
	// methodAccounting counts all methods called by a 'RunMethodTest'
	methodAccounting map[string]int
}

// SetupSuite sets up the suite by creating a map of all methods on AuthzQuerier
// and setting their count to 0.
func (s *MethodTestSuite) SetupSuite() {
	az := &authzquery.AuthzQuerier{}
	azt := reflect.TypeOf(az)
	s.methodAccounting = make(map[string]int)
	for i := 0; i < azt.NumMethod(); i++ {
		method := azt.Method(i)
		if _, ok := skipMethods[method.Name]; ok {
			// We can't use s.T().Skip as this will skip the entire suite.
			s.T().Logf("Skipping method %q: %s", method.Name, skipMethods[method.Name])
			continue
		}
		s.methodAccounting[method.Name] = 0
	}
}

// TearDownSuite asserts that all methods were called at least once.
func (s *MethodTestSuite) TearDownSuite() {
	s.Run("Accounting", func() {
		t := s.T()
		notCalled := []string{}
		for m, c := range s.methodAccounting {
			if c <= 0 {
				notCalled = append(notCalled, m)
			}
		}
		sort.Strings(notCalled)
		for _, m := range notCalled {
			t.Errorf("Method never called: %q", m)
		}
	})
}

// RunMethodTest runs a method test case.
// The method to be tested is inferred from the name of the test case.
func (s *MethodTestSuite) RunMethodTest(testCaseF func(t *testing.T, db database.Store) MethodCase) {
	t := s.T()
	testName := s.T().Name()
	names := strings.Split(testName, "/")
	methodName := names[len(names)-1]
	s.methodAccounting[methodName]++

	db := dbfake.New()
	rec := &coderdtest.RecordingAuthorizer{
		Wrapped: &coderdtest.FakeAuthorizer{
			AlwaysReturn: nil,
		},
	}
	az := authzquery.NewAuthzQuerier(db, rec, slog.Make())
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}
	ctx := authzquery.WithAuthorizeContext(context.Background(), actor)

	testCase := testCaseF(t, db)

	// Find the method with the name of the test.
	found := false
	azt := reflect.TypeOf(az)
MethodLoop:
	for i := 0; i < azt.NumMethod(); i++ {
		method := azt.Method(i)
		if method.Name == methodName {
			resp := reflect.ValueOf(az).Method(i).Call(append([]reflect.Value{reflect.ValueOf(ctx)}, testCase.Inputs...))
			// TODO: Should we assert the object returned is the correct one?
			for _, r := range resp {
				if r.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
					if r.IsNil() {
						// no error!
						break
					}
					err, ok := r.Interface().(error)
					if !ok {
						t.Fatal("error is not an error?!")
					}
					require.NoError(t, err, "method %q returned an error", testName)
					break
				}
			}
			found = true
			break MethodLoop
		}
	}

	require.True(t, found, "method %q does not exist", testName)

	var pairs []coderdtest.ActionObjectPair
	for _, assrt := range testCase.Assertions {
		for _, action := range assrt.Actions {
			pairs = append(pairs, coderdtest.ActionObjectPair{
				Action: action,
				Object: assrt.Object,
			})
		}
	}

	rec.AssertActor(t, actor, pairs...)
	require.NoError(t, rec.AllAsserted(), "all rbac calls must be asserted")
}

// A MethodCase contains the inputs to be provided to a single method call,
// and the assertions to be made on the RBAC checks.
type MethodCase struct {
	Inputs     []reflect.Value
	Assertions []AssertRBAC
}

// AssertRBAC contains the object and actions to be asserted.
type AssertRBAC struct {
	Object  rbac.Object
	Actions []rbac.Action
}

// methodCase is a convenience method for creating MethodCases.
//
//	methodCase(inputs(workspace, template, ...), asserts(workspace, rbac.ActionRead, template, rbac.ActionRead, ...))
//
// is equivalent to
//
//	MethodCase{
//	  Inputs: inputs(workspace, template, ...),
//	  Assertions: asserts(workspace, rbac.ActionRead, template, rbac.ActionRead, ...),
//	}
func methodCase(inputs []reflect.Value, assertions []AssertRBAC) MethodCase {
	return MethodCase{
		Inputs:     inputs,
		Assertions: assertions,
	}
}

// inputs is a convenience method for creating []reflect.Value.
//
// inputs(workspace, template, ...)
//
// is equivalent to
//
//	[]reflect.Value{
//	  reflect.ValueOf(workspace),
//	  reflect.ValueOf(template),
//	  ...
//	}
func inputs(inputs ...any) []reflect.Value {
	out := make([]reflect.Value, 0)
	for _, input := range inputs {
		input := input
		out = append(out, reflect.ValueOf(input))
	}
	return out
}

// asserts is a convenience method for creating AssertRBACs.
//
// The number of inputs must be an even number.
// asserts() will panic if this is not the case.
//
// Even-numbered inputs are the objects, and odd-numbered inputs are the actions.
// Objects must implement rbac.Objecter.
// Inputs can be a single rbac.Action, or a slice of rbac.Action.
//
//	asserts(workspace, rbac.ActionRead, template, slice(rbac.ActionRead, rbac.ActionWrite), ...)
//
// is equivalent to
//
//	[]AssertRBAC{
//	  {Object: workspace, Actions: []rbac.Action{rbac.ActionRead}},
//	  {Object: template, Actions: []rbac.Action{rbac.ActionRead, rbac.ActionWrite)}},
//	   ...
//	}
func asserts(inputs ...any) []AssertRBAC {
	if len(inputs)%2 != 0 {
		panic(fmt.Sprintf("Must be an even length number of args, found %d", len(inputs)))
	}

	out := make([]AssertRBAC, 0)
	for i := 0; i < len(inputs); i += 2 {
		obj, ok := inputs[i].(rbac.Objecter)
		if !ok {
			panic(fmt.Sprintf("object type '%T' does not implement rbac.Objecter", inputs[i]))
		}
		rbacObj := obj.RBACObject()

		var actions []rbac.Action
		actions, ok = inputs[i+1].([]rbac.Action)
		if !ok {
			action, ok := inputs[i+1].(rbac.Action)
			if !ok {
				// Could be the string type.
				actionAsString, ok := inputs[i+1].(string)
				if !ok {
					panic(fmt.Sprintf("action '%q' not a supported action", actionAsString))
				}
				action = rbac.Action(actionAsString)
			}
			actions = []rbac.Action{action}
		}

		out = append(out, AssertRBAC{
			Object:  rbacObj,
			Actions: actions,
		})
	}
	return out
}

func (s *MethodTestSuite) TestExtraMethods() {
	s.Run("GetProvisionerDaemons", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			d, err := db.InsertProvisionerDaemon(context.Background(), database.InsertProvisionerDaemonParams{
				ID: uuid.New(),
			})
			require.NoError(t, err, "insert provisioner daemon")
			return methodCase(inputs(), asserts(d, rbac.ActionRead))
		})
	})
	s.Run("GetDeploymentDAUs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(inputs(), asserts(rbac.ResourceUser.All(), rbac.ActionRead))
		})
	})
}

type emptyPreparedAuthorized struct{}

func (emptyPreparedAuthorized) Authorize(_ context.Context, _ rbac.Object) error { return nil }
func (emptyPreparedAuthorized) CompileToSQL(_ context.Context, _ regosql.ConvertConfig) (string, error) {
	return "", nil
}
