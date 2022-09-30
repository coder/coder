import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import { FC, useEffect } from "react"
import { useParams } from "react-router-dom"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { firstOrItem } from "../../util/array"
import { workspaceMachine } from "../../xServices/workspace/workspaceXService"
import { WorkspaceReadyPage } from "./WorkspaceReadyPage"

export const WorkspacePage: FC = () => {
  const { username: usernameQueryParam, workspace: workspaceQueryParam } = useParams()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)
  const [workspaceState, workspaceSend] = useMachine(workspaceMachine)
  const { workspace, getWorkspaceError, getTemplateWarning, checkPermissionsError } =
    workspaceState.context
  const styles = useStyles()

  /**
   * Get workspace, template, and organization on mount and whenever workspaceId changes.
   * workspaceSend should not change.
   */
  useEffect(() => {
    username && workspaceName && workspaceSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, workspaceSend])

  if (workspaceState.matches("error")) {
    return (
      <div className={styles.error}>
        {Boolean(getWorkspaceError) && <ErrorSummary error={getWorkspaceError} />}
        {Boolean(getTemplateWarning) && <ErrorSummary error={getTemplateWarning} />}
        {Boolean(checkPermissionsError) && <ErrorSummary error={checkPermissionsError} />}
      </div>
    )
  } else if (workspace && workspaceState.matches("ready")) {
    return <WorkspaceReadyPage workspaceState={workspaceState} workspaceSend={workspaceSend} />
  } else {
    return <FullScreenLoader />
  }
}

const useStyles = makeStyles((theme) => ({
  error: {
    margin: theme.spacing(2),
  },
}))

export default WorkspacePage
