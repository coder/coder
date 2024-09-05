import { API } from "api/api";
import type { Role } from "api/typesGenerated";
import type { QueryClient } from "react-query";

const getRoleQueryKey = (organizationId: string, roleName: string) => [
	"organization",
	organizationId,
	"role",
	roleName,
];

export const rolesQueryKey = ["roles"];

export const roles = () => {
	return {
		queryKey: rolesQueryKey,
		queryFn: API.getRoles,
	};
};

export const organizationRoles = (organization: string) => {
	return {
		queryKey: ["organization", organization, "roles"],
		queryFn: () => API.getOrganizationRoles(organization),
	};
};

export const createOrganizationRole = (
	queryClient: QueryClient,
	organization: string,
) => {
	return {
		mutationFn: (request: Role) =>
			API.createOrganizationRole(organization, request),
		onSuccess: async (updatedRole: Role) =>
			await queryClient.invalidateQueries(
				getRoleQueryKey(organization, updatedRole.name),
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
			await queryClient.invalidateQueries(
				getRoleQueryKey(organization, updatedRole.name),
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
			await queryClient.invalidateQueries(
				getRoleQueryKey(organization, roleName),
			),
	};
};
