import * as API from "api/api";
import { WorkspaceBuildsRequest } from "api/typesGenerated";

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

export const workspaceBuilds = (
  workspaceId: string,
  req?: WorkspaceBuildsRequest,
) => {
  return {
    queryKey: ["workspaceBuilds", workspaceId, req],
    queryFn: () => {
      return API.getWorkspaceBuilds(workspaceId, req);
    },
  };
};
