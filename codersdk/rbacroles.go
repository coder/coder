package codersdk

// Ideally these roles would be generated from the rbac/roles.go package.
const (
	RoleOwner         string = "owner"
	RoleMember        string = "member"
	RoleTemplateAdmin string = "template-admin"
	RoleUserAdmin     string = "user-admin"
	RoleAuditor       string = "auditor"
	// Deprecated: the agents-access role was removed. Coder Agents chat
	// access is part of the organization-member permission floor, and
	// servers without this built-in role reject assigning it.
	RoleAgentsAccess string = "agents-access"

	RoleOrganizationAdmin                string = "organization-admin"
	RoleOrganizationMember               string = "organization-member"
	RoleOrganizationAuditor              string = "organization-auditor"
	RoleOrganizationTemplateAdmin        string = "organization-template-admin"
	RoleOrganizationUserAdmin            string = "organization-user-admin"
	RoleOrganizationWorkspaceCreationBan string = "organization-workspace-creation-ban"
	RoleOrganizationWorkspaceAccess      string = "organization-workspace-access"
)
