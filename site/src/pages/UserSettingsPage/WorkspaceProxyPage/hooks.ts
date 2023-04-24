import {
  useQuery,
  useMutation,
  useQueryClient,
  QueryKey,
} from "@tanstack/react-query"
import { deleteToken, getWorkspaceProxies } from "api/api"


// Loads all workspace proxies
export const useWorkspaceProxiesData = () => {
  const result = useQuery({
    queryFn: () =>
      getWorkspaceProxies(),
  })

  return {
    ...result,
  }
}


// Delete a token
export const useDeleteToken = (queryKey: QueryKey) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: deleteToken,
    onSuccess: () => {
      // Invalidate and refetch
      void queryClient.invalidateQueries(queryKey)
    },
  })
}
