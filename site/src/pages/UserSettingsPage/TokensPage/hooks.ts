import {
  type QueryKey,
  useMutation,
  useQuery,
  useQueryClient,
} from "react-query";
import { getTokens, deleteToken } from "api/api";
import type { TokensFilter } from "api/typesGenerated";

// Load all tokens
export const useTokensData = ({ include_all }: TokensFilter) => {
  const queryKey = ["tokens", include_all];
  const result = useQuery({
    queryKey,
    queryFn: () =>
      getTokens({
        include_all,
      }),
  });

  return {
    queryKey,
    ...result,
  };
};

// Delete a token
export const useDeleteToken = (queryKey: QueryKey) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: deleteToken,
    onSuccess: () => {
      // Invalidate and refetch
      void queryClient.invalidateQueries(queryKey);
    },
  });
};
