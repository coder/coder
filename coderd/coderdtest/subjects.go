package coderdtest

import (
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
)

func MemberSubject(userID, orgID uuid.UUID) rbac.Subject {
	memberRole, err := rbac.RoleByName(rbac.RoleMember())
	if err != nil {
		panic(err)
	}
	orgMember, err := rolestore.TestingGetSystemRole(
		rbac.RoleOrgMember(),
		orgID,
		rbac.OrgSettings{ShareableWorkspaceOwners: rbac.ShareableWorkspaceOwnersNone},
	)
	if err != nil {
		panic(err)
	}
	return rbac.Subject{
		FriendlyName: "coderdtest-member",
		Email:        "member@coderd.test",
		Type:         rbac.SubjectTypeUser,
		ID:           userID.String(),
		Roles:        rbac.Roles{memberRole, orgMember},
		Scope:        rbac.ScopeAll,
	}.WithCachedASTValue()
}
