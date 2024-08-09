import type { QueryClient } from "react-query";
import { API } from "api/api";
import type { Role } from "api/typesGenerated";

const getRoleQueryKey = (organizationId: string, roleName: string) => [
  "organization",
  organizationId,
  "role",
  roleName,
];

export const roles = () => {
  return {
    queryKey: ["roles"],
    queryFn: API.getRoles,
  };
};

export const organizationRoles = (organization: string) => {
  return {
    queryKey: ["organization", organization, "roles"],
    queryFn: () => API.getOrganizationRoles(organization),
  };
};

export const patchOrganizationRole = (
  queryClient: QueryClient,
  organization: string,
) => {
  return {
    mutationFn: (request: Role) =>
      API.patchOrganizationRole(organization, request),
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
    onSuccess: async (_: void, roleName: string) =>
      await queryClient.invalidateQueries(
        getRoleQueryKey(organization, roleName),
      ),
  };
};
