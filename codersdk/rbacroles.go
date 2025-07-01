package codersdk

// Ideally this roles would be generated from the rbac/roles.go package.
const (
	RoleOwner         string = "owner"
	RoleMember        string = "member"
	RoleTemplateAdmin string = "template-admin"
	RoleUserAdmin     string = "user-admin"
	RoleAuditor       string = "auditor"

	RoleOrganizationAdmin                string = "organization-admin"
	RoleOrganizationMember               string = "organization-member"
	RoleOrganizationAuditor              string = "organization-auditor"
	RoleOrganizationTemplateAdmin        string = "organization-template-admin"
	RoleOrganizationUserAdmin            string = "organization-user-admin"
	RoleOrganizationWorkspaceCreationBan string = "organization-workspace-creation-ban"
)
