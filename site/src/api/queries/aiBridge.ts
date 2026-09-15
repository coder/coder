import type { UseInfiniteQueryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeSessionThreadsResponse,
	OrganizationAISpendFilter,
	OrganizationAISpendReport,
} from "#/api/typesGenerated";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";

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

const organizationAISpendKey = (
	organizationId: string,
	filter: OrganizationAISpendFilter,
	pageNumber: number,
) => ["organizations", organizationId, "aiSpend", filter, pageNumber] as const;

export const paginatedOrganizationAISpend = (
	organizationId: string,
	filter: OrganizationAISpendFilter,
): UsePaginatedQueryOptions<
	OrganizationAISpendReport,
	OrganizationAISpendFilter
> => {
	return {
		queryPayload: () => filter,
		queryKey: ({ payload, pageNumber }) =>
			organizationAISpendKey(organizationId, payload, pageNumber),
		queryFn: ({ payload, limit, offset }) =>
			API.getOrganizationAISpendUsers(organizationId, {
				...payload,
				limit,
				offset,
			}),
		// Every page aggregates the whole organization window, so the adjacent
		// pages are not fetched speculatively.
		prefetch: false,
		staleTime: 60_000,
		// Rows from another organization must not appear under the newly
		// selected organization while its report loads.
		placeholderData: (previousData, previousQuery) =>
			previousQuery?.queryKey[1] === organizationId ? previousData : undefined,
	};
};

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
