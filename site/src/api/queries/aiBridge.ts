import type { UseInfiniteQueryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListInterceptionsResponse,
	AIBridgeListSessionsResponse,
	AIBridgeSessionThreadsResponse,
} from "#/api/typesGenerated";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";

export const paginatedInterceptions = (
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<AIBridgeListInterceptionsResponse, string> => {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ limit, offset, payload }) => {
			return ["aiBridgeInterceptions", limit, offset, payload] as const;
		},
		queryFn: ({ limit, offset, payload }) =>
			API.getAIBridgeInterceptions({
				offset,
				limit,
				q: payload,
			}),
	};
};

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

export const infiniteSessionThreads = (sessionId: string, limit = 1) => {
	return {
		queryKey: ["aiBridgeSessionThreads", sessionId],
		getNextPageParam: (lastPage: AIBridgeSessionThreadsResponse) => {
			const threads = lastPage.threads;
			if (threads.length < limit) {
				return undefined;
			}
			return threads[threads.length - 1].id;
		},
		initialPageParam: undefined as string | undefined,
		queryFn: ({ pageParam }) => {
			console.warn("Fetching threads with pageParam:", pageParam);
			return API.getAIBridgeSessionThreads(sessionId, {
				limit,
				after_id: pageParam as string | undefined,
			});
		},
	} satisfies UseInfiniteQueryOptions<AIBridgeSessionThreadsResponse>;
};
