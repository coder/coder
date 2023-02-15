package regosql

import "github.com/coder/coder/coderd/rbac/regosql/sqltypes"

func resourceIDMatcher(prefix string) sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher(prefix+"id :: text", []string{"input", "object", "id"})
}

func organizationOwnerMatcher(prefix string) sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher(prefix+"organization_id :: text", []string{"input", "object", "org_owner"})
}

func userOwnerMatcher(prefix string) sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher(prefix+"owner_id :: text", []string{"input", "object", "owner"})
}

func groupACLMatcher(prefix string, m sqltypes.VariableMatcher) sqltypes.VariableMatcher {
	return ACLGroupMatcher(m, prefix+"group_acl", []string{"input", "object", "acl_group_list"})
}

func userACLMatcher(prefix string, m sqltypes.VariableMatcher) sqltypes.VariableMatcher {
	return ACLGroupMatcher(m, prefix+"user_acl", []string{"input", "object", "acl_user_list"})
}

func TemplateConverter() *sqltypes.VariableConverter {
	prefix := ""
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(prefix),
		organizationOwnerMatcher(prefix),
		// Templates have no user owner, only owner by an organization.
		sqltypes.AlwaysFalse(userOwnerMatcher(prefix)),
	)
	matcher.RegisterMatcher(
		groupACLMatcher(prefix, matcher),
		userACLMatcher(prefix, matcher),
	)
	return matcher
}

// NoACLConverter should be used when the target SQL table does not contain
// group or user ACL columns.
//
// prefix allows namespacing the generated sql columns.
func NoACLConverter(prefix string) *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(prefix),
		organizationOwnerMatcher(prefix),
		userOwnerMatcher(prefix),
	)
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(groupACLMatcher(prefix, matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(prefix, matcher)),
	)

	return matcher
}

func DefaultVariableConverter(prefix string) *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(prefix),
		organizationOwnerMatcher(prefix),
		userOwnerMatcher(prefix),
	)
	matcher.RegisterMatcher(
		groupACLMatcher(prefix, matcher),
		userACLMatcher(prefix, matcher),
	)

	return matcher
}
