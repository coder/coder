package dbauthz_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/regosql"
	"github.com/coder/coder/v2/coderd/util/slice"
)

var skipMethods = map[string]string{
	"InTx":     "Not relevant",
	"Ping":     "Not relevant",
	"Wrappers": "Not relevant",
}

// TestMethodTestSuite runs MethodTestSuite.
// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
// nolint: paralleltest
func TestMethodTestSuite(t *testing.T) {
	suite.Run(t, new(MethodTestSuite))
}

// MethodTestSuite runs all methods tests for querier. We use
// a test suite so we can account for all functions tested on the querier.
// We can then assert all methods were tested and asserted for proper RBAC
// checks. This forces RBAC checks to be written for all methods.
// Additionally, the way unit tests are written allows for easily executing
// a single test for debugging.
type MethodTestSuite struct {
	suite.Suite
	// methodAccounting counts all methods called by a 'RunMethodTest'
	methodAccounting map[string]int
}

// SetupSuite sets up the suite by creating a map of all methods on querier
// and setting their count to 0.
func (s *MethodTestSuite) SetupSuite() {
	ctrl := gomock.NewController(s.T())
	mockStore := dbmock.NewMockStore(ctrl)
	// We intentionally set no expectations apart from this.
	mockStore.EXPECT().Wrappers().Return([]string{}).AnyTimes()
	az := dbauthz.New(mockStore, nil, slog.Make(), coderdtest.AccessControlStorePointer())
	// Take the underlying type of the interface.
	azt := reflect.TypeOf(az).Elem()
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

// Subtest is a helper function that returns a function that can be passed to
// s.Run(). This function will run the test case for the method that is being
// tested. The check parameter is used to assert the results of the method.
// If the caller does not use the `check` parameter, the test will fail.
func (s *MethodTestSuite) Subtest(testCaseF func(db database.Store, check *expects)) func() {
	return func() {
		t := s.T()
		testName := s.T().Name()
		names := strings.Split(testName, "/")
		methodName := names[len(names)-1]
		s.methodAccounting[methodName]++

		db := dbmem.New()
		fakeAuthorizer := &coderdtest.FakeAuthorizer{
			AlwaysReturn: nil,
		}
		rec := &coderdtest.RecordingAuthorizer{
			Wrapped: fakeAuthorizer,
		}
		az := dbauthz.New(db, rec, slog.Make(), coderdtest.AccessControlStorePointer())
		actor := rbac.Subject{
			ID:     uuid.NewString(),
			Roles:  rbac.RoleNames{rbac.RoleOwner()},
			Groups: []string{},
			Scope:  rbac.ScopeAll,
		}
		ctx := dbauthz.As(context.Background(), actor)

		var testCase expects
		testCaseF(db, &testCase)
		// Check the developer added assertions. If there are no assertions,
		// an empty list should be passed.
		s.Require().False(testCase.assertions == nil, "rbac assertions not set, use the 'check' parameter")

		// Find the method with the name of the test.
		var callMethod func(ctx context.Context) ([]reflect.Value, error)
		azt := reflect.TypeOf(az)
	MethodLoop:
		for i := 0; i < azt.NumMethod(); i++ {
			method := azt.Method(i)
			if method.Name == methodName {
				methodF := reflect.ValueOf(az).Method(i)

				callMethod = func(ctx context.Context) ([]reflect.Value, error) {
					resp := methodF.Call(append([]reflect.Value{reflect.ValueOf(ctx)}, testCase.inputs...))
					return splitResp(t, resp)
				}
				break MethodLoop
			}
		}

		require.NotNil(t, callMethod, "method %q does not exist", methodName)

		if len(testCase.assertions) > 0 {
			// Only run these tests if we know the underlying call makes
			// rbac assertions.
			s.NotAuthorizedErrorTest(ctx, fakeAuthorizer, callMethod)
		}

		if len(testCase.assertions) > 0 ||
			slice.Contains([]string{
				"GetAuthorizedWorkspaces",
				"GetAuthorizedTemplates",
			}, methodName) {
			// Some methods do not make RBAC assertions because they use
			// SQL. We still want to test that they return an error if the
			// actor is not set.
			s.NoActorErrorTest(callMethod)
		}

		// Always run
		s.Run("Success", func() {
			rec.Reset()
			fakeAuthorizer.AlwaysReturn = nil

			outputs, err := callMethod(ctx)
			s.NoError(err, "method %q returned an error", methodName)

			// Some tests may not care about the outputs, so we only assert if
			// they are provided.
			if testCase.outputs != nil {
				// Assert the required outputs
				s.Equal(len(testCase.outputs), len(outputs), "method %q returned unexpected number of outputs", methodName)
				for i := range outputs {
					a, b := testCase.outputs[i].Interface(), outputs[i].Interface()
					if reflect.TypeOf(a).Kind() == reflect.Slice || reflect.TypeOf(a).Kind() == reflect.Array {
						// Order does not matter
						s.ElementsMatch(a, b, "method %q returned unexpected output %d", methodName, i)
					} else {
						s.Equal(a, b, "method %q returned unexpected output %d", methodName, i)
					}
				}
			}

			var pairs []coderdtest.ActionObjectPair
			for _, assrt := range testCase.assertions {
				for _, action := range assrt.Actions {
					pairs = append(pairs, coderdtest.ActionObjectPair{
						Action: action,
						Object: assrt.Object,
					})
				}
			}

			rec.AssertActor(s.T(), actor, pairs...)
			s.NoError(rec.AllAsserted(), "all rbac calls must be asserted")
		})
	}
}

func (s *MethodTestSuite) NoActorErrorTest(callMethod func(ctx context.Context) ([]reflect.Value, error)) {
	s.Run("AsRemoveActor", func() {
		// Call without any actor
		_, err := callMethod(context.Background())
		s.ErrorIs(err, dbauthz.NoActorError, "method should return NoActorError error when no actor is provided")
	})
}

// NotAuthorizedErrorTest runs the given method with an authorizer that will fail authz.
// Asserts that the error returned is a NotAuthorizedError.
func (s *MethodTestSuite) NotAuthorizedErrorTest(ctx context.Context, az *coderdtest.FakeAuthorizer, callMethod func(ctx context.Context) ([]reflect.Value, error)) {
	s.Run("NotAuthorized", func() {
		az.AlwaysReturn = rbac.ForbiddenWithInternal(xerrors.New("Always fail authz"), rbac.Subject{}, "", rbac.Object{}, nil)

		// If we have assertions, that means the method should FAIL
		// if RBAC will disallow the request. The returned error should
		// be expected to be a NotAuthorizedError.
		resp, err := callMethod(ctx)

		// This is unfortunate, but if we are using `Filter` the error returned will be nil. So filter out
		// any case where the error is nil and the response is an empty slice.
		if err != nil || !hasEmptySliceResponse(resp) {
			s.ErrorContainsf(err, "unauthorized", "error string should have a good message")
			s.Errorf(err, "method should an error with disallow authz")
			s.ErrorAs(err, &dbauthz.NotAuthorizedError{}, "error should be NotAuthorizedError")
		}
	})

	s.Run("Canceled", func() {
		// Pass in a canceled context
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		az.AlwaysReturn = rbac.ForbiddenWithInternal(&topdown.Error{Code: topdown.CancelErr},
			rbac.Subject{}, "", rbac.Object{}, nil)

		// If we have assertions, that means the method should FAIL
		// if RBAC will disallow the request. The returned error should
		// be expected to be a NotAuthorizedError.
		resp, err := callMethod(ctx)

		// This is unfortunate, but if we are using `Filter` the error returned will be nil. So filter out
		// any case where the error is nil and the response is an empty slice.
		if err != nil || !hasEmptySliceResponse(resp) {
			s.Errorf(err, "method should an error with cancellation")
			s.ErrorIsf(err, context.Canceled, "error should match context.Cancelled")
		}
	})
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
	}
	t.Fatal("no expected error value found in responses (error can be nil)")
	return nil, nil // unreachable, required to compile
}

// expects is used to build a test case for a method.
// It includes the expected inputs, rbac assertions, and expected outputs.
type expects struct {
	inputs     []reflect.Value
	assertions []AssertRBAC
	// outputs is optional. Can assert non-error return values.
	outputs []reflect.Value
}

// Asserts is required. Asserts the RBAC authorize calls that should be made.
// If no RBAC calls are expected, pass an empty list: 'm.Asserts()'
func (m *expects) Asserts(pairs ...any) *expects {
	m.assertions = asserts(pairs...)
	return m
}

// Args is required. The arguments to be provided to the method.
// If there are no arguments, pass an empty list: 'm.Args()'
// The first context argument should not be included, as the test suite
// will provide it.
func (m *expects) Args(args ...any) *expects {
	m.inputs = values(args...)
	return m
}

// Returns is optional. If it is never called, it will not be asserted.
func (m *expects) Returns(rets ...any) *expects {
	m.outputs = values(rets...)
	return m
}

// AssertRBAC contains the object and actions to be asserted.
type AssertRBAC struct {
	Object  rbac.Object
	Actions []rbac.Action
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

type emptyPreparedAuthorized struct{}

func (emptyPreparedAuthorized) Authorize(_ context.Context, _ rbac.Object) error { return nil }
func (emptyPreparedAuthorized) CompileToSQL(_ context.Context, _ regosql.ConvertConfig) (string, error) {
	return "", nil
}
