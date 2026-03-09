import { API } from "api/api";
import type {
	AIBridgeListInterceptionsResponse,
	AIBridgeSessionListResponse,
	AIBridgeSessionResponse,
} from "api/typesGenerated";
import { useFilterParamsKey } from "components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";
import type { UseQueryOptions } from "react-query";

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
	// FIXME remove typeof when we get generated types
): UsePaginatedQueryOptions<AIBridgeSessionListResponse, string> => {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ payload, pageNumber }) => {
			return ["aiBridgeSessions", payload, pageNumber] as const;
		},
		queryFn: () => API.getAIBridgeSessionList({}),
	};
};

// FIXME paginate once we get the real APIs
export const paginatedSession = (
	sessionId: string,
): UseQueryOptions<AIBridgeSessionResponse, string> => {
	return {
		queryKey: ["aiBridgeSession", sessionId],
		queryFn: () => API.getAIBridgeSession(sessionId, {}),
	};
};
