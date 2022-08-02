import { FC } from "react"
import { ProvisionerJobLog, WorkspaceBuild } from "../../api/typesGenerated"
import { Loader } from "../../components/Loader/Loader"
import { Margins } from "../../components/Margins/Margins"
import { PageHeader, PageHeaderTitle } from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { WorkspaceBuildLogs } from "../../components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { WorkspaceBuildStats } from "../../components/WorkspaceBuildStats/WorkspaceBuildStats"

const sortLogsByCreatedAt = (logs: ProvisionerJobLog[]) => {
  return [...logs].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  )
}

export interface WorkspaceBuildPageViewProps {
  logs: ProvisionerJobLog[] | undefined
  build: WorkspaceBuild | undefined
}

export const WorkspaceBuildPageView: FC<React.PropsWithChildren<WorkspaceBuildPageViewProps>> = ({ logs, build }) => {
  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>Logs</PageHeaderTitle>
      </PageHeader>

      <Stack>
        {build && <WorkspaceBuildStats build={build} />}
        {!logs && <Loader />}
        {logs && <WorkspaceBuildLogs logs={sortLogsByCreatedAt(logs)} />}
      </Stack>
    </Margins>
  )
}
