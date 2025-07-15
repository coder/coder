import { API } from "api/api";
import type { ConnectionLogResponse } from "api/typesGenerated";
import { useFilterParamsKey } from "components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedConnectionLogs(
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<ConnectionLogResponse, string> {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ payload, pageNumber }) => {
			return ["connectionLogs", payload, pageNumber] as const;
		},
		queryFn: ({ payload, limit, offset }) => {
			return API.getConnectionLogs({
				offset,
				limit,
				q: payload,
			});
		},
		prefetch: false,
	};
}
