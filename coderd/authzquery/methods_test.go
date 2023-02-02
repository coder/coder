package authzquery_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database/databasefake"
	"github.com/stretchr/testify/suite"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

var (
	skipMethods = map[string]any{
		"InTx": struct{}{},
		"Ping": struct{}{},
	}
)

// MethodTestSuite runs all methods tests for AuthzQuerier. The reason we use
// a test suite, is so we can account for all functions tested on the AuthzQuerier.
// We can then assert all methods were tested and asserted for proper RBAC
// checks. This forces RBAC checks to be written for all methods.
// Additionally, the way unit tests are written allows for easily executing
// a single test for debugging.
type MethodTestSuite struct {
	suite.Suite
	// methodAccounting counts all methods called by a 'RunMethodTest'
	methodAccounting map[string]int
}

func (suite *MethodTestSuite) SetupSuite() {
	az := &authzquery.AuthzQuerier{}
	azt := reflect.TypeOf(az)
	suite.methodAccounting = make(map[string]int)
	for i := 0; i < azt.NumMethod(); i++ {
		method := azt.Method(i)
		if _, ok := skipMethods[method.Name]; ok {
			continue
		}
		suite.methodAccounting[method.Name] = 0
	}
}

func (suite *MethodTestSuite) TearDownSuite() {
	suite.Run("Accounting", func() {
		t := suite.T()
		for m, c := range suite.methodAccounting {
			if c <= 0 {
				t.Errorf("Method %q never called", m)
			}
		}
	})
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestMethodTestSuite(t *testing.T) {
	suite.Run(t, new(MethodTestSuite))
}

type MethodCase struct {
	Inputs     []reflect.Value
	Assertions []AssertRBAC
}

type AssertRBAC struct {
	Object  rbac.Object
	Actions []rbac.Action
}

func (suite *MethodTestSuite) RunMethodTest(testCaseF func(t *testing.T, db database.Store) MethodCase) {
	t := suite.T()
	testName := suite.T().Name()
	names := strings.Split(testName, "/")
	methodName := names[len(names)-1]
	suite.methodAccounting[methodName]++

	db := databasefake.New()
	rec := &coderdtest.RecordingAuthorizer{
		Wrapped: &coderdtest.FakeAuthorizer{},
	}
	az := authzquery.NewAuthzQuerier(db, rec)
	actor := rbac.Subject{
		ID:     uuid.NewString(),
		Roles:  rbac.RoleNames{},
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
			var _ = resp
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

func methodInputs(inputs ...any) []reflect.Value {
	out := make([]reflect.Value, 0)
	for _, input := range inputs {
		input := input
		out = append(out, reflect.ValueOf(input))
	}
	return out
}

func asserts(inputs ...any) []AssertRBAC {
	if len(inputs)%2 != 0 {
		panic(fmt.Sprintf("Must be an even length number of args, found %d", len(inputs)))
	}

	out := make([]AssertRBAC, 0)
	for i := 0; i < len(inputs); i += 2 {
		obj, ok := inputs[i].(rbac.Objecter)
		if !ok {
			panic(fmt.Sprintf("object type '%T' not a supported key", obj))
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
					panic(fmt.Sprintf("action type '%T' not a supported action", obj))
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
