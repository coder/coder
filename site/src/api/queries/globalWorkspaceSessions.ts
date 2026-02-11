import { API } from "api/api";
import type { GlobalWorkspaceSessionsResponse } from "api/typesGenerated";
import { useFilterParamsKey } from "components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedGlobalWorkspaceSessions(
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<GlobalWorkspaceSessionsResponse, string> {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ payload, pageNumber }) => {
			return ["globalWorkspaceSessions", payload, pageNumber] as const;
		},
		queryFn: ({ payload, limit, offset }) => {
			return API.getGlobalWorkspaceSessions({
				offset,
				limit,
				q: payload,
			});
		},
		prefetch: false,
	};
}
