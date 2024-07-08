package codersdk

// Ideally this roles would be generated from the rbac/roles.go package.
const (
	RoleOwner         string = "owner"
	RoleMember        string = "member"
	RoleTemplateAdmin string = "template-admin"
	RoleUserAdmin     string = "user-admin"
	RoleAuditor       string = "auditor"

	RoleOrganizationAdmin  string = "organization-admin"
	RoleOrganizationMember string = "organization-member"
)
