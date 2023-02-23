import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import * as API from "api/api"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { FC, useEffect, useState } from "react"
import { useParams } from "react-router-dom"
import { Loader } from "components/Loader/Loader"
import { firstOrItem } from "util/array"
import { workspaceMachine } from "xServices/workspace/workspaceXService"
import { WorkspaceReadyPage } from "./WorkspaceReadyPage"
import { quotaMachine } from "xServices/quotas/quotasXService"

export const WorkspacePage: FC = () => {
  const { username: usernameQueryParam, workspace: workspaceQueryParam } =
    useParams()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)
  const [workspaceState, workspaceSend] = useMachine(workspaceMachine)
  const {
    workspace,
    getWorkspaceError,
    getTemplateWarning,
    getTemplateParametersWarning,
    checkPermissionsError,
  } = workspaceState.context
  const [quotaState, quotaSend] = useMachine(quotaMachine)
  const { getQuotaError } = quotaState.context
  const styles = useStyles()

  const [startupScriptLogs, setStartupScriptLogs] = useState<
    Record<string, string> | Error | undefined | unknown
  >(undefined)

  /**
   * Get workspace, template, and organization on mount and whenever workspaceId changes.
   * workspaceSend should not change.
   */
  useEffect(() => {
    username &&
      workspaceName &&
      workspaceSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, workspaceSend])

  useEffect(() => {
    username && quotaSend({ type: "GET_QUOTA", username })
  }, [username, quotaSend])

  // Get startup logs once we have agents or when the agents change.
  // TODO: Should use xstate?  Or that new thing?
  // TODO: Does not stream yet.
  // TODO: Should maybe add to the existing SSE endpoint instead?
  useEffect(() => {
    if (workspace?.latest_build) {
      API.getStartupScriptLogs(workspace.latest_build.id)
        .then((logs) => {
          setStartupScriptLogs(logs)
        })
        .catch((error) => {
          setStartupScriptLogs(error)
        })
    }
  }, [workspace])

  return (
    <ChooseOne>
      <Cond condition={workspaceState.matches("error")}>
        <div className={styles.error}>
          {Boolean(getWorkspaceError) && (
            <AlertBanner severity="error" error={getWorkspaceError} />
          )}
          {Boolean(getTemplateWarning) && (
            <AlertBanner severity="error" error={getTemplateWarning} />
          )}
          {Boolean(getTemplateParametersWarning) && (
            <AlertBanner
              severity="error"
              error={getTemplateParametersWarning}
            />
          )}
          {Boolean(checkPermissionsError) && (
            <AlertBanner severity="error" error={checkPermissionsError} />
          )}
          {Boolean(getQuotaError) && (
            <AlertBanner severity="error" error={getQuotaError} />
          )}
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
          workspaceState={workspaceState}
          quotaState={quotaState}
          workspaceSend={workspaceSend}
          startupScriptLogs={startupScriptLogs}
        />
      </Cond>
      <Cond>
        <Loader />
      </Cond>
    </ChooseOne>
  )
}

const useStyles = makeStyles((theme) => ({
  error: {
    margin: theme.spacing(2),
  },
}))

export default WorkspacePage
