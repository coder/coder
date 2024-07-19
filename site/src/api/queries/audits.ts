import { API } from "api/api";
import type { AuditLogResponse } from "api/typesGenerated";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";
import { filterParamsKey } from "utils/filters";

export function paginatedAudits(
  searchParams: URLSearchParams,
): UsePaginatedQueryOptions<AuditLogResponse, string> {
  return {
    searchParams,
    queryPayload: () => searchParams.get(filterParamsKey) ?? "",
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
