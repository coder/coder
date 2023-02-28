import {
  useQuery,
  useMutation,
  useQueryClient,
  QueryKey,
} from "@tanstack/react-query"
import { getTokens, deleteAPIKey } from "api/api"
import { TokensFilter } from "api/typesGenerated"

export const useTokensData = ({ include_all }: TokensFilter) => {
  const queryKey = ["tokens", include_all]
  const result = useQuery({
    queryKey,
    queryFn: () =>
      getTokens({
        include_all,
      }),
  })

  return {
    queryKey,
    ...result,
  }
}

export const useDeleteToken = (queryKey: QueryKey) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: deleteAPIKey,
    onSuccess: () => {
      // Invalidate and refetch
      void queryClient.invalidateQueries(queryKey)
    },
  })
}
