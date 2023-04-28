import { restartWorkspace } from "api/api"
import { useMutation } from "@tanstack/react-query"

export const useRestartWorkspace = () => {
  return useMutation({
    mutationFn: restartWorkspace,
  })
}
