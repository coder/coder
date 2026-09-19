package regosql

import "github.com/coder/coder/v2/coderd/rbac/regosql/sqltypes"

func resourceIDMatcher() sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher("id :: text", []string{"input", "object", "id"})
}

func chatResourceIDMatcher() sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher("chats_expanded.id :: text", []string{"input", "object", "id"})
}

func organizationOwnerMatcher() sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher("organization_id :: text", []string{"input", "object", "org_owner"})
}

func userOwnerMatcher() sqltypes.VariableMatcher {
	return sqltypes.StringVarMatcher("owner_id :: text", []string{"input", "object", "owner"})
}

func groupACLMatcher(m sqltypes.VariableMatcher) ACLMappingVar {
	return ACLMappingMatcher(m, "group_acl", []string{"input", "object", "acl_group_list"})
}

func userACLMatcher(m sqltypes.VariableMatcher) ACLMappingVar {
	return ACLMappingMatcher(m, "user_acl", []string{"input", "object", "acl_user_list"})
}

func TemplateConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		sqltypes.StringVarMatcher("t.organization_id :: text", []string{"input", "object", "org_owner"}),
		// Templates have no user owner, only owner by an organization.
		sqltypes.AlwaysFalse(userOwnerMatcher()),
	)
	matcher.RegisterMatcher(
		groupACLMatcher(matcher),
		userACLMatcher(matcher),
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
		ACLMappingMatcher(matcher, "workspaces.group_acl", []string{"input", "object", "acl_group_list"}).UsingSubfield("permissions"),
		ACLMappingMatcher(matcher, "workspaces.user_acl", []string{"input", "object", "acl_user_list"}).UsingSubfield("permissions"),
	)

	return matcher
}

func ChatConverter() *sqltypes.VariableConverter {
	matcher := chatBaseConverter()
	matcher.RegisterMatcher(
		ACLMappingMatcher(matcher, "chats_expanded.group_acl", []string{"input", "object", "acl_group_list"}).UsingSubfield("permissions"),
		ACLMappingMatcher(matcher, "chats_expanded.user_acl", []string{"input", "object", "acl_user_list"}).UsingSubfield("permissions"),
	)

	return matcher
}

func ChatNoACLConverter() *sqltypes.VariableConverter {
	matcher := chatBaseConverter()
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
	)

	return matcher
}

func MCPServerConfigConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		sqltypes.StringVarMatcher("mcp_server_configs.organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.AlwaysFalse(userOwnerMatcher()),
	)
	matcher.RegisterMatcher(
		ACLMappingMatcher(matcher, "mcp_server_configs.group_acl", []string{"input", "object", "acl_group_list"}).UsingSubfield("permissions"),
		ACLMappingMatcher(matcher, "mcp_server_configs.user_acl", []string{"input", "object", "acl_user_list"}).UsingSubfield("permissions"),
	)
	return matcher
}

// ChatProjectConverter qualifies chat project columns for SQL filters built
// from the RBAC policy, mirroring WorkspaceConverter.
func ChatProjectConverter() *sqltypes.VariableConverter {
	matcher := chatProjectBaseConverter()
	matcher.RegisterMatcher(
		ACLMappingMatcher(matcher, "chat_projects.group_acl", []string{"input", "object", "acl_group_list"}).UsingSubfield("permissions"),
		ACLMappingMatcher(matcher, "chat_projects.user_acl", []string{"input", "object", "acl_user_list"}).UsingSubfield("permissions"),
	)
	return matcher
}

// ChatProjectNoACLConverter ignores stored project ACLs so a disabled
// sharing kill switch also hides shared projects from list queries.
func ChatProjectNoACLConverter() *sqltypes.VariableConverter {
	matcher := chatProjectBaseConverter()
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
	)
	return matcher
}

func chatProjectBaseConverter() *sqltypes.VariableConverter {
	return sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		sqltypes.StringVarMatcher("chat_projects.organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.StringVarMatcher("chat_projects.created_by :: text", []string{"input", "object", "owner"}),
	)
}

func chatBaseConverter() *sqltypes.VariableConverter {
	return sqltypes.NewVariableConverter().RegisterMatcher(
		chatResourceIDMatcher(),
		sqltypes.StringVarMatcher("chats_expanded.organization_id :: text", []string{"input", "object", "org_owner"}),
		userOwnerMatcher(),
	)
}

// ChatModelConfigConverter qualifies columns against the cmc alias used by
// GetChatModelConfigs. Chat model configs have no user owner,
// only an organization owner.
func ChatModelConfigConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		sqltypes.StringVarMatcher("cmc.id :: text", []string{"input", "object", "id"}),
		sqltypes.StringVarMatcher("cmc.organization_id :: text", []string{"input", "object", "org_owner"}),
		sqltypes.AlwaysFalse(userOwnerMatcher()),
	)
	matcher.RegisterMatcher(
		ACLMappingMatcher(matcher, "cmc.group_acl", []string{"input", "object", "acl_group_list"}).UsingSubfield("permissions"),
		ACLMappingMatcher(matcher, "cmc.user_acl", []string{"input", "object", "acl_user_list"}).UsingSubfield("permissions"),
	)

	return matcher
}

func AuditLogConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		sqltypes.UUIDVarMatcher("audit_logs.organization_id", []string{"input", "object", "org_owner"}),
		// Audit logs have no user owner, only owner by an organization.
		sqltypes.AlwaysFalse(userOwnerMatcher()),
	)
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
	)
	return matcher
}

func ConnectionLogConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		sqltypes.UUIDVarMatcher("connection_logs.organization_id", []string{"input", "object", "org_owner"}),
		// Connection logs have no user owner, only owner by an organization.
		sqltypes.AlwaysFalse(userOwnerMatcher()),
	)
	matcher.RegisterMatcher(
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
	)
	return matcher
}

func AIBridgeInterceptionConverter() *sqltypes.VariableConverter {
	matcher := sqltypes.NewVariableConverter().RegisterMatcher(
		resourceIDMatcher(),
		// AI Bridge interceptions are not tied to any organization.
		sqltypes.StringVarMatcher("''", []string{"input", "object", "org_owner"}),
		sqltypes.StringVarMatcher("initiator_id :: text", []string{"input", "object", "owner"}),
	)
	matcher.RegisterMatcher(
		// No ACLs on the aibridge interception type
		sqltypes.AlwaysFalse(groupACLMatcher(matcher)),
		sqltypes.AlwaysFalse(userACLMatcher(matcher)),
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
