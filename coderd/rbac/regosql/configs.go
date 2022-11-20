package regosql

import "github.com/coder/coder/coderd/rbac/regosql/sqltypes"

// NoACLConverter should be used when the target SQL table does not contain
// group or user ACL columns.
func NoACLConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		// Basic strings
		sqltypes.StringVarMatcher("organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.StringVarMatcher("owner_id :: text", []string{"input", "object", "owner"}),
	)
	aclGroups := aclGroupMatchers(matcher)
	for i := range aclGroups {
		// Disable acl groups
		matcher.RegisterMatcher(aclGroups[i].Disable())
	}

	return matcher
}

func DefaultVariableConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		// Basic strings
		sqltypes.StringVarMatcher("organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.StringVarMatcher("owner_id :: text", []string{"input", "object", "owner"}),
	)
	aclGroups := aclGroupMatchers(matcher)
	for i := range aclGroups {
		matcher.RegisterMatcher(aclGroups[i])
	}

	return matcher
}

func aclGroupMatchers(c *sqltypes.VariableConverter) []ACLGroupVar {
	return []ACLGroupVar{
		ACLGroupMatcher(c, "group_acl", []string{"input", "object", "acl_group_list"}),
		ACLGroupMatcher(c, "user_acl", []string{"input", "object", "acl_user_list"}),
	}
}
