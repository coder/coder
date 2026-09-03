import type { UseInfiniteQueryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeSessionThreadsResponse,
	AIGatewaySpendUsersResponse,
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

interface AIGatewaySpendDateParams {
	start_date: string;
	end_date: string;
}

interface PaginatedAIGatewaySpendUsersPayload extends AIGatewaySpendDateParams {
	search: string;
}

export const paginatedAIGatewaySpendUsers = (
	payload: PaginatedAIGatewaySpendUsersPayload,
): UsePaginatedQueryOptions<
	AIGatewaySpendUsersResponse,
	PaginatedAIGatewaySpendUsersPayload
> => {
	return {
		queryPayload: () => payload,
		queryKey: ({ payload, pageNumber }) =>
			["aiGatewaySpendUsers", payload, pageNumber] as const,
		queryFn: ({ payload, limit, offset }) =>
			API.getAIGatewaySpendUsers({
				start_date: payload.start_date,
				end_date: payload.end_date,
				search: payload.search || undefined,
				limit,
				offset,
			}),
		staleTime: 60_000,
	};
};

export const aiGatewaySpendUserSummary = (
	user: string,
	params: AIGatewaySpendDateParams,
) => ({
	queryKey: ["aiGatewaySpendUserSummary", user, params] as const,
	queryFn: () => API.getAIGatewaySpendUserSummary(user, params),
	staleTime: 60_000,
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
