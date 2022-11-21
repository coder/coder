package regosql

import "github.com/coder/coder/coderd/rbac/regosql/sqltypes"

// For the templates table
func TemplateConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		// Basic strings
		sqltypes.StringVarMatcher("organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.AlwaysFalse(sqltypes.StringVarMatcher("owner_id :: text", []string{"input", "object", "owner"})),
	)
	matcher.RegisterMatcher(
		ACLGroupMatcher(matcher, "group_acl", []string{"input", "object", "acl_group_list"}),
		ACLGroupMatcher(matcher, "user_acl", []string{"input", "object", "acl_user_list"}),
	)
	return matcher
}

// NoACLConverter should be used when the target SQL table does not contain
// group or user ACL columns.
func NoACLConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		// Basic strings
		sqltypes.StringVarMatcher("organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.StringVarMatcher("owner_id :: text", []string{"input", "object", "owner"}),
	)
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(ACLGroupMatcher(matcher, "group_acl", []string{"input", "object", "acl_group_list"})),
		sqltypes.AlwaysFalse(ACLGroupMatcher(matcher, "user_acl", []string{"input", "object", "acl_user_list"})),
	)

	return matcher
}

func DefaultVariableConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		// Basic strings
		sqltypes.StringVarMatcher("organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.StringVarMatcher("owner_id :: text", []string{"input", "object", "owner"}),
	)
	matcher.RegisterMatcher(
		ACLGroupMatcher(matcher, "group_acl", []string{"input", "object", "acl_group_list"}),
		ACLGroupMatcher(matcher, "user_acl", []string{"input", "object", "acl_user_list"}),
	)

	return matcher
}
