import { useQuery } from "@tanstack/react-query"
import { useMachine } from "@xstate/react"
import { getWorkspaceBuildLogs } from "api/api"
import { Workspace } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Loader } from "components/Loader/Loader"
import { FC, useRef } from "react"
import { useParams } from "react-router-dom"
import { quotaMachine } from "xServices/quotas/quotasXService"
import { workspaceMachine } from "xServices/workspace/workspaceXService"
import { WorkspaceReadyPage } from "./WorkspaceReadyPage"
import { RequirePermission } from "components/RequirePermission/RequirePermission"
import { ErrorAlert } from "components/Alert/ErrorAlert"
import { useOrganizationId } from "hooks"
import { isAxiosError } from "axios"

const useFailedBuildLogs = (workspace: Workspace | undefined) => {
  const now = useRef(new Date())
  return useQuery({
    queryKey: ["logs", workspace?.latest_build.id],
    queryFn: () => {
      if (!workspace) {
        throw new Error(
          `Build log query being called before workspace is defined`,
        )
      }

      return getWorkspaceBuildLogs(workspace.latest_build.id, now.current)
    },
    enabled: workspace?.latest_build.job.error !== undefined,
  })
}

export const WorkspacePage: FC = () => {
  const {
    username,
    workspace: workspaceName,
    template: templateName,
  } = useParams() as {
    username: string
    workspace: string
    template: string
  }
  const orgId = useOrganizationId()
  const [workspaceState, workspaceSend] = useMachine(workspaceMachine, {
    context: {
      orgId,
      templateName,
      workspaceName,
      username,
    },
  })
  const { workspace, error } = workspaceState.context
  const [quotaState] = useMachine(quotaMachine, { context: { username } })
  const { getQuotaError } = quotaState.context
  const failedBuildLogs = useFailedBuildLogs(workspace)
  const pageError = error ?? getQuotaError

  return (
    <RequirePermission
      isFeatureVisible={isAxiosError(error) && error.response?.status !== 404}
    >
      <ChooseOne>
        <Cond condition={Boolean(pageError)}>
          <ErrorAlert error={pageError} sx={{ my: 2 }} />
        </Cond>
        <Cond
          condition={
            Boolean(workspace) &&
            workspaceState.matches("ready") &&
            quotaState.matches("success")
          }
        >
          <WorkspaceReadyPage
            failedBuildLogs={failedBuildLogs.data}
            workspaceState={workspaceState}
            quotaState={quotaState}
            workspaceSend={workspaceSend}
          />
        </Cond>
        <Cond>
          <Loader />
        </Cond>
      </ChooseOne>
    </RequirePermission>
  )
}

export default WorkspacePage
