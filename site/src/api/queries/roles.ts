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

export const organizationRoles = (organizationId: string) => {
  return {
    queryKey: ["organization", organizationId, "roles"],
    queryFn: () => API.getOrganizationRoles(organizationId),
  };
};

export const patchOrganizationRole = (
  queryClient: QueryClient,
  organizationId: string,
) => {
  return {
    mutationFn: (request: Role) =>
      API.patchOrganizationRole(organizationId, request),
    onSuccess: async (updatedRole: Role) =>
      await queryClient.invalidateQueries(
        getRoleQueryKey(organizationId, updatedRole.name),
      ),
  };
};

export const deleteRole = (
  queryClient: QueryClient,
  organizationId: string,
) => {
  return {
    mutationFn: API.deleteOrganizationRole,
    onSuccess: async (_: void, roleName: string) =>
      await queryClient.invalidateQueries(
        getRoleQueryKey(organizationId, roleName),
      ),
  };
};
