import type { QueryClient, UseQueryOptions } from "react-query";
import { API } from "api/api";
import type {
  CreateGroupRequest,
  Group,
  PatchGroupRequest,
} from "api/typesGenerated";

const GROUPS_QUERY_KEY = ["groups"];
type GroupSortOrder = "asc" | "desc";

const getGroupQueryKey = (groupId: string) => ["group", groupId];

export const groups = (organizationId: string) => {
  return {
    queryKey: GROUPS_QUERY_KEY,
    queryFn: () => API.getGroups(organizationId),
  } satisfies UseQueryOptions<Group[]>;
};

export const group = (groupId: string) => {
  return {
    queryKey: getGroupQueryKey(groupId),
    queryFn: () => API.getGroup(groupId),
  };
};

export type GroupsByUserId = Readonly<Map<string, readonly Group[]>>;

export function groupsByUserId(organizationId: string) {
  return {
    ...groups(organizationId),
    select: (allGroups) => {
      // Sorting here means that nothing has to be sorted for the individual
      // user arrays later
      const sorted = sortGroupsByName(allGroups, "asc");
      const userIdMapper = new Map<string, Group[]>();

      for (const group of sorted) {
        for (const user of group.members) {
          let groupsForUser = userIdMapper.get(user.id);
          if (groupsForUser === undefined) {
            groupsForUser = [];
            userIdMapper.set(user.id, groupsForUser);
          }

          groupsForUser.push(group);
        }
      }

      return userIdMapper as GroupsByUserId;
    },
  } satisfies UseQueryOptions<Group[], unknown, GroupsByUserId>;
}

export function groupsForUser(organizationId: string, userId: string) {
  return {
    ...groups(organizationId),
    select: (allGroups) => {
      const groupsForUser = allGroups.filter((group) => {
        const groupMemberIds = group.members.map((member) => member.id);
        return groupMemberIds.includes(userId);
      });

      return sortGroupsByName(groupsForUser, "asc");
    },
  } as const satisfies UseQueryOptions<Group[], unknown, readonly Group[]>;
}

export const groupPermissions = (groupId: string) => {
  return {
    queryKey: [...getGroupQueryKey(groupId), "permissions"],
    queryFn: () =>
      API.checkAuthorization({
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
      await queryClient.invalidateQueries(GROUPS_QUERY_KEY);
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
    onSuccess: async (updatedGroup: Group) =>
      invalidateGroup(queryClient, updatedGroup.id),
  };
};

export const deleteGroup = (queryClient: QueryClient) => {
  return {
    mutationFn: API.deleteGroup,
    onSuccess: async (_: void, groupId: string) =>
      invalidateGroup(queryClient, groupId),
  };
};

export const addMember = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      API.addMember(groupId, userId),
    onSuccess: async (updatedGroup: Group) =>
      invalidateGroup(queryClient, updatedGroup.id),
  };
};

export const removeMember = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      API.removeMember(groupId, userId),
    onSuccess: async (updatedGroup: Group) =>
      invalidateGroup(queryClient, updatedGroup.id),
  };
};

export const invalidateGroup = (queryClient: QueryClient, groupId: string) =>
  Promise.all([
    queryClient.invalidateQueries(GROUPS_QUERY_KEY),
    queryClient.invalidateQueries(getGroupQueryKey(groupId)),
  ]);

export function sortGroupsByName(
  groups: readonly Group[],
  order: GroupSortOrder,
) {
  return [...groups].sort((g1, g2) => {
    const key = g1.display_name && g2.display_name ? "display_name" : "name";

    if (g1[key] === g2[key]) {
      return 0;
    }

    if (order === "asc") {
      return g1[key] < g2[key] ? -1 : 1;
    } else {
      return g1[key] < g2[key] ? 1 : -1;
    }
  });
}
