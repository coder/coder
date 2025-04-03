import { API } from "api/api";
import type {
	CreateOrganizationRequest,
	GroupSyncSettings,
	PaginatedMembersRequest,
	PaginatedMembersResponse,
	RoleSyncSettings,
	UpdateOrganizationRequest,
} from "api/typesGenerated";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";
import {
	type OrganizationPermissionName,
	type OrganizationPermissions,
	organizationPermissionChecks,
} from "modules/permissions/organizations";
import {
	type WorkspacePermissionName,
	type WorkspacePermissions,
	workspacePermissionChecks,
} from "modules/permissions/workspaces";
import type { QueryClient } from "react-query";
import { meKey } from "./users";

export const createOrganization = (queryClient: QueryClient) => {
	return {
		mutationFn: (params: CreateOrganizationRequest) =>
			API.createOrganization(params),

		onSuccess: async () => {
			await queryClient.invalidateQueries(meKey);
			await queryClient.invalidateQueries(organizationsKey);
		},
	};
};

interface UpdateOrganizationVariables {
	organizationId: string;
	req: UpdateOrganizationRequest;
}

export const updateOrganization = (queryClient: QueryClient) => {
	return {
		mutationFn: (variables: UpdateOrganizationVariables) =>
			API.updateOrganization(variables.organizationId, variables.req),

		onSuccess: async () => {
			await queryClient.invalidateQueries(organizationsKey);
		},
	};
};

export const deleteOrganization = (queryClient: QueryClient) => {
	return {
		mutationFn: (organizationId: string) =>
			API.deleteOrganization(organizationId),

		onSuccess: async () => {
			await queryClient.invalidateQueries(meKey);
			await queryClient.invalidateQueries(organizationsKey);
		},
	};
};

export const organizationMembersKey = (id: string) => [
	"organization",
	id,
	"members",
];

/**
 * Creates a query configuration to fetch all members of an organization.
 *
 * Unlike the paginated version, this function sets the `limit` parameter to 0,
 * which instructs the API to return all organization members in a single request
 * without pagination.
 *
 * @param id - The unique identifier of the organization
 * @returns A query configuration object for use with React Query
 *
 * @see paginatedOrganizationMembers - For fetching members with pagination support
 */
export const organizationMembers = (id: string) => {
	return {
		queryFn: () => API.getOrganizationPaginatedMembers(id, { limit: 0 }),
		queryKey: organizationMembersKey(id),
	};
};

export const paginatedOrganizationMembers = (
	id: string,
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<
	PaginatedMembersResponse,
	PaginatedMembersRequest
> => {
	return {
		searchParams,
		queryPayload: ({ limit, offset }) => {
			return {
				limit: limit,
				offset: offset,
			};
		},
		queryKey: ({ payload }) => [...organizationMembersKey(id), payload],
		queryFn: ({ payload }) => API.getOrganizationPaginatedMembers(id, payload),
	};
};

export const addOrganizationMember = (queryClient: QueryClient, id: string) => {
	return {
		mutationFn: (userId: string) => {
			return API.addOrganizationMember(id, userId);
		},

		onSuccess: async () => {
			await queryClient.invalidateQueries(["organization", id, "members"]);
		},
	};
};

export const removeOrganizationMember = (
	queryClient: QueryClient,
	id: string,
) => {
	return {
		mutationFn: (userId: string) => {
			return API.removeOrganizationMember(id, userId);
		},

		onSuccess: async () => {
			await queryClient.invalidateQueries(["organization", id, "members"]);
		},
	};
};

export const updateOrganizationMemberRoles = (
	queryClient: QueryClient,
	organizationId: string,
) => {
	return {
		mutationFn: ({ userId, roles }: { userId: string; roles: string[] }) => {
			return API.updateOrganizationMemberRoles(organizationId, userId, roles);
		},

		onSuccess: async () => {
			await queryClient.invalidateQueries([
				"organization",
				organizationId,
				"members",
			]);
		},
	};
};

export const organizationsKey = ["organizations"] as const;

export const organizations = () => {
	return {
		queryKey: organizationsKey,
		queryFn: () => API.getOrganizations(),
	};
};

export const getProvisionerDaemonsKey = (
	organization: string,
	tags?: Record<string, string>,
) => ["organization", organization, tags, "provisionerDaemons"];

export const provisionerDaemons = (
	organization: string,
	tags?: Record<string, string>,
) => {
	return {
		queryKey: getProvisionerDaemonsKey(organization, tags),
		queryFn: () => API.getProvisionerDaemonsByOrganization(organization, tags),
	};
};

export const getProvisionerDaemonGroupsKey = (organization: string) => [
	"organization",
	organization,
	"provisionerDaemons",
];

export const provisionerDaemonGroups = (organization: string) => {
	return {
		queryKey: getProvisionerDaemonGroupsKey(organization),
		queryFn: () => API.getProvisionerDaemonGroupsByOrganization(organization),
	};
};

export const getGroupIdpSyncSettingsKey = (organization: string) => [
	"organizations",
	organization,
	"groupIdpSyncSettings",
];

export const groupIdpSyncSettings = (organization: string) => {
	return {
		queryKey: getGroupIdpSyncSettingsKey(organization),
		queryFn: () => API.getGroupIdpSyncSettingsByOrganization(organization),
	};
};

export const patchGroupSyncSettings = (
	organization: string,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: (request: GroupSyncSettings) =>
			API.patchGroupIdpSyncSettings(request, organization),
		onSuccess: async () =>
			await queryClient.invalidateQueries(groupIdpSyncSettings(organization)),
	};
};

export const getRoleIdpSyncSettingsKey = (organization: string) => [
	"organizations",
	organization,
	"roleIdpSyncSettings",
];

export const roleIdpSyncSettings = (organization: string) => {
	return {
		queryKey: getRoleIdpSyncSettingsKey(organization),
		queryFn: () => API.getRoleIdpSyncSettingsByOrganization(organization),
	};
};

export const patchRoleSyncSettings = (
	organization: string,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: (request: RoleSyncSettings) =>
			API.patchRoleIdpSyncSettings(request, organization),
		onSuccess: async () =>
			await queryClient.invalidateQueries(
				getRoleIdpSyncSettingsKey(organization),
			),
	};
};

export const provisionerJobQueryKey = (orgId: string) => [
	"organization",
	orgId,
	"provisionerjobs",
];

export const provisionerJobs = (orgId: string) => {
	return {
		queryKey: provisionerJobQueryKey(orgId),
		queryFn: () => API.getProvisionerJobs(orgId),
	};
};

/**
 * Fetch permissions for all provided organizations.
 *
 * If organizations are undefined, return a disabled query.
 */
export const organizationsPermissions = (
	organizationIds: string[] | undefined,
) => {
	if (!organizationIds) {
		return { enabled: false };
	}

	return {
		queryKey: ["organizations", organizationIds.sort(), "permissions"],
		queryFn: async () => {
			// Only request what we need for the sidebar, which is one edit permission
			// per sub-link (settings, groups, roles, and members pages) that tells us
			// whether to show that page, since we only show them if you can edit (and
			// not, at the moment if you can only view).

			// The endpoint takes a flat array, so to avoid collisions prepend each
			// check with the org ID (the key can be anything we want).
			const prefixedChecks = organizationIds.flatMap((orgId) =>
				Object.entries(organizationPermissionChecks(orgId)).map(
					([key, val]) => [`${orgId}.${key}`, val],
				),
			);

			const response = await API.checkAuthorization({
				checks: Object.fromEntries(prefixedChecks),
			});

			// Now we can unflatten by parsing out the org ID from each check.
			return Object.entries(response).reduce(
				(acc, [key, value]) => {
					const index = key.indexOf(".");
					const orgId = key.substring(0, index);
					const perm = key.substring(index + 1);
					if (!acc[orgId]) {
						acc[orgId] = {};
					}
					acc[orgId][perm as OrganizationPermissionName] = value;
					return acc;
				},
				{} as Record<string, Partial<OrganizationPermissions>>,
			) as Record<string, OrganizationPermissions>;
		},
	};
};

export const workspacePermissionsByOrganization = (
	organizationIds: string[] | undefined,
) => {
	if (!organizationIds) {
		return { enabled: false };
	}

	return {
		queryKey: ["workspaces", organizationIds.sort(), "permissions"],
		queryFn: async () => {
			const prefixedChecks = organizationIds.flatMap((orgId) =>
				Object.entries(workspacePermissionChecks(orgId)).map(([key, val]) => [
					`${orgId}.${key}`,
					val,
				]),
			);

			const response = await API.checkAuthorization({
				checks: Object.fromEntries(prefixedChecks),
			});

			return Object.entries(response).reduce(
				(acc, [key, value]) => {
					const index = key.indexOf(".");
					const orgId = key.substring(0, index);
					const perm = key.substring(index + 1);
					if (!acc[orgId]) {
						acc[orgId] = {};
					}
					acc[orgId][perm as WorkspacePermissionName] = value;
					return acc;
				},
				{} as Record<string, Partial<WorkspacePermissions>>,
			) as Record<string, WorkspacePermissions>;
		},
	};
};

export const getOrganizationIdpSyncClaimFieldValuesKey = (
	organization: string,
	field: string,
) => [organization, "idpSync", "fieldValues", field];

export const organizationIdpSyncClaimFieldValues = (
	organization: string,
	field: string,
) => {
	return {
		queryKey: getOrganizationIdpSyncClaimFieldValuesKey(organization, field),
		queryFn: () =>
			API.getOrganizationIdpSyncClaimFieldValues(organization, field),
	};
};
