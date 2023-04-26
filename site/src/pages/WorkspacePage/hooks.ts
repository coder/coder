import {
  getWorkspaceBuildByNumber,
  stopWorkspace,
  startWorkspace,
} from "api/api"
import { delay } from "utils/delay"
import { ProvisionerJob, WorkspaceBuild } from "api/typesGenerated"
import { useMutation } from "@tanstack/react-query"

export function waitForStop(build: WorkspaceBuild) {
  return new Promise((res, reject) => {
    void (async () => {
      let latestJobInfo: ProvisionerJob | undefined = undefined

      while (latestJobInfo?.status !== "succeeded") {
        const { job } = await getWorkspaceBuildByNumber(
          build.workspace_owner_name,
          build.workspace_name,
          String(build.build_number),
        )
        latestJobInfo = job
        console.log("latest job status", latestJobInfo.status)

        if (
          ["failed", "canceled"].some((status) =>
            latestJobInfo?.status.includes(status),
          )
        ) {
          return reject(latestJobInfo)
        }

        await delay(1000)
      }

      console.log("resolving status status", latestJobInfo.status)

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
        await waitForStop(data)
        await startWorkspace({
          workspaceId: data.workspace_id,
          templateVersionId: data.template_version_id,
        })
      } catch (error) {
        if ((error as WorkspaceBuild).status === "canceled") {
          return
        }
        setRestartBuildError(error)
      }
    },
  })
}
