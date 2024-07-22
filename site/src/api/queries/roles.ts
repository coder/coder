import { API } from "api/api";

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
