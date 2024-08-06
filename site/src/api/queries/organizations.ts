import type { QueryClient } from "react-query";
import { API } from "api/api";
import type {
  AuthorizationCheck,
  AuthorizationResponse,
  CreateOrganizationRequest,
  Organization,
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

export const getProvisionerDaemonsKey = (organization: string) => [
  "organization",
  organization,
  "provisionerDaemons",
];

export const provisionerDaemons = (organization: string) => {
  return {
    queryKey: getProvisionerDaemonsKey(organization),
    queryFn: () => API.getProvisionerDaemonsByOrganization(organization),
  };
};

const orgChecks = (
  organizationId: string,
): Record<string, AuthorizationCheck> => ({
  viewMembers: {
    object: {
      resource_type: "organization_member",
      organization_id: organizationId,
    },
    action: "read",
  },
  editMembers: {
    object: {
      resource_type: "organization_member",
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
  editGroups: {
    object: {
      resource_type: "group",
      organization_id: organizationId,
    },
    action: "update",
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
});

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
      API.checkAuthorization({ checks: orgChecks(organizationId) }),
  };
};

/**
 * Fetch permissions for all provided organizations.
 *
 * If organizations are undefined, return a disabled query.
 */
export const organizationsPermissions = (
  organizations: Organization[] | undefined,
) => {
  if (!organizations) {
    return { enabled: false };
  }

  return {
    queryKey: ["organizations", "permissions"],
    queryFn: async () => {
      // The endpoint takes a flat array, so to avoid collisions prepend each
      // check with the org ID (the key can be anything we want).
      const checks = organizations
        .map((org) =>
          Object.entries(orgChecks(org.id)).map(([key, val]) => [
            `${org.id}.${key}`,
            val,
          ]),
        )
        .flat();

      const response = await API.checkAuthorization({
        checks: Object.fromEntries(checks),
      });

      // Now we can unflatten by parsing out the org ID from each check.
      return Object.entries(response).reduce(
        (acc, [key, value]) => {
          const index = key.indexOf(".");
          const orgId = key.substring(0, index);
          const perm = key.substring(index + 1);
          if (!acc[orgId]) {
            acc[orgId] = { [perm]: value };
          } else {
            acc[orgId][perm] = value;
          }
          return acc;
        },
        {} as Record<string, AuthorizationResponse>,
      );
    },
  };
};
