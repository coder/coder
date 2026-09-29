import { hashKey, type UseInfiniteQueryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeSessionThreadsResponse,
	OrganizationAISpendDetailsFilter,
	OrganizationAISpendFilter,
	OrganizationAISpendReport,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { permittedOrganizations } from "./organizations";

const SESSION_THREADS_INFINITE_PAGE_SIZE = 20;

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

const organizationAISpendScopeKey = (
	organizationId: string,
	filter: OrganizationAISpendFilter,
) => ["organizations", organizationId, "aiSpend", filter] as const;

export const paginatedOrganizationAISpend = (
	organizationId: string,
	filter: OrganizationAISpendFilter,
): UsePaginatedQueryOptions<
	OrganizationAISpendReport,
	OrganizationAISpendFilter
> => {
	return {
		queryPayload: () => filter,
		queryKey: ({ payload, pageNumber }) => [
			...organizationAISpendScopeKey(organizationId, payload),
			pageNumber,
		],
		queryFn: ({ payload, limit, offset }) =>
			API.experimental.getOrganizationAISpendUsers(organizationId, {
				...payload,
				limit,
				offset,
			}),
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

// The largest page the users endpoint serves.
const AI_SPEND_USERS_MAX_PAGE_SIZE = 100;

/**
 * Every user matching the filter, for summaries that the paged report does
 * not carry, such as which models lack pricing across all users.
 */
export const organizationAISpendAllUsers = (
	organizationId: string,
	filter: OrganizationAISpendFilter,
) => ({
	queryKey: [
		...organizationAISpendScopeKey(organizationId, filter),
		"allUsers",
	],
	queryFn: async (): Promise<OrganizationAISpendUser[]> => {
		const first = await API.experimental.getOrganizationAISpendUsers(
			organizationId,
			{ ...filter, limit: AI_SPEND_USERS_MAX_PAGE_SIZE },
		);
		const offsets: number[] = [];
		for (
			let offset = AI_SPEND_USERS_MAX_PAGE_SIZE;
			offset < first.count;
			offset += AI_SPEND_USERS_MAX_PAGE_SIZE
		) {
			offsets.push(offset);
		}
		const rest = await Promise.all(
			offsets.map((offset) =>
				API.experimental.getOrganizationAISpendUsers(organizationId, {
					...filter,
					limit: AI_SPEND_USERS_MAX_PAGE_SIZE,
					offset,
				}),
			),
		);
		return [first, ...rest].flatMap((report) => report.users);
	},
});

export const exportOrganizationAISpend = () => ({
	mutationFn: ({
		organizationId,
		filter,
	}: {
		organizationId: string;
		filter: OrganizationAISpendDetailsFilter;
	}) => API.exportOrganizationAISpend(organizationId, filter),
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
