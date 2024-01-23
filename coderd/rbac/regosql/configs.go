package regosql

import "github.com/coder/coder/v2/coderd/rbac/regosql/sqltypes"

func resourceIDMatcher() sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher("id :: text", []string{"input", "object", "id"})
}

func organizationOwnerMatcher() sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher("organization_id :: text", []string{"input", "object", "org_owner"})
}

func userOwnerMatcher() sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher("owner_id :: text", []string{"input", "object", "owner"})
}

func groupACLMatcher(m sqltypes.VariableMatcher) sqltypes.VariableMatcher {
	return ACLGroupMatcher(m, "group_acl", []string{"input", "object", "acl_group_list"})
}

func userACLMatcher(m sqltypes.VariableMatcher) sqltypes.VariableMatcher {
	return ACLGroupMatcher(m, "user_acl", []string{"input", "object", "acl_user_list"})
}

func TemplateConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		organizationOwnerMatcher(),
		// Templates have no user owner, only owner by an organization.
		sqltypes.AlwaysFalse(userOwnerMatcher()),
	)
	matcher.RegisterMatcher(
		groupACLMatcher(matcher),
		userACLMatcher(matcher),
	)
	return matcher
}

func UserConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		// Users are never owned by an organization, so always return the empty string
		// for the org owner.
		sqltypes.StringVarMatcher("''", []string{"input", "object", "org_owner"}),
		// Users are always owned by themselves.
		sqltypes.StringVarMatcher("id :: text", []string{"input", "object", "owner"}),
	)
	matcher.RegisterMatcher(
		// No ACLs on the user type
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
	)
	return matcher
}

func WorkspaceConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		sqltypes.StringVarMatcher("workspaces.organization_id :: text", []string{"input", "object", "org_owner"}),
		userOwnerMatcher(),
	)
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
	)

	return matcher
}

// NoACLConverter should be used when the target SQL table does not contain
// group or user ACL columns.
func NoACLConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		organizationOwnerMatcher(),
		userOwnerMatcher(),
	)
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
	)

	return matcher
}

func DefaultVariableConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		organizationOwnerMatcher(),
		userOwnerMatcher(),
	)
	matcher.RegisterMatcher(
		groupACLMatcher(matcher),
		userACLMatcher(matcher),
	)

	return matcher
}
