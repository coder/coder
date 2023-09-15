import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";
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
