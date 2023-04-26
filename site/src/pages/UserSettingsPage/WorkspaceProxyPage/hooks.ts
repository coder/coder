import { useQuery } from "@tanstack/react-query"
import { getWorkspaceProxies } from "api/api"

// Loads all workspace proxies
export const useWorkspaceProxiesData = () => {
  const queryKey = ["workspace-proxies"]
  const result = useQuery({
    queryKey,
    queryFn: () => getWorkspaceProxies(),
  })

  return {
    ...result,
  }
}
