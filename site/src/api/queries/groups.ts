import { API } from "api/api";
import type {
	CreateGroupRequest,
	Group,
	PatchGroupRequest,
} from "api/typesGenerated";
import type { QueryClient, UseQueryOptions } from "react-query";

type GroupSortOrder = "asc" | "desc";

export const groupsQueryKey = ["groups"];

const groups = () => {
	return {
		queryKey: groupsQueryKey,
		queryFn: () => API.getGroups(),
	} satisfies UseQueryOptions<Group[]>;
};

const getGroupsByOrganizationQueryKey = (organization: string) => [
	"organization",
	organization,
	"groups",
];

export const groupsByOrganization = (organization: string) => {
	return {
		queryKey: getGroupsByOrganizationQueryKey(organization),
		queryFn: () => API.getGroupsByOrganization(organization),
	} satisfies UseQueryOptions<Group[]>;
};

export const getGroupQueryKey = (organization: string, groupName: string) => [
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

export function groupsByUserId() {
	return {
		...groups(),
		select: selectGroupsByUserId,
	} satisfies UseQueryOptions<Group[], unknown, GroupsByUserId>;
}

export function groupsByUserIdInOrganization(organization: string) {
	return {
		...groupsByOrganization(organization),
		select: selectGroupsByUserId,
	} satisfies UseQueryOptions<Group[], unknown, GroupsByUserId>;
}

export function selectGroupsByUserId(groups: Group[]): GroupsByUserId {
	// Sorting here means that nothing has to be sorted for the individual
	// user arrays later
	const sorted = sortGroupsByName(groups, "asc");
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
}

export function groupsForUser(userId: string) {
	return {
		queryKey: groupsQueryKey,
		queryFn: () => API.getGroups({ userId }),
	} as const satisfies UseQueryOptions<Group[]>;
}

export const groupPermissionsKey = (groupId: string) => [
	"group",
	groupId,
	"permissions",
];

export const groupPermissions = (groupId: string) => {
	return {
		queryKey: groupPermissionsKey(groupId),
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
			await queryClient.invalidateQueries(groupsQueryKey);
			await queryClient.invalidateQueries(
				getGroupsByOrganizationQueryKey(organization),
			);
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
		onSuccess: async (_: unknown, groupId: string) =>
			invalidateGroup(queryClient, "default", groupId),
	};
};

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
		queryClient.invalidateQueries(groupsQueryKey),
		queryClient.invalidateQueries(
			getGroupsByOrganizationQueryKey(organization),
		),
		queryClient.invalidateQueries(getGroupQueryKey(organization, groupId)),
	]);

export function sortGroupsByName<T extends Group>(
	groups: readonly T[],
	order: GroupSortOrder,
) {
	return [...groups].sort((g1, g2) => {
		const key = g1.display_name && g2.display_name ? "display_name" : "name";
		const direction = order === "asc" ? 1 : -1;

		if (g1[key] === g2[key]) {
			return 0;
		}

		return (g1[key] < g2[key] ? -1 : 1) * direction;
	});
}
