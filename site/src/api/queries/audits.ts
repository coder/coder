import { API } from "api/api";
import type { AuditLogResponse } from "api/typesGenerated";
import { useFilterParamsKey } from "components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedAudits(
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<AuditLogResponse, string> {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ payload, pageNumber }) => {
			return ["auditLogs", payload, pageNumber] as const;
		},
		queryFn: ({ payload, limit, offset }) => {
			return API.getAuditLogs({
				offset,
				limit,
				q: payload,
			});
		},
		prefetch: false,
	};
}
