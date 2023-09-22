import * as API from "api/api";

export const workspaceBuildByNumber = (
  username: string,
  workspaceName: string,
  buildNumber: number,
) => {
  return {
    queryKey: [username, workspaceName, "workspaceBuild", buildNumber],
    queryFn: () =>
      API.getWorkspaceBuildByNumber(username, workspaceName, buildNumber),
  };
};
