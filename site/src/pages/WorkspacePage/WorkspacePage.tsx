import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Loader } from "components/Loader/Loader"
import { FC } from "react"
import { useParams } from "react-router-dom"
import { quotaMachine } from "xServices/quotas/quotasXService"
import { workspaceMachine } from "xServices/workspace/workspaceXService"
import { WorkspaceReadyPage } from "./WorkspaceReadyPage"

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
