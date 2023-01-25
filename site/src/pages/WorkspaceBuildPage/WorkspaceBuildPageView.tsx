import { BuildAvatar } from "components/BuildsTable/BuildAvatar"
import { FC } from "react"
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated"
import { Loader } from "../../components/Loader/Loader"
import { Margins } from "../../components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { WorkspaceBuildLogs } from "../../components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { WorkspaceBuildStats } from "../../components/WorkspaceBuildStats/WorkspaceBuildStats"
import { WorkspaceBuildStateError } from "./WorkspaceBuildStateError"

const sortLogsByCreatedAt = (logs: ProvisionerJobLog[]) => {
  return [...logs].sort(
    (a, b) =>
      new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  )
}

export interface WorkspaceBuildPageViewProps {
  logs: ProvisionerJobLog[] | undefined
  build: WorkspaceBuild | undefined
}

export const WorkspaceBuildPageView: FC<WorkspaceBuildPageViewProps> = ({
  logs,
  build,
}) => {
  return (
    <Margins>
      {build && (
        <PageHeader>
          <Stack direction="row" alignItems="center" spacing={3}>
            <BuildAvatar build={build} size="xl" />
            <div>
              <PageHeaderTitle>Build #{build.build_number}</PageHeaderTitle>
              <PageHeaderSubtitle condensed>
                {build.initiator_name}
              </PageHeaderSubtitle>
            </div>
          </Stack>
        </PageHeader>
      )}

      <Stack spacing={4}>
        {build &&
          build.transition === "delete" &&
          build.job.status === "failed" && (
            <WorkspaceBuildStateError build={build} />
          )}
        {build && <WorkspaceBuildStats build={build} />}
        {!logs && <Loader />}
        {logs && <WorkspaceBuildLogs logs={sortLogsByCreatedAt(logs)} />}
      </Stack>
    </Margins>
  )
}
