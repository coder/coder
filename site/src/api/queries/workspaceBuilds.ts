import { UseInfiniteQueryOptions } from "react-query";
import * as API from "api/api";
import { WorkspaceBuild, WorkspaceBuildsRequest } from "api/typesGenerated";

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

export const infiniteWorkspaceBuilds = (
  workspaceId: string,
  req?: WorkspaceBuildsRequest,
): UseInfiniteQueryOptions<WorkspaceBuild[]> => {
  const limit = req?.limit ?? 25;

  return {
    queryKey: ["workspaceBuilds", workspaceId, req],
    getNextPageParam: (lastPage, pages) => {
      return pages.length + 1;
    },
    queryFn: ({ pageParam = 0 }) => {
      return API.getWorkspaceBuilds(workspaceId, {
        limit,
        offset: pageParam <= 0 ? 0 : (pageParam - 1) * limit,
      });
    },
  };
};
