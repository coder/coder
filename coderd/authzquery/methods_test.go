package authzquery_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"golang.org/x/xerrors"

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
		"InTx": "Not relevant",
		"Ping": "Not relevant",
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

	// Individual state for each unit test.
	// State used by developer
	DB database.Store
	// State set by setup
	ctx   context.Context
	az    *authzquery.AuthzQuerier
	rec   *coderdtest.RecordingAuthorizer
	authz *coderdtest.FakeAuthorizer
	actor rbac.Subject
	// State set by developer
	testCase MethodCase
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

func (s *MethodTestSuite) clear() {
	s.DB = nil
	s.ctx = nil
	s.az = nil
	s.rec = nil
	s.actor = rbac.Subject{}
	s.testCase = MethodCase{}
	s.authz = nil
}

func (s *MethodTestSuite) SetupTest() {
	s.clear()

	s.DB = dbfake.New()
	s.authz = &coderdtest.FakeAuthorizer{
		AlwaysReturn: nil,
	}
	s.rec = &coderdtest.RecordingAuthorizer{
		Wrapped: s.authz,
	}
	s.az = authzquery.New(s.DB, s.rec, slog.Make())
	s.actor = rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}
	s.ctx = authzquery.WithAuthorizeContext(context.Background(), s.actor)
}

func (s *MethodTestSuite) TearDownTest() {
	var (
		t              = s.T()
		az             = s.az
		testCase       = s.testCase
		fakeAuthorizer = s.authz
		ctx            = s.ctx
		rec            = s.rec
	)

	require.NotEqualf(t, "", testCase.MethodName, "Method name must be set")

	methodName := testCase.MethodName
	s.methodAccounting[methodName]++

	// Find the method with the name of the test.
	found := false
	azt := reflect.TypeOf(az)
MethodLoop:
	for i := 0; i < azt.NumMethod(); i++ {
		method := azt.Method(i)
		if method.Name == methodName {
			if len(testCase.Assertions) > 0 {
				fakeAuthorizer.AlwaysReturn = xerrors.New("Always fail authz")
				// If we have assertions, that means the method should FAIL
				// if RBAC will disallow the request. The returned error should
				// be expected to be a NotAuthorizedError.
				erroredResp := reflect.ValueOf(az).Method(i).Call(append([]reflect.Value{reflect.ValueOf(ctx)}, testCase.Inputs...))
				_, err := splitResp(t, erroredResp)
				// This is unfortunate, but if we are using `Filter` the error returned will be nil. So filter out
				// any case where the error is nil and the response is an empty slice.
				if err != nil || !hasEmptySliceResponse(erroredResp) {
					require.Errorf(t, err, "method %q should an error with disallow authz", methodName)
					require.ErrorIsf(t, err, sql.ErrNoRows, "error should match sql.ErrNoRows")
					require.ErrorAs(t, err, &authzquery.NotAuthorizedError{}, "error should be NotAuthorizedError")
				}
				// Set things back to normal.
				fakeAuthorizer.AlwaysReturn = nil
				rec.Reset()
			}

			resp := reflect.ValueOf(az).Method(i).Call(append([]reflect.Value{reflect.ValueOf(ctx)}, testCase.Inputs...))

			outputs, err := splitResp(t, resp)
			require.NoError(t, err, "method %q returned an error", t.Name())

			// Some tests may not care about the outputs, so we only assert if
			// they are provided.
			if testCase.ExpectedOutputs != nil {
				// Assert the required outputs
				require.Equal(t, len(testCase.ExpectedOutputs), len(outputs), "method %q returned unexpected number of outputs", methodName)
				for i := range outputs {
					a, b := testCase.ExpectedOutputs[i].Interface(), outputs[i].Interface()
					if reflect.TypeOf(a).Kind() == reflect.Slice || reflect.TypeOf(a).Kind() == reflect.Array {
						// Order does not matter
						require.ElementsMatch(t, a, b, "method %q returned unexpected output %d", methodName, i)
					} else {
						require.Equal(t, a, b, "method %q returned unexpected output %d", methodName, i)
					}
				}
			}

			found = true
			break MethodLoop
		}
	}

	require.True(t, found, "method %q does not exist", methodName)

	var pairs []coderdtest.ActionObjectPair
	for _, assrt := range testCase.Assertions {
		for _, action := range assrt.Actions {
			pairs = append(pairs, coderdtest.ActionObjectPair{
				Action: action,
				Object: assrt.Object,
			})
		}
	}

	rec.AssertActor(t, s.actor, pairs...)
	require.NoError(t, rec.AllAsserted(), "all rbac calls must be asserted")
	s.clear()
}

func (s *MethodTestSuite) Asserts(v ...any) *MethodTestSuite {
	s.testCase.MethodName = methodName(s.T())
	s.testCase = s.testCase.Asserts(v...)
	return s
}

func (s *MethodTestSuite) Args(v ...any) *MethodTestSuite {
	s.testCase = s.testCase.Args(v...)
	return s
}

func (s *MethodTestSuite) Returns(v ...any) *MethodTestSuite {
	s.testCase = s.testCase.Returns(v...)
	return s
}

// RunMethodTest runs a method test case.
// The method to be tested is inferred from the name of the test case.
// Deprecated
func (s *MethodTestSuite) RunMethodTest(testCaseF func(t *testing.T, db database.Store) MethodCase) {
	t := s.T()
	testName := s.T().Name()
	names := strings.Split(testName, "/")
	methodName := names[len(names)-1]
	s.methodAccounting[methodName]++

	db := dbfake.New()
	fakeAuthorizer := &coderdtest.FakeAuthorizer{
		AlwaysReturn: nil,
	}
	rec := &coderdtest.RecordingAuthorizer{
		Wrapped: fakeAuthorizer,
	}
	az := authzquery.New(db, rec, slog.Make())
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
			if len(testCase.Assertions) > 0 {
				fakeAuthorizer.AlwaysReturn = xerrors.New("Always fail authz")
				// If we have assertions, that means the method should FAIL
				// if RBAC will disallow the request. The returned error should
				// be expected to be a NotAuthorizedError.
				erroredResp := reflect.ValueOf(az).Method(i).Call(append([]reflect.Value{reflect.ValueOf(ctx)}, testCase.Inputs...))
				_, err := splitResp(t, erroredResp)
				// This is unfortunate, but if we are using `Filter` the error returned will be nil. So filter out
				// any case where the error is nil and the response is an empty slice.
				if err != nil || !hasEmptySliceResponse(erroredResp) {
					require.Errorf(t, err, "method %q should an error with disallow authz", testName)
					require.ErrorIsf(t, err, sql.ErrNoRows, "error should match sql.ErrNoRows")
					require.ErrorAs(t, err, &authzquery.NotAuthorizedError{}, "error should be NotAuthorizedError")
				}
				// Set things back to normal.
				fakeAuthorizer.AlwaysReturn = nil
				rec.Reset()
			}

			resp := reflect.ValueOf(az).Method(i).Call(append([]reflect.Value{reflect.ValueOf(ctx)}, testCase.Inputs...))

			outputs, err := splitResp(t, resp)
			require.NoError(t, err, "method %q returned an error", testName)

			// Some tests may not care about the outputs, so we only assert if
			// they are provided.
			if testCase.ExpectedOutputs != nil {
				// Assert the required outputs
				require.Equal(t, len(testCase.ExpectedOutputs), len(outputs), "method %q returned unexpected number of outputs", testName)
				for i := range outputs {
					a, b := testCase.ExpectedOutputs[i].Interface(), outputs[i].Interface()
					if reflect.TypeOf(a).Kind() == reflect.Slice || reflect.TypeOf(a).Kind() == reflect.Array {
						// Order does not matter
						require.ElementsMatch(t, a, b, "method %q returned unexpected output %d", testName, i)
					} else {
						require.Equal(t, a, b, "method %q returned unexpected output %d", testName, i)
					}
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

func hasEmptySliceResponse(values []reflect.Value) bool {
	for _, r := range values {
		if r.Kind() == reflect.Slice || r.Kind() == reflect.Array {
			if r.Len() == 0 {
				return true
			}
		}
	}
	return false
}

func splitResp(t *testing.T, values []reflect.Value) ([]reflect.Value, error) {
	outputs := []reflect.Value{}
	for _, r := range values {
		if r.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			if r.IsNil() {
				// Error is found, but it's nil!
				return outputs, nil
			}
			err, ok := r.Interface().(error)
			if !ok {
				t.Fatal("error is not an error?!")
			}
			return outputs, err
		}
		outputs = append(outputs, r)
	} //nolint: unreachable
	t.Fatal("no expected error value found in responses (error can be nil)")
	return nil, nil // unreachable, required to compile
}

// A MethodCase contains the inputs to be provided to a single method call,
// and the assertions to be made on the RBAC checks.
type MethodCase struct {
	// MethodName is the name of the method to be called on the AuthzQuerier.
	MethodName string
	Inputs     []reflect.Value
	Assertions []AssertRBAC
	// Output is optional. Can assert non-error return values.
	ExpectedOutputs []reflect.Value
}

func (m MethodCase) Asserts(pairs ...any) MethodCase {
	m.Assertions = asserts(pairs...)
	return m
}

func (m MethodCase) Args(args ...any) MethodCase {
	m.Inputs = values(args...)
	return m
}

func (m MethodCase) Returns(rets ...any) MethodCase {
	m.ExpectedOutputs = values(rets...)
	return m
}

// AssertRBAC contains the object and actions to be asserted.
type AssertRBAC struct {
	Object  rbac.Object
	Actions []rbac.Action
}

// methodCase is a convenience method for creating MethodCases.
//
//	methodCase(values(workspace, template, ...), asserts(workspace, rbac.ActionRead, template, rbac.ActionRead, ...))
//
// is equivalent to
//
//	MethodCase{
//	  Inputs: values(workspace, template, ...),
//	  Assertions: asserts(workspace, rbac.ActionRead, template, rbac.ActionRead, ...),
//	}
func methodCase(ins []reflect.Value, assertions []AssertRBAC, outs []reflect.Value) MethodCase {
	return MethodCase{
		Inputs:          ins,
		Assertions:      assertions,
		ExpectedOutputs: outs,
	}
}

// values is a convenience method for creating []reflect.Value.
//
// values(workspace, template, ...)
//
// is equivalent to
//
//	[]reflect.Value{
//	  reflect.ValueOf(workspace),
//	  reflect.ValueOf(template),
//	  ...
//	}
func values(ins ...any) []reflect.Value {
	out := make([]reflect.Value, 0)
	for _, input := range ins {
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

func methodName(t *testing.T) string {
	testName := t.Name()
	names := strings.Split(testName, "/")
	methodName := names[len(names)-1]
	return methodName
}

func (s *MethodTestSuite) TestExtraMethods() {
	s.Run("GetProvisionerDaemons", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			d, err := db.InsertProvisionerDaemon(context.Background(), database.InsertProvisionerDaemonParams{
				ID: uuid.New(),
			})
			require.NoError(t, err, "insert provisioner daemon")
			return methodCase(values(), asserts(d, rbac.ActionRead), nil)
		})
	})
	s.Run("GetDeploymentDAUs", func() {
		s.RunMethodTest(func(t *testing.T, db database.Store) MethodCase {
			return methodCase(values(), asserts(rbac.ResourceUser.All(), rbac.ActionRead), nil)
		})
	})
}

type emptyPreparedAuthorized struct{}

func (emptyPreparedAuthorized) Authorize(_ context.Context, _ rbac.Object) error { return nil }
func (emptyPreparedAuthorized) CompileToSQL(_ context.Context, _ regosql.ConvertConfig) (string, error) {
	return "", nil
}
