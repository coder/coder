import type { AuthorizationCheck } from "api/typesGenerated";
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
			permissions.viewOrganizationIDPSyncSettings)
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
		(permissions.viewAnyMembers ||
			permissions.editAnyGroups ||
			permissions.assignAnyRoles ||
			permissions.viewAnyIdpSyncSettings ||
			permissions.editAnySettings)
	);
};
