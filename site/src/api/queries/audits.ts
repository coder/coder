import { getAuditLogs } from "api/api";
import { type AuditLogResponse } from "api/typesGenerated";
import { type UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedAudits(
  searchParams: URLSearchParams,
  filterParamsKey: string,
) {
  return {
    searchParams,
    queryPayload: ({ searchParams }) => {
      return searchParams.get(filterParamsKey) ?? "";
    },
    queryKey: ({ payload, pageNumber }) => {
      return ["auditLogs", payload, pageNumber] as const;
    },
    queryFn: ({ payload, limit, offset }) => {
      return getAuditLogs({
        offset,
        limit,
        q: payload,
      });
    },

    cacheTime: 5 * 1000 * 60,
  } as const satisfies UsePaginatedQueryOptions<AuditLogResponse, string>;
}
