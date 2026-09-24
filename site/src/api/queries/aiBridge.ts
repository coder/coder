import {
	hashKey,
	type UseInfiniteQueryOptions,
	type UseQueryOptions,
} from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeProvider,
	AIBridgeSessionThreadsResponse,
	OrganizationAISpendDetailsFilter,
	OrganizationAISpendFilter,
	OrganizationAISpendReport,
	Pagination,
} from "#/api/typesGenerated";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { permittedOrganizations } from "./organizations";

const SESSION_THREADS_INFINITE_PAGE_SIZE = 20;

export const aiBridgeProviders = (): UseQueryOptions<AIBridgeProvider[]> => ({
	queryKey: ["aiBridgeProviders"],
	queryFn: () => API.getAIBridgeProviders(),
});

export const aiBridgeModels = (
	options: Pagination & { model?: string },
): UseQueryOptions<string[]> => ({
	queryKey: ["aiBridgeModels", options],
	queryFn: () => API.getAIBridgeModels(options),
});

export const aiBridgeClients = (options: {
	q?: string;
	limit?: number;
	offset?: number;
}): UseQueryOptions<string[]> => ({
	queryKey: ["aiBridgeClients", options],
	queryFn: () => API.getAIBridgeClients(options),
});

export const paginatedSessions = (
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<AIBridgeListSessionsResponse, string> => {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ limit, offset, payload }) => {
			return ["aiBridgeSessions", limit, offset, payload] as const;
		},
		queryFn: ({ limit, offset, payload }) =>
			API.getAIBridgeSessionList({
				offset,
				limit,
				q: payload,
			}),
	};
};

// The spend endpoints authorize on reading the organization's group members,
// so this lists exactly the organizations they would serve.
export const aiSpendOrganizations = () =>
	permittedOrganizations({
		object: { resource_type: "group_member" },
		action: "read",
	});

/**
 * Spend filters the users endpoint does not accept yet. They are applied to
 * the report in the browser until the endpoint supports them.
 */
type OrganizationAISpendUserFilter = {
	/** Only include this user. */
	username?: string;
	/** Only include members of this organization group. */
	group?: string;
	/** Only include users with usage of models that had no price. */
	unconfiguredPricing?: boolean;
};

export type OrganizationAISpendQuery = OrganizationAISpendFilter &
	OrganizationAISpendUserFilter;

// The largest page the users endpoint serves.
const AI_SPEND_USERS_MAX_PAGE_SIZE = 100;

/**
 * Loads every user matching the server-side filter, keeps those matching the
 * browser-side filter, and returns the requested page with the count and
 * totals recomputed over the kept users.
 */
const getOrganizationAISpendUsersFilteredInBrowser = async (
	organizationId: string,
	{ username, group, unconfiguredPricing, ...filter }: OrganizationAISpendQuery,
	limit: number,
	offset: number,
): Promise<OrganizationAISpendReport> => {
	const [first, memberIds] = await Promise.all([
		API.getOrganizationAISpendUsers(organizationId, {
			...filter,
			limit: AI_SPEND_USERS_MAX_PAGE_SIZE,
		}),
		group
			? API.getGroup(organizationId, group, { exclude_members: false }).then(
					(g) => new Set(g.members.map((member) => member.id)),
				)
			: undefined,
	]);
	const remainingOffsets: number[] = [];
	for (
		let next = AI_SPEND_USERS_MAX_PAGE_SIZE;
		next < first.count;
		next += AI_SPEND_USERS_MAX_PAGE_SIZE
	) {
		remainingOffsets.push(next);
	}
	const rest = await Promise.all(
		remainingOffsets.map((pageOffset) =>
			API.getOrganizationAISpendUsers(organizationId, {
				...filter,
				limit: AI_SPEND_USERS_MAX_PAGE_SIZE,
				offset: pageOffset,
			}),
		),
	);
	const users = [first, ...rest]
		.flatMap((report) => report.users)
		.filter(
			(user) =>
				(!username || user.username === username) &&
				(!memberIds || memberIds.has(user.user_id)) &&
				(!unconfiguredPricing || user.unpriced_usage_count > 0),
		);
	return {
		...first,
		count: users.length,
		totals: {
			cost_micros: users.reduce((sum, user) => sum + user.cost_micros, 0),
			unpriced_usage_count: users.reduce(
				(sum, user) => sum + user.unpriced_usage_count,
				0,
			),
		},
		users: users.slice(offset, offset + limit),
	};
};

const organizationAISpendScopeKey = (
	organizationId: string,
	filter: OrganizationAISpendQuery,
) => ["organizations", organizationId, "aiSpend", filter] as const;

export const paginatedOrganizationAISpend = (
	organizationId: string,
	filter: OrganizationAISpendQuery,
): UsePaginatedQueryOptions<
	OrganizationAISpendReport,
	OrganizationAISpendQuery
> => {
	return {
		queryPayload: () => filter,
		queryKey: ({ payload, pageNumber }) => [
			...organizationAISpendScopeKey(organizationId, payload),
			pageNumber,
		],
		queryFn: ({ payload, limit, offset }) => {
			const { username, group, unconfiguredPricing, ...serverFilter } = payload;
			if (username || group || unconfiguredPricing) {
				return getOrganizationAISpendUsersFilteredInBrowser(
					organizationId,
					payload,
					limit,
					offset,
				);
			}
			return API.getOrganizationAISpendUsers(organizationId, {
				...serverFilter,
				limit,
				offset,
			});
		},
		// Every page aggregates the whole organization window.
		prefetch: false,
		// Rows from another organization or filter must not appear under the
		// new selection while its report loads; only a page change keeps the
		// previous rows behind the refresh overlay.
		placeholderData: (previousData, previousQuery) =>
			previousQuery &&
			hashKey(previousQuery.queryKey.slice(0, -1)) ===
				hashKey(organizationAISpendScopeKey(organizationId, filter))
				? previousData
				: undefined,
	};
};

/**
 * Every user matching the filter, for summaries that the paged report does
 * not carry, such as which models lack pricing across all users.
 */
export const organizationAISpendAllUsers = (
	organizationId: string,
	filter: OrganizationAISpendQuery,
) => ({
	queryKey: [
		...organizationAISpendScopeKey(organizationId, filter),
		"allUsers",
	],
	queryFn: async () => {
		const report = await getOrganizationAISpendUsersFilteredInBrowser(
			organizationId,
			filter,
			Number.POSITIVE_INFINITY,
			0,
		);
		return report.users;
	},
});

export const exportOrganizationAISpend = () => ({
	mutationFn: async ({
		organizationId,
		username,
		group,
		filter,
	}: {
		organizationId: string;
		/** Resolved to the user ID the export endpoint filters by. */
		username?: string;
		/** Resolved to the group ID the export endpoint filters by. */
		group?: string;
		filter: OrganizationAISpendDetailsFilter;
	}) => {
		const [user, groupDetails] = await Promise.all([
			username ? API.getUser(username) : undefined,
			group
				? API.getGroup(organizationId, group, { exclude_members: true })
				: undefined,
		]);
		return API.exportOrganizationAISpend(organizationId, {
			...filter,
			user_id: user?.id,
			group_id: groupDetails?.id,
		});
	},
});

export const infiniteSessionThreads = (sessionId: string) => {
	return {
		queryKey: ["aiBridgeSessionThreads", sessionId],
		getNextPageParam: (lastPage: AIBridgeSessionThreadsResponse) => {
			const threads = lastPage.threads;
			if (threads.length < SESSION_THREADS_INFINITE_PAGE_SIZE) {
				return undefined;
			}
			return threads.at(-1)?.id;
		},
		initialPageParam: undefined as string | undefined,
		queryFn: ({ pageParam }) =>
			API.getAIBridgeSessionThreads(sessionId, {
				limit: SESSION_THREADS_INFINITE_PAGE_SIZE,
				after_id: pageParam as string | undefined,
			}),
	} satisfies UseInfiniteQueryOptions<AIBridgeSessionThreadsResponse>;
};
