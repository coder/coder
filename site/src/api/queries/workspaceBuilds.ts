import type { QueryOptions, UseInfiniteQueryOptions } from "react-query";
import { client } from "api/api";
import type {
  WorkspaceBuild,
  WorkspaceBuildParameter,
  WorkspaceBuildsRequest,
} from "api/typesGenerated";

export function workspaceBuildParametersKey(workspaceBuildId: string) {
  return ["workspaceBuilds", workspaceBuildId, "parameters"] as const;
}

export function workspaceBuildParameters(workspaceBuildId: string) {
  return {
    queryKey: workspaceBuildParametersKey(workspaceBuildId),
    queryFn: () => client.api.getWorkspaceBuildParameters(workspaceBuildId),
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
      client.api.getWorkspaceBuildByNumber(
        username,
        workspaceName,
        buildNumber,
      ),
  };
};

export const workspaceBuildsKey = (workspaceId: string) => [
  "workspaceBuilds",
  workspaceId,
];

export const infiniteWorkspaceBuilds = (
  workspaceId: string,
  req?: WorkspaceBuildsRequest,
): UseInfiniteQueryOptions<WorkspaceBuild[]> => {
  const limit = req?.limit ?? 25;

  return {
    queryKey: [...workspaceBuildsKey(workspaceId), req],
    getNextPageParam: (lastPage, pages) => {
      if (lastPage.length < limit) {
        return undefined;
      }
      return pages.length + 1;
    },
    queryFn: ({ pageParam = 0 }) => {
      return client.api.getWorkspaceBuilds(workspaceId, {
        limit,
        offset: pageParam <= 0 ? 0 : (pageParam - 1) * limit,
      });
    },
  };
};
