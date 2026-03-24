import { API } from "api/api";
import type {
	AIBridgeListInterceptionsResponse,
	AIBridgeListSessionsResponse,
	AIBridgeSessionThreadsResponse,
} from "api/typesGenerated";
import { useFilterParamsKey } from "components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";
import type { UseInfiniteQueryOptions } from "react-query";

export const paginatedInterceptions = (
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<AIBridgeListInterceptionsResponse, string> => {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ payload, pageNumber }) => {
			return ["aiBridgeInterceptions", payload, pageNumber] as const;
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
		queryKey: ({ payload, pageNumber }) => {
			return ["aiBridgeSessions", payload, pageNumber] as const;
		},
		queryFn: ({ offset, limit, payload }) =>
			API.getAIBridgeSessionList({
				offset,
				limit,
				q: payload,
			}),
	};
};

type CursorParam = { before_id?: string; after_id?: string } | undefined;

export const infiniteSession = (sessionId: string) => {
	return {
		queryKey: ["aiBridgeSession", sessionId] as const,
		initialPageParam: undefined as CursorParam,
		queryFn: ({ pageParam }) =>
			API.getAIBridgeSession(sessionId, pageParam ?? {}),
		getNextPageParam: (lastPage): CursorParam => {
			const lastThread = lastPage.threads.at(-1);

			if (!lastThread) {
				return undefined;
			}

			return { after_id: lastThread.id };
		},
		getPreviousPageParam: (firstPage): CursorParam => {
			const firstThread = firstPage.threads[0];

			if (!firstThread) {
				return undefined;
			}

			return { before_id: firstThread.id };
		},
	} satisfies UseInfiniteQueryOptions<AIBridgeSessionThreadsResponse>;
};
