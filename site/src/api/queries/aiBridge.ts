import { API } from "api/api";
import type { AIBridgeListInterceptionsResponse } from "api/typesGenerated";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";
import { useFilterParamsKey } from "#/components/Filter/Filter";

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
