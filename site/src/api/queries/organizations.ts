import type { QueryClient } from "react-query";
import { API } from "api/api";
import type {
  CreateOrganizationRequest,
  UpdateOrganizationRequest,
} from "api/typesGenerated";
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
  orgId: string;
  req: UpdateOrganizationRequest;
}

export const updateOrganization = (queryClient: QueryClient) => {
  return {
    mutationFn: (variables: UpdateOrganizationVariables) =>
      API.updateOrganization(variables.orgId, variables.req),

    onSuccess: async () => {
      await queryClient.invalidateQueries(organizationsKey);
    },
  };
};

export const deleteOrganization = (queryClient: QueryClient) => {
  return {
    mutationFn: (orgId: string) => API.deleteOrganization(orgId),

    onSuccess: async () => {
      await queryClient.invalidateQueries(meKey);
      await queryClient.invalidateQueries(organizationsKey);
    },
  };
};

export const organizationMembers = (id: string) => {
  return {
    queryFn: () => API.getOrganizationMembers(id),
    key: ["organization", id, "members"],
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

export const organizationsKey = ["organizations", "me"] as const;

export const organizations = () => {
  return {
    queryKey: organizationsKey,
    queryFn: () => API.getOrganizations(),
  };
};
