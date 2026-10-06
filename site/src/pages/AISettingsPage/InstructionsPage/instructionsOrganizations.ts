import type { Organization } from "#/api/typesGenerated";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";

/**
 * Returns the organizations whose instructions the user can read, which
 * requires read access to the organization's chat model configurations.
 */
export const instructionsOrganizations = (
	organizations: readonly Organization[],
	permissionsByOrganization:
		| Readonly<Record<string, OrganizationPermissions | undefined>>
		| undefined,
): readonly Organization[] =>
	organizations.filter(
		(organization) =>
			permissionsByOrganization?.[organization.id]?.viewChatModelConfigs,
	);
