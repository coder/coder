import { API } from "api/api";
import type { WorkspaceSessionsResponse } from "api/typesGenerated";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";

export function paginatedWorkspaceSessions(
	workspaceId: string | undefined,
): UsePaginatedQueryOptions<WorkspaceSessionsResponse, never> {
	return {
		enabled: !!workspaceId,
		queryKey: ({ pageNumber }) => {
			return ["workspaceSessions", workspaceId, pageNumber] as const;
		},
		queryFn: ({ limit, offset }) => {
			return API.getWorkspaceSessions(workspaceId!, { offset, limit });
		},
		prefetch: false,
	};
}
