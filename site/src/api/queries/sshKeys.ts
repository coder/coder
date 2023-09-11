import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as API from "api/api";

const getUserSSHKeyQueryKey = (userId: string) => [userId, "sshKey"];

export const useUserSSHKey = (userId: string) => {
  return useQuery({
    queryKey: getUserSSHKeyQueryKey(userId),
    queryFn: () => API.getUserSSHKey(userId),
  });
};

export const useRegenerateUserSSHKey = (
  userId: string,
  onSuccess: () => void,
) => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => API.regenerateUserSSHKey(userId),
    onSuccess: (newKey) => {
      queryClient.setQueryData(getUserSSHKeyQueryKey(userId), newKey);
      onSuccess();
    },
  });
};
