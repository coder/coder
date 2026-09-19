import type { UseInfiniteQueryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeSessionThreadsResponse,
	OrganizationAISpendDetails,
	OrganizationAISpendDetailsFilter,
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

const organizationAISpendDetailsKey = (
	organizationId: string,
	filter: OrganizationAISpendDetailsFilter,
	pageNumber: number,
) =>
	[
		"organization",
		organizationId,
		"aiSpendDetails",
		filter,
		pageNumber,
	] as const;

export const paginatedOrganizationAISpendDetails = (
	organizationId: string,
	filter: OrganizationAISpendDetailsFilter,
): UsePaginatedQueryOptions<
	OrganizationAISpendDetails,
	OrganizationAISpendDetailsFilter
> => ({
	queryPayload: () => filter,
	queryKey: ({ payload, pageNumber }) =>
		organizationAISpendDetailsKey(organizationId, payload, pageNumber),
	queryFn: ({ payload, limit, offset }) =>
		API.getOrganizationAISpendDetails(organizationId, {
			...payload,
			limit,
			offset,
		}),
	prefetch: false,
	placeholderData: (previousData, previousQuery) =>
		previousQuery?.queryKey[1] === organizationId ? previousData : undefined,
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
