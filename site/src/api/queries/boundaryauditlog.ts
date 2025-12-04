import { API } from "api/api";
import type { BoundaryAuditLogResponse } from "api/typesGenerated";
import { useFilterParamsKey } from "components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedBoundaryAuditLogs(
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<BoundaryAuditLogResponse, string> {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ payload, pageNumber }) => {
			return ["boundaryAuditLogs", payload, pageNumber] as const;
		},
		queryFn: ({ payload, limit, offset }) => {
			return API.getBoundaryAuditLogs({
				offset,
				limit,
				q: payload,
			});
		},
		prefetch: false,
	};
}
