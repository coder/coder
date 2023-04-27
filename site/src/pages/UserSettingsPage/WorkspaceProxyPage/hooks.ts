import { useQuery } from "@tanstack/react-query"
import { getWorkspaceProxies } from "api/api"

// Loads all workspace proxies
export const useWorkspaceProxiesData = () => {
  const queryKey = ["workspace-proxies"]
  return useQuery({
    queryKey,
    queryFn: getWorkspaceProxies,
  })
}
