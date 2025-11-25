package coderdtest

import "github.com/coder/coder/v2/coderd/rbac"

func OwnerSubject() rbac.Subject {
	return rbac.Subject{
		FriendlyName: "coderdtest-owner",
		Email:        "owner@coderd.test",
		Type:         rbac.SubjectTypeUser,
		ID:           "coderdtest-owner-id",
		Roles:        rbac.RoleIdentifiers{rbac.RoleOwner()},
		Scope:        rbac.ScopeAll,
	}.WithCachedASTValue()
}
