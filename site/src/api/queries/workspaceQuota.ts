import { client } from "api/api";

export const getWorkspaceQuotaQueryKey = (username: string) => [
  username,
  "workspaceQuota",
];

export const workspaceQuota = (username: string) => {
  return {
    queryKey: getWorkspaceQuotaQueryKey(username),
    queryFn: () => client.api.getWorkspaceQuota(username),
  };
};

export const getWorkspaceResolveAutostartQueryKey = (workspaceId: string) => [
  workspaceId,
  "workspaceResolveAutostart",
];

export const workspaceResolveAutostart = (workspaceId: string) => {
  return {
    queryKey: getWorkspaceResolveAutostartQueryKey(workspaceId),
    queryFn: () => client.api.getWorkspaceResolveAutostart(workspaceId),
  };
};
