import {
  useQuery,
  useMutation,
  useQueryClient,
  QueryKey,
} from "@tanstack/react-query"
import { getTokens, deleteAPIKey, checkAuthorization } from "api/api"
import { TokensFilter } from "api/typesGenerated"

// Owners have the ability to read all API tokens,
// whereas members can only see the tokens they have created.
// We check permissions here to determine whether to display the
// 'View All' switch on the TokensPage.
export const useCheckTokenPermissions = () => {
  const queryKey = ["auth"]
  const params = {
    checks: {
      readAllApiKeys: {
        object: {
          resource_type: "api_key",
        },
        action: "read",
      },
    },
  }
  return useQuery({
    queryKey,
    queryFn: () => checkAuthorization(params),
  })
}

// Load all tokens
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

// Delete a token
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
