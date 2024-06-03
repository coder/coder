import type { QueryClient } from "react-query";
import { API } from "api/api";
import type { CreateOrganizationRequest } from "api/typesGenerated";
import { meKey, myOrganizationsKey } from "./users";

export const createOrganization = (queryClient: QueryClient) => {
  return {
    mutationFn: (params: CreateOrganizationRequest) =>
      API.createOrganization(params),

    onSuccess: async () => {
      await queryClient.invalidateQueries(meKey);
      await queryClient.invalidateQueries(myOrganizationsKey);
    },
  };
};

export const deleteOrganization = (queryClient: QueryClient) => {
  return {
    mutationFn: (orgId: string) => API.deleteOrganization(orgId),

    onSuccess: async () => {
      await queryClient.invalidateQueries(meKey);
      await queryClient.invalidateQueries(myOrganizationsKey);
    },
  };
};
