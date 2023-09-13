import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";
import { GitSSHKey } from "api/typesGenerated";

const getUserSSHKeyQueryKey = (userId: string) => [userId, "sshKey"];

export const userSSHKey = (userId: string) => {
  return {
    queryKey: getUserSSHKeyQueryKey(userId),
    queryFn: () => API.getUserSSHKey(userId),
  };
};

export const regenerateUserSSHKey = (
  userId: string,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: () => API.regenerateUserSSHKey(userId),
    onSuccess: (newKey: GitSSHKey) => {
      queryClient.setQueryData(getUserSSHKeyQueryKey(userId), newKey);
    },
  };
};
