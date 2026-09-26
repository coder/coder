package codersdk

// Ideally these roles would be generated from the rbac/roles.go package.
const (
	RoleOwner         string = "owner"
	RoleMember        string = "member"
	RoleTemplateAdmin string = "template-admin"
	RoleUserAdmin     string = "user-admin"
	RoleAuditor       string = "auditor"
	// RoleAgentsAccess is the organization role that grants Coder Agents
	// chat access. New organizations include it in their default member
	// roles.
	RoleAgentsAccess string = "agents-access"

	RoleOrganizationAdmin                string = "organization-admin"
	RoleOrganizationMember               string = "organization-member"
	RoleOrganizationAuditor              string = "organization-auditor"
	RoleOrganizationTemplateAdmin        string = "organization-template-admin"
	RoleOrganizationUserAdmin            string = "organization-user-admin"
	RoleOrganizationWorkspaceCreationBan string = "organization-workspace-creation-ban"
	RoleOrganizationWorkspaceAccess      string = "organization-workspace-access"
)
