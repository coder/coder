import type { QueryClient } from "react-query";
import { API } from "api/api";
import type {
  CreateOrganizationRequest,
  UpdateOrganizationRequest,
} from "api/typesGenerated";
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

interface UpdateOrganizationVariables {
  orgId: string;
  req: UpdateOrganizationRequest;
}

export const updateOrganization = (queryClient: QueryClient) => {
  return {
    mutationFn: (variables: UpdateOrganizationVariables) =>
      API.updateOrganization(variables.orgId, variables.req),

    onSuccess: async () => {
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
