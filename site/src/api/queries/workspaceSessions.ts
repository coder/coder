import { API } from "api/api";
import type { WorkspaceSessionsResponse } from "api/typesGenerated";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedWorkspaceSessions(
	workspaceId: string,
): UsePaginatedQueryOptions<WorkspaceSessionsResponse, never> {
	return {
		queryKey: ({ pageNumber }) => {
			return ["workspaceSessions", workspaceId, pageNumber] as const;
		},
		queryFn: ({ limit, offset }) => {
			return API.getWorkspaceSessions(workspaceId, { offset, limit });
		},
		prefetch: false,
	};
}
