import { useQuery } from "@tanstack/react-query"
import { checkAuthorization } from "api/api"

export const useReadPagePermissions = (
  resource_type: string,
  resource_id?: string,
  enabled = true,
) => {
  const queryKey = ["readPagePermissions", resource_type, resource_id]
  const params = {
    checks: {
      readPagePermissions: {
        object: {
          resource_type,
          resource_id,
        },
        action: "read",
      },
    },
  }

  return useQuery({
    queryKey,
    queryFn: () => checkAuthorization(params),
    enabled,
  })
}
