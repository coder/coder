import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";
import { checkAuthorization } from "api/api";
import {
  CreateGroupRequest,
  Group,
  PatchGroupRequest,
} from "api/typesGenerated";

export const groups = (organizationId: string) => {
  return {
    queryKey: ["groups"],
    queryFn: () => API.getGroups(organizationId),
  };
};

export const group = (groupId: string) => {
  return {
    queryKey: ["group", groupId],
    queryFn: () => API.getGroup(groupId),
  };
};

export const groupPermissions = (groupId: string) => {
  return {
    queryKey: ["group", groupId, "permissions"],
    queryFn: () =>
      checkAuthorization({
        checks: {
          canUpdateGroup: {
            object: {
              resource_type: "group",
              resource_id: groupId,
            },
            action: "update",
          },
        },
      }),
  };
};

export const createGroup = (queryClient: QueryClient) => {
  return {
    mutationFn: ({
      organizationId,
      ...request
    }: CreateGroupRequest & { organizationId: string }) =>
      API.createGroup(organizationId, request),
    onSuccess: async () => {
      await queryClient.invalidateQueries(["groups"]);
    },
  };
};

export const patchGroup = (queryClient: QueryClient) => {
  return {
    mutationFn: ({
      groupId,
      ...request
    }: PatchGroupRequest & { groupId: string }) =>
      API.patchGroup(groupId, request),
    onSuccess: async (updatedGroup: Group) => {
      await Promise.all([
        queryClient.invalidateQueries(["groups"]),
        queryClient.invalidateQueries(["group", updatedGroup.id]),
      ]);
    },
  };
};

export const deleteGroup = (queryClient: QueryClient) => {
  return {
    mutationFn: API.deleteGroup,
    onSuccess: async (_: void, groupId: string) => {
      await Promise.all([
        queryClient.invalidateQueries(["groups"]),
        queryClient.invalidateQueries(["group", groupId]),
      ]);
    },
  };
};

export const addMember = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      API.addMember(groupId, userId),
    onSuccess: async (updatedGroup: Group) => {
      await Promise.all([
        queryClient.invalidateQueries(["groups"]),
        queryClient.invalidateQueries(["group", updatedGroup.id]),
      ]);
    },
  };
};

export const removeMember = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      API.removeMember(groupId, userId),
    onSuccess: async (updatedGroup: Group) => {
      await Promise.all([
        queryClient.invalidateQueries(["groups"]),
        queryClient.invalidateQueries(["group", updatedGroup.id]),
      ]);
    },
  };
};
