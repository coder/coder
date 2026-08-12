import type { AuthorizationCheck } from "#/api/typesGenerated";
import permissionChecksData from "../../../permissions.json";

export type Permissions = {
	[k in PermissionName]: boolean;
};

type PermissionName = keyof typeof permissionChecks;

/**
 * Site-wide permission checks, loaded from the shared
 * permissions.json that is also used by the Go backend.
 */
export const permissionChecks =
	permissionChecksData as typeof permissionChecksData &
		Record<string, AuthorizationCheck>;

/**
 * Reads the workspace visibility gate. Permissions come from page metadata that
 * can predate a check, and a key that is absent reads as undefined; only an
 * explicit false hides workspace UI.
 */
export const canViewWorkspaces = (permissions: Permissions): boolean => {
	return permissions.viewWorkspaces !== false;
};

/**
 * Reads the template visibility gate. Template access is usually granted by a
 * per-template ACL, which an abstract permission check cannot evaluate, so
 * workspace visibility also allows templates.
 */
export const canViewTemplates = (permissions: Permissions): boolean => {
	return permissions.viewTemplates !== false || canViewWorkspaces(permissions);
};

export const canViewDeploymentSettings = (
	permissions: Permissions | undefined,
): permissions is Permissions => {
	return (
		permissions !== undefined &&
		(permissions.viewDeploymentConfig ||
			permissions.viewAllLicenses ||
			permissions.viewAllUsers ||
			permissions.viewAnyGroup ||
			permissions.viewNotificationTemplate ||
			permissions.viewOrganizationIDPSyncSettings ||
			permissions.viewAnyAIProvider ||
			permissions.viewAIGatewayKeys)
	);
};

/**
 * Checks if the user can view or edit members or groups for the organization
 * that produced the given OrganizationPermissions.
 */
export const canViewAnyOrganization = (
	permissions: Permissions | undefined,
): permissions is Permissions => {
	return (
		permissions !== undefined &&
		(permissions.editAnyGroups ||
			permissions.assignAnyRoles ||
			permissions.viewAnyIdpSyncSettings ||
			permissions.editAnySettings)
	);
};
