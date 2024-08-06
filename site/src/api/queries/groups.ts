import type { QueryClient, UseQueryOptions } from "react-query";
import { API } from "api/api";
import type {
  CreateGroupRequest,
  Group,
  PatchGroupRequest,
  ReducedGroup,
} from "api/typesGenerated";

type GroupSortOrder = "asc" | "desc";

const getGroupsQueryKey = (organization: string) => [
  "organization",
  organization,
  "groups",
];

export const groups = (organization: string) => {
  return {
    queryKey: getGroupsQueryKey(organization),
    queryFn: () => API.getGroups(organization),
  } satisfies UseQueryOptions<Group[]>;
};

const getGroupQueryKey = (organization: string, groupName: string) => [
  "organization",
  organization,
  "group",
  groupName,
];

export const group = (organization: string, groupName: string) => {
  return {
    queryKey: getGroupQueryKey(organization, groupName),
    queryFn: () => API.getGroup(organization, groupName),
  };
};

export type GroupsByUserId = Readonly<Map<string, readonly Group[]>>;

export function groupsByUserId(organization: string) {
  return {
    ...groups(organization),
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

export function groupsForUser(organization: string, userId: string) {
  return {
    ...groups(organization),
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
    queryKey: ["group", groupId, "permissions"],
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

export const createGroup = (queryClient: QueryClient, organization: string) => {
  return {
    mutationFn: (request: CreateGroupRequest) =>
      API.createGroup(organization, request),
    onSuccess: async () => {
      await queryClient.invalidateQueries(getGroupsQueryKey(organization));
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
      invalidateGroup(queryClient, "default", updatedGroup.id),
  };
};

export const deleteGroup = (queryClient: QueryClient) => {
  return {
    mutationFn: API.deleteGroup,
    onSuccess: async (_: void, groupId: string) =>
      invalidateGroup(queryClient, "default", groupId),
  };
};

export function reducedGroupsForUser(organization: string, userId: string) {
  return {
    queryKey: ["organization", organization, "user", userId, "reduced-groups"],
    queryFn: () => API.getReducedGroupsForUser(organization, userId),
  } as const satisfies UseQueryOptions<ReducedGroup[], unknown, readonly ReducedGroup[]>;
}

export const addMember = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      API.addMember(groupId, userId),
    onSuccess: async (updatedGroup: Group) =>
      invalidateGroup(queryClient, "default", updatedGroup.id),
  };
};

export const removeMember = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
      API.removeMember(groupId, userId),
    onSuccess: async (updatedGroup: Group) =>
      invalidateGroup(queryClient, "default", updatedGroup.id),
  };
};

export const invalidateGroup = (
  queryClient: QueryClient,
  organization: string,
  groupId: string,
) =>
  Promise.all([
    queryClient.invalidateQueries(getGroupsQueryKey(organization)),
    queryClient.invalidateQueries(getGroupQueryKey(organization, groupId)),
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
