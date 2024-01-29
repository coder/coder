import * as API from "api/api";

export const getWorkspaceQuotaQueryKey = (username: string) => [
  username,
  "workspaceQuota",
];

export const workspaceQuota = (username: string) => {
  return {
    queryKey: getWorkspaceQuotaQueryKey(username),
    queryFn: () => API.getWorkspaceQuota(username),
  };
};

export const getWorkspaceResolveAutostartQueryKey = (workspaceId: string) => [
  workspaceId,
  "workspaceResolveAutostart",
];

export const workspaceResolveAutostart = (workspaceId: string) => {
  return {
    queryKey: getWorkspaceResolveAutostartQueryKey(workspaceId),
    queryFn: () => API.getWorkspaceResolveAutostart(workspaceId),
  };
};
