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

export const canAccessAnyChatModelConfig = (
	permissions: Permissions | undefined,
): boolean => {
	return (
		permissions !== undefined &&
		(permissions.viewAnyChatModelConfig ||
			permissions.createAnyChatModelConfig ||
			permissions.editAnyChatModelConfig ||
			permissions.deleteAnyChatModelConfig ||
			permissions.shareAnyChatModelConfig)
	);
};

/**
 * Whether the user can open the Coder Agents settings page at
 * /ai/settings/coder-agents. Deployment admins manage deployment-wide
 * agent settings, and organization model admins manage their
 * organizations' model configurations on the same page.
 */
export const canAccessCoderAgentsSettings = (
	permissions: Permissions | undefined,
): boolean => {
	return (
		permissions !== undefined &&
		(permissions.editDeploymentConfig ||
			canAccessAnyChatModelConfig(permissions))
	);
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
			permissions.viewAIGatewayKeys ||
			canAccessAnyChatModelConfig(permissions))
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
