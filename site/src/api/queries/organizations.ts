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

export const organizationMembers = (id: string) => {
  return {
    queryFn: () => API.getOrganizationMembers(id),
    queryKey: ["organization", id, "members"],
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

/**
 * Fetch permissions for a single organization.
 *
 * If the ID is undefined, return a disabled query.
 */
export const organizationPermissions = (organizationId: string | undefined) => {
  if (!organizationId) {
    return { enabled: false };
  }
  return {
    queryKey: ["organization", organizationId, "permissions"],
    queryFn: () =>
      API.checkAuthorization({
        checks: {
          viewUsers: {
            object: {
              resource_type: "user",
              organization_id: organizationId,
            },
            action: "read",
          },
          editUsers: {
            object: {
              resource_type: "user",
              organization_id: organizationId,
            },
            action: "update",
          },
          createGroup: {
            object: {
              resource_type: "group",
              organization_id: organizationId,
            },
            action: "create",
          },
          viewGroups: {
            object: {
              resource_type: "group",
              organization_id: organizationId,
            },
            action: "read",
          },
          editOrganization: {
            object: {
              resource_type: "organization",
              organization_id: organizationId,
            },
            action: "update",
          },
          auditOrganization: {
            object: {
              resource_type: "audit_log",
              organization_id: organizationId,
            },
            action: "read",
          },
        },
      }),
  };
};
