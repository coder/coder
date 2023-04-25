import { useMutation } from "@tanstack/react-query"
import {
  stopWorkspace,
  getWorkspaceBuildByNumber,
  startWorkspace,
} from "api/api"
import { delay } from "utils/delay"
import { ProvisionerJob, WorkspaceBuild } from "api/typesGenerated"

function waitForStop(
  username: string,
  workspaceName: string,
  buildNumber: string,
) {
  return new Promise((res, reject) => {
    void (async () => {
      let latestJobInfo: ProvisionerJob | undefined = undefined

      while (latestJobInfo?.status !== "succeeded") {
        const { job } = await getWorkspaceBuildByNumber(
          username,
          workspaceName,
          buildNumber,
        )
        latestJobInfo = job

        if (
          ["failed", "canceled"].some((status) =>
            latestJobInfo?.status.includes(status),
          )
        ) {
          return reject(latestJobInfo)
        }

        await delay(1000)
      }

      return res(latestJobInfo)
    })()
  })
}

export const useRestartWorkspace = (
  setRestartBuildError: (arg: Error | unknown | undefined) => void,
) => {
  return useMutation({
    mutationFn: stopWorkspace,
    onSuccess: async (data) => {
      try {
        await waitForStop(
          data.workspace_owner_name,
          data.workspace_name,
          String(data.build_number),
        )
        await startWorkspace(data.workspace_id, data.template_version_id)
      } catch (error) {
        if ((error as WorkspaceBuild).status === "canceled") {
          return
        }
        setRestartBuildError(error)
      }
    },
  })
}
