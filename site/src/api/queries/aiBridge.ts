import type { UseInfiniteQueryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeSessionThreadsResponse,
} from "#/api/typesGenerated";

const SESSIONS_INFINITE_PAGE_SIZE = 25;
const SESSION_THREADS_INFINITE_PAGE_SIZE = 20;

export const infiniteSessions = (filterQuery: string) => {
	return {
		queryKey: ["aiBridgeSessions", filterQuery],
		// Without a staleTime, a remount would refetch every loaded cursor
		// page sequentially. One minute matches usePaginatedQuery's default.
		staleTime: 60_000,
		getNextPageParam: (lastPage: AIBridgeListSessionsResponse) => {
			const sessions = lastPage.sessions;
			if (sessions.length < SESSIONS_INFINITE_PAGE_SIZE) {
				return undefined;
			}
			return sessions.at(-1)?.id;
		},
		initialPageParam: undefined as string | undefined,
		queryFn: ({ pageParam }) =>
			API.getAIBridgeSessionList({
				limit: SESSIONS_INFINITE_PAGE_SIZE,
				after_session_id: pageParam as string | undefined,
				q: filterQuery,
			}),
	} satisfies UseInfiniteQueryOptions<AIBridgeListSessionsResponse>;
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
