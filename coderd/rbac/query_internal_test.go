package rbac

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/google/uuid"
)

func TestCompileQuery(t *testing.T) {
	ctx := context.Background()
	defOrg := uuid.New()
	unuseID := uuid.New()

	user := subject{
		UserID: "me",
		Scope:  must(ScopeRole(ScopeAll)),
		Roles: []Role{
			must(RoleByName(RoleMember())),
			must(RoleByName(RoleOrgMember(defOrg))),
		},
	}
	var action Action = ActionRead
	object := ResourceWorkspace.InOrg(defOrg).WithOwner(unuseID.String())

	auth := NewAuthorizer()
	part, err := auth.Prepare(ctx, user.UserID, user.Roles, user.Scope, action, object.Type)
	require.NoError(t, err)

	result, err := Compile(part.partialQueries)
	require.NoError(t, err)

	fmt.Println("Rego: ", result.RegoString())
	fmt.Println("SQL: ", result.SQLString(SQLConfig{
		map[string]string{
			"input.object.org_owner": "organization_id",
		},
	}))
}
