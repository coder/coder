/**
 * Admin-level permissions that control the Admin settings dropdown visibility.
 * This type is the single source of truth for which permissions classify a user
 * as an admin in the navbar. Both DeploymentDropdown and MobileMenu use it to
 * decide whether to show the Admin settings section, and NavbarView uses it to
 * route the Organizations link to the correct menu surface.
 *
 * When adding a new admin permission, add it here so all consumers stay in sync.
 */
export type AdminPermissions = {
	canViewDeployment: boolean;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewAIBridge: boolean;
	canViewAISettings: boolean;
	canViewHealth: boolean;
};

export function hasAnyAdminPermission(perms: AdminPermissions): boolean {
	return Object.values(perms).some(Boolean);
}
