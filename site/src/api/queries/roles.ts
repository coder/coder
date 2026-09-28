import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type { Role } from "#/api/typesGenerated";

const getRoleQueryKey = (organizationId: string, roleName: string) => [
	"organization",
	organizationId,
	"role",
	roleName,
];

export const rolesQueryKey = ["roles"];

export const organizationRolesQueryKey = (organization: string) =>
	["organization", organization, "roles"] as const;

export const roles = () => {
	return {
		queryKey: rolesQueryKey,
		queryFn: API.getRoles,
	};
};

export const organizationRoles = (organization: string) => {
	return {
		queryKey: organizationRolesQueryKey(organization),
		queryFn: () => API.getOrganizationRoles(organization),
	};
};

const invalidateOrganizationRoles = async (
	queryClient: QueryClient,
	organization: string,
	roleName: string,
) => {
	await queryClient.invalidateQueries({
		queryKey: organizationRolesQueryKey(organization),
	});
	await queryClient.invalidateQueries({
		queryKey: getRoleQueryKey(organization, roleName),
	});
};

export const createOrganizationRole = (
	queryClient: QueryClient,
	organization: string,
) => {
	return {
		mutationFn: (request: Role) =>
			API.createOrganizationRole(organization, request),
		onSuccess: async (updatedRole: Role) =>
			await invalidateOrganizationRoles(
				queryClient,
				organization,
				updatedRole.name,
			),
	};
};

export const updateOrganizationRole = (
	queryClient: QueryClient,
	organization: string,
) => {
	return {
		mutationFn: (request: Role) =>
			API.updateOrganizationRole(organization, request),
		onSuccess: async (updatedRole: Role) =>
			await invalidateOrganizationRoles(
				queryClient,
				organization,
				updatedRole.name,
			),
	};
};

export const deleteOrganizationRole = (
	queryClient: QueryClient,
	organization: string,
) => {
	return {
		mutationFn: (roleName: string) =>
			API.deleteOrganizationRole(organization, roleName),
		onSuccess: async (_: unknown, roleName: string) =>
			await invalidateOrganizationRoles(queryClient, organization, roleName),
	};
};
