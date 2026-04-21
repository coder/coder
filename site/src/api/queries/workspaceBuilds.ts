import type {
	QueryOptions,
	UseInfiniteQueryOptions,
	UseQueryOptions,
} from "react-query";
import { API } from "#/api/api";
import type {
	ProvisionerJobLog,
	WorkspaceBuild,
	WorkspaceBuildParameter,
	WorkspaceBuildsRequest,
} from "#/api/typesGenerated";

export function workspaceBuildParametersKey(workspaceBuildId: string) {
	return ["workspaceBuilds", workspaceBuildId, "parameters"] as const;
}

export function workspaceBuildParameters(workspaceBuildId: string) {
	return {
		queryKey: workspaceBuildParametersKey(workspaceBuildId),
		queryFn: () => API.getWorkspaceBuildParameters(workspaceBuildId),
	} as const satisfies QueryOptions<WorkspaceBuildParameter[]>;
}

export const workspaceBuildByNumber = (
	username: string,
	workspaceName: string,
	buildNumber: number,
) => {
	return {
		queryKey: ["workspaceBuild", username, workspaceName, buildNumber],
		queryFn: () =>
			API.getWorkspaceBuildByNumber(username, workspaceName, buildNumber),
	};
};

export const workspaceBuildsKey = (workspaceId: string) => [
	"workspaceBuilds",
	workspaceId,
];

export const infiniteWorkspaceBuilds = (
	workspaceId: string,
	req?: WorkspaceBuildsRequest,
) => {
	const limit = req?.limit ?? 25;

	return {
		queryKey: [...workspaceBuildsKey(workspaceId), req],
		getNextPageParam: (lastPage, pages) => {
			if (lastPage.length < limit) {
				return undefined;
			}
			return pages.length + 1;
		},
		initialPageParam: 0,
		queryFn: ({ pageParam }) => {
			if (typeof pageParam !== "number") {
				throw new Error("pageParam must be a number");
			}
			return API.getWorkspaceBuilds(workspaceId, {
				limit,
				offset: pageParam <= 0 ? 0 : (pageParam - 1) * limit,
			});
		},
	} satisfies UseInfiniteQueryOptions<WorkspaceBuild[]>;
};

function workspaceBuildLogsKey(workspaceBuildId: string) {
	return ["workspaceBuilds", workspaceBuildId, "logs"] as const;
}

// Fetches build logs via REST. Completed build logs are immutable,
// so the query uses infinite staleTime to cache across re-mounts
// (e.g. collapsible expand/collapse cycles).
export function workspaceBuildLogs(workspaceBuildId: string) {
	return {
		queryKey: workspaceBuildLogsKey(workspaceBuildId),
		queryFn: () => API.getWorkspaceBuildLogs(workspaceBuildId),
		staleTime: Number.POSITIVE_INFINITY,
		gcTime: 10 * 60 * 1000, // 10 minutes. Avoids holding logs in cache forever.
		refetchOnMount: false,
		refetchOnReconnect: false,
		refetchOnWindowFocus: false,
	} as const satisfies UseQueryOptions<ProvisionerJobLog[]>;
}

// We use readyAgentsCount to invalidate the query when an agent connects
export const workspaceBuildTimings = (workspaceBuildId: string) => {
	return {
		queryKey: ["workspaceBuilds", workspaceBuildId, "timings"],
		queryFn: () => API.workspaceBuildTimings(workspaceBuildId),
	};
};
