import { useQuery } from "@tanstack/react-query";
import * as API from "api/api";

const getWorkspaceQuotaQueryKey = (username: string) => [
  username,
  "workspaceQuota",
];

export const useWorkspaceQuota = (username: string) => {
  return useQuery({
    queryKey: getWorkspaceQuotaQueryKey(username),
    queryFn: () => API.getWorkspaceQuota(username),
  });
};
