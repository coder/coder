import { useQuery } from "react-query";
import { organizationsPermissions } from "#/api/queries/organizations";
import type { Organization } from "#/api/typesGenerated";

type UseCanShareOrganizationMCPServersOptions = {
	enabled?: boolean;
};

// Top-level navigation must admit organization-scoped sharers without
// site-wide MCP permissions.
export const useCanShareOrganizationMCPServers = (
	organizations: readonly Organization[],
	options: UseCanShareOrganizationMCPServersOptions = {},
) => {
	const enabled = (options.enabled ?? true) && organizations.length > 0;
	const organizationsPermissionsQuery = useQuery({
		...organizationsPermissions(
			organizations.map((organization) => organization.id),
		),
		enabled,
	});
	return {
		canShare: organizations.some(
			(organization) =>
				organizationsPermissionsQuery.data?.[organization.id]
					?.shareMCPServerConfig,
		),
		isLoading: enabled && organizationsPermissionsQuery.isLoading,
		error: organizationsPermissionsQuery.error,
	};
};
