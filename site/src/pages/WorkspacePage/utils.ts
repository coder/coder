import { getWorkspaceBuildByNumber } from "api/api"
import { delay } from "utils/delay"
import { ProvisionerJob, WorkspaceBuild } from "api/typesGenerated"

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
