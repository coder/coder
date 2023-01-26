package authzquery_test

import (
	"context"
	"reflect"
	"testing"

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
