import type { QueryClient, UseQueryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	CreateGroupRequest,
	Group,
	GroupMembersResponse,
	GroupRequest,
	PatchGroupRequest,
	UsersRequest,
} from "#/api/typesGenerated";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { prepareQuery } from "#/utils/filters";

type GroupSortOrder = "asc" | "desc";

export const groupsQueryKey = ["groups"];

/** @public */
export const groups = () => {
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

const getRootGroupQueryKey = (organization: string, groupName: string) => [
	"organization",
	organization,
	"group",
	groupName,
];

export const getGroupQueryKey = (
	organization: string,
	groupName: string,
	req: GroupRequest,
) => {
	const base = getRootGroupQueryKey(organization, groupName);
	return [...base, req];
};

export const group = (
	organization: string,
	groupName: string,
	req: GroupRequest,
): UseQueryOptions<Group> => {
	return {
		queryKey: getGroupQueryKey(organization, groupName, req),
		queryFn: ({ signal }) => API.getGroup(organization, groupName, req, signal),
	};
};

export const getGroupMembersQueryKey = (
	organization: string,
	groupName: string,
	req?: UsersRequest,
) => {
	const base = [...getRootGroupQueryKey(organization, groupName), "members"];
	return req ? [...base, req] : base;
};

export function groupMembers(
	organization: string,
	groupName: string,
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<GroupMembersResponse, UsersRequest> {
	return {
		searchParams,
		queryPayload: ({ limit, offset }) => {
			return {
				limit,
				offset,
				q: prepareQuery(searchParams.get("filter") ?? ""),
			};
		},

		queryKey: ({ payload }) =>
			getGroupMembersQueryKey(organization, groupName, payload),
		queryFn: ({ payload, signal }) =>
			API.getGroupMembers(organization, groupName, payload, signal),
	};
}

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

function selectGroupsByUserId(groups: Group[]): GroupsByUserId {
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
			await queryClient.invalidateQueries({
				queryKey: groupsQueryKey,
			});
			await queryClient.invalidateQueries({
				queryKey: getGroupsByOrganizationQueryKey(organization),
			});
		},
	};
};

export const patchGroup = (queryClient: QueryClient, organization: string) => {
	return {
		mutationFn: ({
			groupId,
			...request
		}: PatchGroupRequest & { groupId: string }) =>
			API.patchGroup(groupId, request),
		onSuccess: async (updatedGroup: Group) =>
			invalidateGroup(queryClient, organization, updatedGroup.name),
	};
};

export const deleteGroup = (queryClient: QueryClient, organization: string) => {
	return {
		mutationFn: ({ groupId }: { groupId: string; groupName: string }) =>
			API.deleteGroup(groupId),
		onSuccess: async (
			_: unknown,
			{ groupName }: { groupId: string; groupName: string },
		) => invalidateGroup(queryClient, organization, groupName),
	};
};

export const addMembers = (queryClient: QueryClient, organization: string) => {
	return {
		mutationFn: ({
			groupId,
			userIds,
		}: {
			groupId: string;
			userIds: string[];
		}) => API.addMembers(groupId, userIds),
		onSuccess: async (updatedGroup: Group) =>
			invalidateGroup(queryClient, organization, updatedGroup.name),
	};
};

export const removeMember = (
	queryClient: QueryClient,
	organization: string,
) => {
	return {
		mutationFn: ({ groupId, userId }: { groupId: string; userId: string }) =>
			API.removeMember(groupId, userId),
		onSuccess: async (updatedGroup: Group) =>
			invalidateGroup(queryClient, organization, updatedGroup.name),
	};
};

const invalidateGroup = (
	queryClient: QueryClient,
	organization: string,
	groupName: string,
) =>
	Promise.all([
		queryClient.invalidateQueries({ queryKey: groupsQueryKey }),
		queryClient.invalidateQueries({
			queryKey: getGroupsByOrganizationQueryKey(organization),
		}),
		queryClient.invalidateQueries({
			queryKey: getRootGroupQueryKey(organization, groupName),
		}),
	]);

function sortGroupsByName<T extends Group>(
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
