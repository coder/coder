import type { Organization } from "#/api/typesGenerated";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";

/** Returns the organizations whose skills the user can read. */
export const readableSkillsOrganizations = (
	organizations: readonly Organization[],
	permissionsByOrganization:
		| Readonly<Record<string, OrganizationPermissions | undefined>>
		| undefined,
): readonly Organization[] =>
	organizations.filter(
		(organization) =>
			permissionsByOrganization?.[organization.id]?.viewOrganizationSkills,
	);
