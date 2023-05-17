import { makeStyles } from "@mui/styles"
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
  const { username, workspace: workspaceName } = useParams() as {
    username: string
    workspace: string
  }
  const [workspaceState, workspaceSend] = useMachine(workspaceMachine, {
    context: {
      workspaceName,
      username,
    },
  })
  const {
    workspace,
    getWorkspaceError,
    getTemplateWarning,
    getTemplateParametersWarning,
    checkPermissionsError,
  } = workspaceState.context
  const [quotaState] = useMachine(quotaMachine, { context: { username } })
  const { getQuotaError } = quotaState.context
  const styles = useStyles()
  const failedBuildLogs = useFailedBuildLogs(workspace)

  return (
    <RequirePermission
      isFeatureVisible={getWorkspaceError?.response?.status !== 404}
    >
      <ChooseOne>
        <Cond condition={workspaceState.matches("error")}>
          <div className={styles.error}>
            {Boolean(getWorkspaceError) && (
              <ErrorAlert error={getWorkspaceError} />
            )}
            {Boolean(getTemplateWarning) && (
              <ErrorAlert error={getTemplateWarning} />
            )}
            {Boolean(getTemplateParametersWarning) && (
              <ErrorAlert error={getTemplateParametersWarning} />
            )}
            {Boolean(checkPermissionsError) && (
              <ErrorAlert error={checkPermissionsError} />
            )}
            {Boolean(getQuotaError) && <ErrorAlert error={getQuotaError} />}
          </div>
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

const useStyles = makeStyles((theme) => ({
  error: {
    margin: theme.spacing(2),
  },
}))

export default WorkspacePage
