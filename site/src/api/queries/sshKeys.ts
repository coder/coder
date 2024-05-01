import type { QueryClient } from "react-query";
import { client } from "api/api";
import type { GitSSHKey } from "api/typesGenerated";

const getUserSSHKeyQueryKey = (userId: string) => [userId, "sshKey"];

export const userSSHKey = (userId: string) => {
  return {
    queryKey: getUserSSHKeyQueryKey(userId),
    queryFn: () => client.api.getUserSSHKey(userId),
  };
};

export const regenerateUserSSHKey = (
  userId: string,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: () => client.api.regenerateUserSSHKey(userId),
    onSuccess: (newKey: GitSSHKey) => {
      queryClient.setQueryData(getUserSSHKeyQueryKey(userId), newKey);
    },
  };
};
