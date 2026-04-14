/**
 * Human-readable descriptions for built-in roles. Used by role editing
 * UI in both the UsersPage table and the EditUserPage form.
 */
export const roleDescriptions: Record<string, string> = {
	owner:
		"Owner can manage all resources, including users, groups, templates, and workspaces.",
	"user-admin": "User admin can manage all users and groups.",
	"template-admin": "Template admin can manage all templates and workspaces.",
	auditor: "Auditor can access the audit logs.",
	"agents-access": "Coder Agents User allows creating and using Coder Agents.",
	member:
		"Everybody is a member. This is a shared and default role for all users.",
};
