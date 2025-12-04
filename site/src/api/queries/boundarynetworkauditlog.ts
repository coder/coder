import { API } from "api/api";
import type { BoundaryNetworkAuditLogResponse } from "api/typesGenerated";
import { useFilterParamsKey } from "components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedBoundaryNetworkAuditLogs(
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<BoundaryNetworkAuditLogResponse, string> {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ payload, pageNumber }) => {
			return ["boundaryNetworkAuditLogs", payload, pageNumber] as const;
		},
		queryFn: ({ payload, limit, offset }) => {
			return API.getBoundaryNetworkAuditLogs({
				offset,
				limit,
				q: payload,
			});
		},
		prefetch: false,
	};
}
