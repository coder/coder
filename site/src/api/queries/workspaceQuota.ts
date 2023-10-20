import * as API from "api/api";

const getWorkspaceQuotaQueryKey = (username: string) => [
  username,
  "workspaceQuota",
];

export const workspaceQuota = (username: string) => {
  return {
    queryKey: getWorkspaceQuotaQueryKey(username),
    queryFn: () => API.getWorkspaceQuota(username),
  };
};
